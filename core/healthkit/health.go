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
	at  int64
}

type Aggregator struct {
	cache    [3]atomic.Value
	checks   map[Kind][]Check
	cacheTTL time.Duration
	mu       sync.RWMutex
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
				last := time.Unix(0, res.at)
				if now.Sub(last) < a.cacheTTL {
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

	a.cache[kind].Store(cachedResult{err: err, at: now.UnixNano()})

	return err
}
