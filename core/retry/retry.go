// Package retry provides an exponential-backoff retry loop shared by
// infrastructure adapters (databasekit, cachekit) that need to tolerate
// transient backend unavailability during startup.
package retry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

// Do calls fn up to maxAttempts times with exponential backoff between failures.
// On success (fn returns nil), Do returns nil immediately.
// On failure, it sleeps with backoff and jitter before retrying.
// The sleep is interruptible by ctx.Done() — if the context is cancelled
// during backoff, Do returns ctx.Err().
// If maxAttempts is zero or negative, Do returns an error without calling fn.
func Do(ctx context.Context, maxAttempts int, baseBackoff time.Duration, fn func(ctx context.Context) error) error {
	if maxAttempts <= 0 {
		return fmt.Errorf("retry: maxAttempts must be positive, got %d", maxAttempts)
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Check context before each attempt.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		// Last attempt failed — return the error.
		if attempt == maxAttempts-1 {
			return err
		}

		// Compute backoff with jitter.
		backoff := baseBackoff * time.Duration(1<<attempt)
		jitter := time.Duration(0)
		if half := int64(baseBackoff / 2); half > 0 {
			jitter = time.Duration(rand.Int64N(half))
		}

		timer := time.NewTimer(backoff + jitter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}

	// Unreachable — the loop always returns inside.
	return nil
}
