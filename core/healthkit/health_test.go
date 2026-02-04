package healthkit

import (
	"testing"
	"time"
)

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
		})
	}
}
