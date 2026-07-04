package healthkit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewAggregator verifies that NewAggregator correctly initializes
// an Aggregator with the specified cacheTTL duration.
func TestNewAggregator(t *testing.T) {
	tests := []struct {
		name     string
		cacheTTL time.Duration
		wantNil  bool
	}{
		{
			name:     "zero TTL",
			cacheTTL: 0,
			wantNil:  false,
		},
		{
			name:     "positive TTL",
			cacheTTL: 5 * time.Second,
			wantNil:  false,
		},
		{
			name:     "negative TTL",
			cacheTTL: -5 * time.Second,
			wantNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewAggregator(tt.cacheTTL)
			if (got == nil) != tt.wantNil {
				t.Errorf("NewAggregator() = %v, wantNil %v", got, tt.wantNil)
			}
			if got != nil && got.cacheTTL != tt.cacheTTL {
				t.Errorf("NewAggregator().cacheTTL = %v, want %v", got.cacheTTL, tt.cacheTTL)
			}
			if got != nil {
				if got.checks == nil {
					t.Error("NewAggregator().checks should not be nil")
				}
				if len(got.checks) != 0 {
					t.Errorf("NewAggregator().checks should be empty, got %d items", len(got.checks))
				}
			}
		})
	}
}

// TestAggregator_Register verifies that Register correctly adds
// health checks to the aggregator and applies default timeout when needed.
func TestAggregator_Register(t *testing.T) {
	tests := []struct {
		name     string
		checks   []Check
		wantLen  int
		withKind Kind
	}{
		{
			name:     "single liveness check",
			checks:   []Check{{Name: "test", Kind: Liveness, Fn: func(ctx context.Context) error { return nil }}},
			wantLen:  1,
			withKind: Liveness,
		},
		{
			name: "multiple readiness checks",
			checks: []Check{
				{Name: "test1", Kind: Readiness, Fn: func(ctx context.Context) error { return nil }},
				{Name: "test2", Kind: Readiness, Fn: func(ctx context.Context) error { return nil }},
			},
			wantLen:  2,
			withKind: Readiness,
		},
		{
			name: "mixed kinds",
			checks: []Check{
				{Name: "liveness", Kind: Liveness, Fn: func(ctx context.Context) error { return nil }},
				{Name: "readiness", Kind: Readiness, Fn: func(ctx context.Context) error { return nil }},
				{Name: "startup", Kind: Startup, Fn: func(ctx context.Context) error { return nil }},
			},
			wantLen: 1,
			withKind: Liveness,
		},
		{
			name:     "default timeout applied",
			checks:   []Check{{Name: "no-timeout", Kind: Liveness, Fn: func(ctx context.Context) error { return nil }}},
			wantLen:  1,
			withKind: Liveness,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAggregator(0)
			a.Register(tt.checks...)

			a.mu.RLock()
			got := len(a.checks[tt.withKind])
			a.mu.RUnlock()

			if got != tt.wantLen {
				t.Errorf("Register() %v checks count = %v, want %v", tt.withKind, got, tt.wantLen)
			}

			// Verify default timeout is set
			if tt.name == "default timeout applied" {
				a.mu.RLock()
				check := a.checks[tt.withKind][0]
				a.mu.RUnlock()
				if check.Timeout != 300*time.Millisecond {
					t.Errorf("Register() default timeout = %v, want %v", check.Timeout, 300*time.Millisecond)
				}
			}
		})
	}
}

func TestAggregator_evaluate_Success(t *testing.T) {
	a := NewAggregator(0)
	a.Register(
		Check{Name: "check1", Kind: Liveness, Fn: func(ctx context.Context) error { return nil }},
		Check{Name: "check2", Kind: Liveness, Fn: func(ctx context.Context) error { return nil }},
	)

	ctx := context.Background()
	err := a.evaluate(ctx, Liveness)
	if err != nil {
		t.Errorf("evaluate() = %v, want nil", err)
	}
}

