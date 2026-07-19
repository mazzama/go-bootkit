package healthkit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Kind int

const (
	Liveness Kind = iota
	Readiness
	Startup
)

type Check struct {
	Fn      func(ctx context.Context) error
	Name    string
	Kind    Kind
	Timeout time.Duration
}

type cachedResult struct {
	err error
	at  time.Time
}

type Aggregator struct {
	cache    [3]atomic.Value
	checks   map[Kind][]Check
	cacheTTL time.Duration
	mu       sync.RWMutex
}

// StandardChecks returns the standard liveness/readiness pair shared by
// infrastructure components. The liveness check is a no-op (it exists to keep
// pod restart storms at bay during transient backend issues); the readiness
// actively reflects backend availability. Callers supply only the readiness closure.
func StandardChecks(name string, readyFn func(ctx context.Context) error) []Check {
	return []Check{
		{
			Name: name + "-liveness",
			Kind: Liveness,
			Fn: func(ctx context.Context) error {
				return nil
			},
		},
		{
			Name:    name + "-readiness",
			Kind:    Readiness,
			Timeout: 2 * time.Second,
			Fn:      readyFn,
		},
	}
}

func NewAggregator(cacheTTL time.Duration) *Aggregator {
	return &Aggregator{
		checks:   make(map[Kind][]Check),
		cacheTTL: cacheTTL,
	}
}

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

func (a *Aggregator) Handler(kind Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := a.evaluate(r.Context(), kind); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(err.Error())) //nolint:errcheck
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok")) //nolint:errcheck
	}
}

func (a *Aggregator) evaluate(ctx context.Context, kind Kind) error {
	now := time.Now()

	if a.cacheTTL > 0 {
		if v := a.cache[kind].Load(); v != nil {
			if res, ok := v.(cachedResult); ok {
				if now.Sub(res.at) < a.cacheTTL {
					return res.err
				}
			}
		}
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
				errCh <- fmt.Errorf("%s: %w", check.Name, err)
			}
		}()
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for e := range errCh {
		errs = append(errs, e)
	}
	err := errors.Join(errs...)

	a.cache[kind].Store(cachedResult{err: err, at: now})

	return err
}
