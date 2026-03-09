package healthkit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAggregator(t *testing.T) {
	agg := NewAggregator(5 * time.Second)
	if agg == nil {
		t.Fatal("expected non-nil Aggregator")
	}
	if agg.cacheTTL != 5*time.Second {
		t.Fatalf("expected cacheTTL=5s, got %v", agg.cacheTTL)
	}
}

func TestRegisterSetsDefaultTimeout(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(Check{
		Name: "no-timeout",
		Kind: Liveness,
		Fn:   func(ctx context.Context) error { return nil },
	})

	checks := agg.checks[Liveness]
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Timeout != 300*time.Millisecond {
		t.Fatalf("expected default timeout 300ms, got %v", checks[0].Timeout)
	}
}

func TestRegisterPreservesCustomTimeout(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(Check{
		Name:    "custom-timeout",
		Kind:    Liveness,
		Timeout: 2 * time.Second,
		Fn:      func(ctx context.Context) error { return nil },
	})

	checks := agg.checks[Liveness]
	if len(checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(checks))
	}
	if checks[0].Timeout != 2*time.Second {
		t.Fatalf("expected timeout 2s, got %v", checks[0].Timeout)
	}
}

func TestHandlerReturns200WhenAllChecksPass(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(Check{
		Name: "ok-check",
		Kind: Liveness,
		Fn:   func(ctx context.Context) error { return nil },
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	agg.Handler(Liveness).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("expected body \"ok\", got %q", body)
	}
}

func TestHandlerReturns503WhenCheckFails(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(Check{
		Name: "fail-check",
		Kind: Readiness,
		Fn:   func(ctx context.Context) error { return errors.New("db down") },
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	agg.Handler(Readiness).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !containsAll(body, "fail-check", "db down") {
		t.Fatalf("expected body to contain check name and error message, got %q", body)
	}
}

func TestHandlerNoChecksReturns200(t *testing.T) {
	agg := NewAggregator(0)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/startupz", nil)
	agg.Handler(Startup).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != "ok" {
		t.Fatalf("expected body \"ok\", got %q", body)
	}
}

func TestEvaluateCachesResults(t *testing.T) {
	var callCount int32
	agg := NewAggregator(1 * time.Second)
	agg.Register(Check{
		Name: "counted-check",
		Kind: Liveness,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&callCount, 1)
			return nil
		},
	})

	handler := agg.Handler(Liveness)

	// First call — should invoke Fn.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec1, req1)

	// Second call within the cache TTL — should NOT invoke Fn again.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec2, req2)

	count := atomic.LoadInt32(&callCount)
	if count != 1 {
		t.Fatalf("expected Fn called once due to caching, got %d", count)
	}
}

func TestEvaluateMultipleChecksConcurrent(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(
		Check{
			Name: "pass-check",
			Kind: Liveness,
			Fn:   func(ctx context.Context) error { return nil },
		},
		Check{
			Name: "fail-check",
			Kind: Liveness,
			Fn:   func(ctx context.Context) error { return errors.New("boom") },
		},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	agg.Handler(Liveness).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 when one check fails, got %d", rec.Code)
	}
}

func TestRegisterMultipleKinds(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(
		Check{
			Name: "live-ok",
			Kind: Liveness,
			Fn:   func(ctx context.Context) error { return nil },
		},
		Check{
			Name: "ready-fail",
			Kind: Readiness,
			Fn:   func(ctx context.Context) error { return errors.New("not ready") },
		},
	)

	// Liveness should pass.
	recLive := httptest.NewRecorder()
	reqLive := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	agg.Handler(Liveness).ServeHTTP(recLive, reqLive)
	if recLive.Code != http.StatusOK {
		t.Fatalf("liveness: expected 200, got %d", recLive.Code)
	}

	// Readiness should fail.
	recReady := httptest.NewRecorder()
	reqReady := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	agg.Handler(Readiness).ServeHTTP(recReady, reqReady)
	if recReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness: expected 503, got %d", recReady.Code)
	}
}

func TestCacheExpiresAfterTTL(t *testing.T) {
	var callCount int32
	agg := NewAggregator(50 * time.Millisecond)
	agg.Register(Check{
		Name: "expiry-check",
		Kind: Liveness,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&callCount, 1)
			return nil
		},
	})

	handler := agg.Handler(Liveness)

	// First call — invokes Fn.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec1, req1)

	// Wait for the cache to expire.
	time.Sleep(100 * time.Millisecond)

	// Second call — cache expired, should invoke Fn again.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec2, req2)

	count := atomic.LoadInt32(&callCount)
	if count != 2 {
		t.Fatalf("expected Fn called twice after cache expiry, got %d", count)
	}
}

func TestSlowCheckCancelledByTimeout(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(Check{
		Name:    "slow-check",
		Kind:    Liveness,
		Timeout: 50 * time.Millisecond,
		Fn: func(ctx context.Context) error {
			select {
			case <-time.After(5 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	agg.Handler(Liveness).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 for timed-out check, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !containsAll(body, "slow-check") {
		t.Fatalf("expected body to contain check name, got %q", body)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
