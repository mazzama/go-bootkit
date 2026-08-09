package retry_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core/retry"
)

var errBoom = errors.New("boom")

func TestDoSucceedsFirstAttempt(t *testing.T) {
	var calls atomic.Int32
	err := retry.Do(context.Background(), 3, 10*time.Millisecond, func(ctx context.Context) error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", calls.Load())
	}
}

func TestDoSucceedsAfterRetries(t *testing.T) {
	var calls atomic.Int32
	err := retry.Do(context.Background(), 3, 10*time.Millisecond, func(ctx context.Context) error {
		if calls.Add(1) < 3 {
			return errBoom
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestDoExhaustsAttempts(t *testing.T) {
	var calls atomic.Int32
	err := retry.Do(context.Background(), 3, 10*time.Millisecond, func(ctx context.Context) error {
		calls.Add(1)
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected errBoom, got %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", calls.Load())
	}
}

func TestDoContextCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- retry.Do(ctx, 5, 100*time.Millisecond, func(ctx context.Context) error {
			calls.Add(1)
			return errBoom
		})
	}()

	// Wait for first call to fail and backoff to start
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls.Load() < 1 {
		t.Fatalf("expected at least 1 call, got %d", calls.Load())
	}
}

func TestDoContextCancelBeforeFirstCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var calls atomic.Int32
	err := retry.Do(ctx, 3, 10*time.Millisecond, func(ctx context.Context) error {
		calls.Add(1)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("expected 0 calls, got %d", calls.Load())
	}
}

func TestDoMaxAttemptsZero(t *testing.T) {
	var calls atomic.Int32
	err := retry.Do(context.Background(), 0, 10*time.Millisecond, func(ctx context.Context) error {
		calls.Add(1)
		return errBoom
	})
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if calls.Load() != 0 {
		t.Fatalf("expected 0 calls, got %d", calls.Load())
	}
}

func TestDoMaxAttemptsOne(t *testing.T) {
	var calls atomic.Int32
	err := retry.Do(context.Background(), 1, 10*time.Millisecond, func(ctx context.Context) error {
		calls.Add(1)
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected errBoom, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", calls.Load())
	}
}

func TestDoWrapsLastError(t *testing.T) {
	sentinel := fmt.Errorf("wrapped: %w", errBoom)
	var calls atomic.Int32
	err := retry.Do(context.Background(), 2, 10*time.Millisecond, func(ctx context.Context) error {
		if calls.Add(1) == 2 {
			return sentinel
		}
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected error wrapping errBoom, got %v", err)
	}
	if !errors.Is(err, sentinel) {
		// the last error IS sentinel, so it should be found directly
		t.Fatalf("expected sentinel to be retrievable, got %v", err)
	}
}
