// Package healthkit provides Kubernetes-style health check aggregation.
//
// # Health Check Probes
//
// HealthKit supports three probe types matching Kubernetes conventions:
//
//   - Liveness: Indicates if the component is running. If this fails, the
//     component/should be restarted.
//   - Readiness: Indicates if the component is ready to serve traffic.
//     If this fails, traffic should not be sent to the component.
//   - Startup: Indicates if the component's initial startup is complete.
//     This is an alternative to liveness for slow-starting containers.
//
// # Check Registration
//
// Components register health checks via the Register method. Each check has
// a name, kind, timeout, and check function:
//
//	health.Register(healthkit.Check{
//	    Name: "database",
//	    Kind: healthkit.Liveness,
//	    Timeout: 2 * time.Second,
//	    Fn: func(ctx context.Context) error {
//	        return db.Ping(ctx)
//	    },
//	})
//
// # HTTP Handlers
//
// HealthKit provides HTTP handlers for each probe type, typically mounted
// at /health/liveness, /health/readiness, and /health/startup:
//
//	http.HandleFunc("/health/liveness", health.Handler(healthkit.Liveness))
//
// # Caching
//
// Check results can be cached to reduce overhead. The cache TTL is configured
// when creating the Aggregator. Cached results are stored per probe type.
//
// Example:
//
//	health := healthkit.NewAggregator(5 * time.Second)
//	health.Register(db.HealthChecks()...)
//
//	// In HTTP setup
//	router.Get("/health/liveness", health.Handler(healthkit.Liveness))
package healthkit

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// Kind represents the type of health check probe.
//
// The three probe types match Kubernetes conventions:
//   - Liveness: Is the component running?
//   - Readiness: Is the component ready to serve traffic?
//   - Startup: Has the component completed initial startup?
type Kind int

const (
	// Liveness probe indicates if the component is alive and running.
	// If a liveness check fails, the component should be restarted.
	Liveness Kind = iota

	// Readiness probe indicates if the component is ready to serve traffic.
	// If a readiness check fails, traffic should not be sent to the component.
	Readiness

	// Startup probe indicates if the component has completed its initial
	// startup sequence. This is useful for components that take longer to
	// start than the liveness probe timeout would allow.
	Startup
)

func (k Kind) String() string {
	switch k {
	case Liveness:
		return "liveness"
	case Readiness:
		return "readiness"
	case Startup:
		return "startup"
	default:
		return "unknown"
	}
}

// Check represents a single health check probe.
//
// Checks are registered with an Aggregator and executed when the corresponding
// probe type is evaluated. The check function runs with a timeout to prevent
// hung checks from blocking indefinitely.
//
// Example:
//
//	Check{
//	    Name: "postgres",
//	    Kind: Liveness,
//	    Timeout: 2 * time.Second,
//	    Fn: func(ctx context.Context) error {
//	        return db.Ping(ctx)
//	    },
//	}
type Check struct {
	// Name is a unique identifier for this check, used in error messages.
	Name string

	// Kind specifies the probe type (Liveness, Readiness, or Startup).
	Kind Kind

	// Timeout is the maximum time to wait for the check function to complete.
	// If zero, a default timeout of 300ms is used.
	Timeout time.Duration

	// Fn is the function that executes the health check.
	// It should return nil if the check passes, or an error describing the failure.
	Fn func(ctx context.Context) error
}

// Aggregator collects and executes health checks for all probe types.
//
// The Aggregator runs checks concurrently within each probe type, aggregates
// errors, and optionally caches results to reduce execution overhead. Failed
// checks are combined into a single error message with all failures.
//
// # Caching
//
// When cacheTTL is greater than zero, check results are cached for the
// specified duration. Each probe type has its own cache. Cached results
// are returned without re-executing the checks.
//
// # Thread Safety
//
// The Aggregator is safe for concurrent use. Checks can be registered
// and handlers invoked from multiple goroutines.
//
// Example:
//
//	health := NewAggregator(5 * time.Second)
//	health.Register(
//	    Check{Name: "db", Kind: Liveness, Fn: dbPing},
//	    Check{Name: "cache", Kind: Liveness, Fn: cachePing},
//	)
//
//	http.HandleFunc("/health/liveness", health.Handler(Liveness))

