package cachekit

import (
	"context"
	"time"
)

// Get is a type-safe generic helper for retrieving a value from the cache.
// It relies on strict value types. Developers MUST pass value types (e.g., Get[User]).
// Passing a pointer type (e.g., Get[*User]) will cause underlying strict codecs
// or unmarshalers to fail. We intentionally avoid reflection overhead here.
func Get[T any](ctx context.Context, c Cache, key string) (T, error) {
	var zero T
	err := c.Get(ctx, key, &zero)
	return zero, err
}

// Set is a type-safe generic helper for setting a value in the cache.
func Set[T any](ctx context.Context, c Cache, key string, value T, ttl time.Duration) error {
	return c.Set(ctx, key, value, ttl)
}