func TestAggregator_evaluate_Failure(t *testing.T) {
	a := NewAggregator(0)
	a.Register(
		Check{Name: "check1", Kind: Liveness, Fn: func(ctx context.Context) error { return fmt.Errorf("failed1") }},
		Check{Name: "check2", Kind: Liveness, Fn: func(ctx context.Context) error { return fmt.Errorf("failed2") }},
	)

	ctx := context.Background()
	err := a.evaluate(ctx, Liveness)
	if err == nil {
		t.Error("evaluate() = nil, want error")
	} else {
		// Both error messages should be present
		if !strings.Contains(err.Error(), "check1:") {
			t.Errorf("error should contain 'check1:', got %v", err)
		}
		if !strings.Contains(err.Error(), "check2:") {
			t.Errorf("error should contain 'check2:', got %v", err)
		}
	}
}

func TestAggregator_evaluate_Timeout(t *testing.T) {
	a := NewAggregator(0)
	a.Register(
		Check{
			Name:    "slow-check",
			Kind:    Liveness,
			Timeout: 10 * time.Millisecond,
			Fn: func(ctx context.Context) error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
					return nil
				}
			},
		},
	)

	ctx := context.Background()
	err := a.evaluate(ctx, Liveness)
	if err == nil {
		t.Error("evaluate() should timeout and return error")
	}
}

func TestAggregator_evaluate_Caching(t *testing.T) {
	callCount := 0
	a := NewAggregator(100 * time.Millisecond)
	a.Register(
		Check{
			Name: "cached-check",
			Kind: Liveness,
			Fn: func(ctx context.Context) error {
				callCount++
				return nil
			},
		},
	)

	ctx := context.Background()

	// First call
	err := a.evaluate(ctx, Liveness)
	if err != nil {
		t.Fatalf("first evaluate() = %v, want nil", err)
	}
	firstCount := callCount

	// Second call within cache TTL
	err = a.evaluate(ctx, Liveness)
	if err != nil {
		t.Fatalf("second evaluate() = %v, want nil", err)
	}
	secondCount := callCount

	if secondCount != firstCount {
		t.Errorf("cached evaluate() called check %d times, want %d", secondCount, firstCount)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Third call after cache expiry
	err = a.evaluate(ctx, Liveness)
	if err != nil {
		t.Fatalf("third evaluate() = %v, want nil", err)
	}
	thirdCount := callCount

	if thirdCount != firstCount+1 {
		t.Errorf("post-cache evaluate() called check %d times, want %d", thirdCount, firstCount+1)
	}
}

func TestAggregator_AllKinds(t *testing.T) {
	a := NewAggregator(0)

	for _, kind := range []Kind{Liveness, Readiness, Startup} {
		t.Run(kind.String(), func(t *testing.T) {
			a.Register(
				Check{Name: kind.String(), Kind: kind, Fn: func(ctx context.Context) error { return nil }},
			)

			ctx := context.Background()
			err := a.evaluate(ctx, kind)
			if err != nil {
				t.Errorf("evaluate(%v) = %v, want nil", kind, err)
			}
		})
	}
}

func TestKind_String(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{Liveness, "liveness"},
		{Readiness, "readiness"},
		{Startup, "startup"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("Kind.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAggregator_Handler_Success(t *testing.T) {
	a := NewAggregator(0)
	a.Register(
		Check{Name: "ok", Kind: Liveness, Fn: func(ctx context.Context) error { return nil }},
	)

	handler := a.Handler(Liveness)
	req := httptest.NewRequest("GET", "/health/liveness", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Handler() status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if body != "ok" {
		t.Errorf("Handler() body = %q, want 'ok'", body)
	}
}

func TestAggregator_Handler_Failure(t *testing.T) {
	a := NewAggregator(0)
	a.Register(
		Check{Name: "bad", Kind: Liveness, Fn: func(ctx context.Context) error { return fmt.Errorf("failed") }},
	)

	handler := a.Handler(Liveness)
	req := httptest.NewRequest("GET", "/health/liveness", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Handler() status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "bad:") {
		t.Errorf("Handler() body should contain error, got %q", body)
	}
}