// cachedResult wraps an evaluation result for atomic storage.
// atomic.Value requires a consistent concrete type.
type cachedResult struct {
	err error
}

type Aggregator struct {
	mu     sync.RWMutex
	checks map[Kind][]Check

	cacheTTL time.Duration
	cacheAt  [3]int64
	cacheErr [3]atomic.Value
}

// NewAggregator creates a new health check aggregator.
//
// The cacheTTL parameter specifies how long to cache check results. A zero
// value disables caching, while a positive value caches results for the
// specified duration per probe type.
//
// Example:
//
//	// No caching
//	health := NewAggregator(0)
//
//	// Cache for 5 seconds
//	health := NewAggregator(5 * time.Second)
func NewAggregator(cacheTTL time.Duration) *Aggregator {
	return &Aggregator{
		checks:   make(map[Kind][]Check),
		cacheTTL: cacheTTL,
	}
}

// Register adds one or more health checks to the aggregator.
//
// Checks are grouped by their Kind. Multiple checks for the same probe type
// are executed concurrently when that probe type is evaluated.
//
// If a check's Timeout is zero or negative, it is set to 300 milliseconds.
//
// Register is safe to call from multiple goroutines.
//
// Example:
//
//	health.Register(
//	    Check{Name: "db", Kind: Liveness, Fn: dbPing},
//	    Check{Name: "cache", Kind: Readiness, Fn: cachePing},
//	)
func (a *Aggregator) Register(checks ...Check) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, c := range checks {
		if c.Timeout <= 0 {
			c.Timeout = 300 * time.Millisecond
		}
		a.checks[c.Kind] = append(a.checks[c.Kind], c)
	}
}

// Handler returns an HTTP handler function for the specified probe type.
//
// The handler executes all checks registered for the given Kind and returns:
//   - HTTP 200 OK: All checks passed
//   - HTTP 503 Service Unavailable: One or more checks failed
//
// The response body contains "ok" on success, or a semicolon-separated list
// of check errors on failure.
//
// Example:
//
//	router.Get("/health/liveness", health.Handler(Liveness))
//	router.Get("/health/readiness", health.Handler(Readiness))
//	router.Get("/health/startup", health.Handler(Startup))
func (a *Aggregator) Handler(kind Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.evaluate(r.Context(), kind); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func (a *Aggregator) evaluate(ctx context.Context, kind Kind) error {
	now := time.Now()
	last := time.Unix(0, atomic.LoadInt64(&a.cacheAt[kind]))

	if a.cacheTTL > 0 && now.Sub(last) < a.cacheTTL {
		if v := a.cacheErr[kind].Load(); v != nil {
			return v.(cachedResult).err
		}
		return nil
	}

	a.mu.RLock()
	checks := append([]Check(nil), a.checks[kind]...)
	a.mu.RUnlock()

	errCh := make(chan error, len(checks))
	var wg sync.WaitGroup
	wg.Add(len(checks))
	for _, c := range checks {
		check := c
		go func() {
			defer wg.Done()
			ctxTimeout, cancel := context.WithTimeout(ctx, check.Timeout)
			defer cancel()
			if err := check.Fn(ctxTimeout); err != nil {
				errCh <- errors.New(check.Name + ": " + err.Error())
			}
		}()
	}
	wg.Wait()
	close(errCh)

	var err error
	for e := range errCh {
		if err == nil {
			err = e
		} else {
			err = errors.New(err.Error() + "; " + e.Error())
		}
	}

	a.cacheErr[kind].Store(cachedResult{err: err})
	atomic.StoreInt64(&a.cacheAt[kind], now.UnixNano())
	return err
}
