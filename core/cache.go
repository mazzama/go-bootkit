package core

import (
	"context"
	"time"
)

// Cache defines the generic interface for key-value stores.
// By depending on this interface instead of a specific implementation (e.g. RedisCache),
// components can remain decoupled from the underlying infrastructure.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}
