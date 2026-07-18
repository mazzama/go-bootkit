package healthkit

import (
	"context"
	"errors"
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
}

type Aggregator struct {
	cacheErr [3]atomic.Value
	checks   map[Kind][]Check
	cacheAt  [3]int64
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
	last := time.Unix(0, atomic.LoadInt64(&a.cacheAt[kind]))

	if a.cacheTTL > 0 && now.Sub(last) < a.cacheTTL {
		if v := a.cacheErr[kind].Load(); v != nil {
			if res, ok := v.(cachedResult); ok {
				return res.err
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
				errCh <- errors.New(check.Name + ": " + err.Error())
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

	a.cacheErr[kind].Store(cachedResult{err: err})

	atomic.StoreInt64(&a.cacheAt[kind], now.UnixNano())

	return err
}
