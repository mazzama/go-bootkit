package healthkit

import (
	"context"
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
