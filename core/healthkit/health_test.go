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

func TestStandardChecksShape(t *testing.T) {
	checks := StandardChecks("test-db", func(ctx context.Context) error {
		return nil
	})

	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	if checks[0].Name != "test-db-liveness" {
		t.Errorf("expected 'test-db-liveness', got %q", checks[0].Name)
	}
	if checks[0].Kind != Liveness {
		t.Errorf("expected Liveness kind for check[0]")
	}
	if checks[0].Timeout != 0 {
		t.Errorf("expected liveness Timeout=0, got %v", checks[0].Timeout)
	}

	if checks[1].Name != "test-db-readiness" {
		t.Errorf("expected 'test-db-readiness', got %q", checks[1].Name)
	}
	if checks[1].Kind != Readiness {
		t.Errorf("expected Readiness kind for check[1]")
	}
	if checks[1].Timeout != 2*time.Second {
		t.Errorf("expected readiness Timeout=2s, got %v", checks[1].Timeout)
	}
}

func TestStandardChecksLivenessIsNop(t *testing.T) {
	checks := StandardChecks("svc", func(ctx context.Context) error {
		return errors.New("readiness should not be invoked here")
	})

	if err := checks[0].Fn(context.Background()); err != nil {
		t.Fatalf("expected nop liveness to return nil, got %v", err)
	}
}

func TestStandardChecksReadinessInvokesSuppliedClosure(t *testing.T) {
	var called atomic.Bool
	sentinel := errors.New("backend down")

	checks := StandardChecks("svc", func(ctx context.Context) error {
		called.Store(true)
		return sentinel
	})

	err := checks[1].Fn(context.Background())
	if !called.Load() {
		t.Fatal("expected readiness closure to be invoked")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected readiness closure error to propagate, got %v", err)
	}
}

func TestNewAggregator(t *testing.T) {
	agg := NewAggregator(5 * time.Second)
	if agg == nil {
		t.Fatal("expected non-nil Aggregator")
	}
	if agg.cacheTTL != 5*time.Second {
		t.Fatalf("expected cacheTTL=5s, got %v", agg.cacheTTL)
	}
}

func TestNewAggregatorZeroAppliesFloor(t *testing.T) {
	agg := NewAggregator(0)
	if agg.cacheTTL != 1*time.Second {
		t.Fatalf("expected 0 to apply a 1s floor, got %v", agg.cacheTTL)
	}
}

func TestNewAggregatorNegativeDisablesCache(t *testing.T) {
	agg := NewAggregator(-1)
	if agg.cacheTTL != 0 {
		t.Fatalf("expected negative TTL to disable caching (0), got %v", agg.cacheTTL)
	}
}

func TestNewAggregatorPositiveHonored(t *testing.T) {
	agg := NewAggregator(250 * time.Millisecond)
	if agg.cacheTTL != 250*time.Millisecond {
		t.Fatalf("expected positive TTL honored unchanged, got %v", agg.cacheTTL)
	}
}

func TestNewAggregatorNegativeEvaluatesLive(t *testing.T) {
	var callCount int32
	agg := NewAggregator(-1)
	agg.Register(Check{
		Name: "live-check",
		Kind: Liveness,
		Fn: func(ctx context.Context) error {
			atomic.AddInt32(&callCount, 1)
			return nil
		},
	})

	handler := agg.Handler(Liveness)
	for range 3 {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		handler.ServeHTTP(rec, req)
	}

	if count := atomic.LoadInt32(&callCount); count != 3 {
		t.Fatalf("expected Fn called live on every probe (3), got %d", count)
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

func TestCacheRecovery(t *testing.T) {
	var shouldFail int32 = 1
	agg := NewAggregator(50 * time.Millisecond)
	agg.Register(Check{
		Name: "recovery-check",
		Kind: Liveness,
		Fn: func(ctx context.Context) error {
			if atomic.LoadInt32(&shouldFail) == 1 {
				return errors.New("temporary failure")
			}
			return nil
		},
	})

	handler := agg.Handler(Liveness)

	// 1. Initial check - fails
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec1.Code)
	}

	// 2. Recovery happens
	atomic.StoreInt32(&shouldFail, 0)

	// 3. Check within TTL - should still return cached 503
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected cached status 503 within TTL, got %d", rec2.Code)
	}

	// 4. Wait for TTL to expire
	time.Sleep(100 * time.Millisecond)

	// 5. Check after TTL - should run again, succeed, and return 200 OK
	rec3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected status 200 after recovery and TTL expiry, got %d", rec3.Code)
	}

	// 6. Check again within TTL - should return cached 200 OK
	rec4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected cached status 200 within TTL, got %d", rec4.Code)
	}
}

func TestErrorsJoin(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(
		Check{
			Name: "check-1",
			Kind: Liveness,
			Fn:   func(ctx context.Context) error { return errors.New("err-1") },
		},
		Check{
			Name: "check-2",
			Kind: Liveness,
			Fn:   func(ctx context.Context) error { return errors.New("err-2") },
		},
	)

	err := agg.evaluate(context.Background(), Liveness)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	// Verify we can find individual errors using errors.Unwrap/errors.As via the Join interface
	var errList []error
	if joinErr, ok := err.(interface{ Unwrap() []error }); ok {
		errList = joinErr.Unwrap()
	}

	if len(errList) != 2 {
		t.Fatalf("expected 2 wrapped errors, got %v", len(errList))
	}
}

func TestHealthHookIsCalled(t *testing.T) {
	agg := NewAggregator(0)
	agg.Register(Check{
		Name: "hook-check",
		Kind: Liveness,
		Fn:   func(ctx context.Context) error { return errors.New("boom") },
	})

	var called bool
	var calledKind Kind
	var calledErr error
	var calledDur time.Duration
	agg.SetHook(func(kind Kind, duration time.Duration, err error) {
		called = true
		calledKind = kind
		calledErr = err
		calledDur = duration
	})

	_ = agg.evaluate(context.Background(), Liveness)

	if !called {
		t.Fatal("expected hook to be called")
	}
	if calledKind != Liveness {
		t.Errorf("expected Liveness, got %v", calledKind)
	}
	if calledErr == nil || !strings.Contains(calledErr.Error(), "boom") {
		t.Errorf("expected boom error, got %v", calledErr)
	}
	if calledDur <= 0 {
		t.Errorf("expected positive duration, got %v", calledDur)
	}
}
