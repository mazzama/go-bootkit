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
	Name    string
	Kind    Kind
	Timeout time.Duration
	Fn      func(ctx context.Context) error
}

type Aggregator struct {
	mu     sync.RWMutex
	checks map[Kind][]Check

	cacheTTL time.Duration
	cacheAt  [3]int64
	cacheErr [3]atomic.Value
}

func NewAggregator(cacheTTL time.Duration) *Aggregator {
	return &Aggregator{
		checks:   make(map[Kind][]Check),
		cacheTTL: cacheTTL,
	}
}

func (a *Aggregator) Register(c Check) {
	if c.Timeout <= 0 {
		c.Timeout = 300 * time.Millisecond
	}
	a.mu.Lock()
	a.checks[c.Kind] = append(a.checks[c.Kind], c)
	a.mu.Unlock()
}

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
			if err, _ := v.(error); err != nil {
				return err
			}
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

	if err != nil {
		a.cacheErr[kind].Store(err)
	}

	atomic.StoreInt64(&a.cacheAt[kind], now.UnixNano())

	return err
}
