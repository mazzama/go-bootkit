package cachekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/mazzama/go-bootkit/core/retry"
)

// Codec defines the interface for cache value serialization and deserialization.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSONCodec is the default Codec implementation using encoding/json.
type JSONCodec struct{}

// Cache is the interface for generic cache operations. RedisCache satisfies it;
// for tests, use memcache.New() from the cachekit/memcache sub-package.
type Cache interface {
	// Get retrieves a cached value by key and unmarshals it into dest.
	// On a cache miss, Get returns an error satisfying errors.Is(err, ErrCacheMiss).
	Get(ctx context.Context, key string, dest any) error
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// RedisCache is a redis-backed Cache and core.Component. It connects lazily
// during Start (with optional retry) and reports readiness via Lifecycle.
type RedisCache struct {
	core.Lifecycle

	client        *redis.Client
	options       *redis.Options
	codec         Codec
	name          string
	logger        *slog.Logger
	retryAttempts int
	retryBackoff  time.Duration
}

// RedisOption configures a RedisCache at construction time.
type RedisOption func(*RedisCache)

// DefaultCodec is the default JSON codec used by RedisCache and MemoryCache.
var DefaultCodec Codec = JSONCodec{}

// ErrCacheMiss is returned by Cache implementations when a requested key is not found.
var ErrCacheMiss = errors.New("cache miss")

var (
	_ core.Component = (*RedisCache)(nil)
	_ core.Readyable = (*RedisCache)(nil)
	_ Cache          = (*RedisCache)(nil)
)

// Marshal serializes v to JSON.
func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal deserializes JSON data into v.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// NewRedisCache creates a RedisCache for the given address. Connect retries
// and other behavior are configurable via RedisOption values.
func NewRedisCache(addr string, options ...RedisOption) (*RedisCache, error) {
	if addr == "" {
		return nil, fmt.Errorf("redis address cannot be empty")
	}

	cache := &RedisCache{
		name: "redis-cache",
		options: &redis.Options{
			Addr:     addr,
			Password: "",
			DB:       0,
		},
	}

	for _, option := range options {
		option(cache)
	}

	cache.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		var client *redis.Client

		attempts := cache.retryAttempts
		if attempts <= 0 {
			attempts = 1
		}

		err := retry.Do(ctx, attempts, cache.retryBackoff, func(ctx context.Context) error {
			c := redis.NewClient(cache.options)
			if pingErr := c.Ping(ctx).Err(); pingErr != nil {
				_ = c.Close()
				return pingErr
			}
			client = c
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to connect to Redis: %w", err)
		}

		cache.client = client

		return func(_ context.Context) error {
			return cache.client.Close()
		}, nil
	})

	return cache, nil
}

// WithName sets the component name reported by Name and health checks.
func WithName(name string) RedisOption {
	return func(r *RedisCache) {
		r.name = name
	}
}

// WithLogger sets the cache's logger. Currently reserved for diagnostics.
func WithLogger(logger *slog.Logger) RedisOption {
	return func(r *RedisCache) {
		r.logger = logger
	}
}

// WithConnectRetry sets how many connection attempts (with backoff) Start
// performs before failing. Both values must be positive to take effect.
func WithConnectRetry(maxAttempts int, backoff time.Duration) RedisOption {
	return func(r *RedisCache) {
		if maxAttempts > 0 && backoff > 0 {
			r.retryAttempts = maxAttempts
			r.retryBackoff = backoff
		}
	}
}

// WithPassword sets the Redis AUTH password.
func WithPassword(password string) RedisOption {
	return func(r *RedisCache) {
		r.options.Password = password
	}
}

// WithDB selects the Redis logical database number.
func WithDB(db int) RedisOption {
	return func(r *RedisCache) {
		r.options.DB = db
	}
}

// WithUsername sets the Redis ACL username.
func WithUsername(username string) RedisOption {
	return func(r *RedisCache) {
		r.options.Username = username
	}
}

// WithCodec replaces the default JSON codec used to serialize values.
func WithCodec(codec Codec) RedisOption {
	return func(r *RedisCache) {
		if codec != nil {
			r.codec = codec
		}
	}
}

// Name returns the component name.
func (r *RedisCache) Name() string {
	return r.name
}

func (r *RedisCache) codecOrDefault() Codec {
	if r.codec != nil {
		return r.codec
	}
	return DefaultCodec
}

// Set stores the value under key, serialized with the configured codec.
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	b, err := r.codecOrDefault().Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, b, expiration).Err()
}

// Get retrieves the value at key and unmarshals it into dest. A cache miss
// returns an error satisfying errors.Is(err, ErrCacheMiss).
func (r *RedisCache) Get(ctx context.Context, key string, dest any) error {
	str, err := r.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return fmt.Errorf("%s: %w", key, ErrCacheMiss)
		}
		return err
	}
	return r.codecOrDefault().Unmarshal([]byte(str), dest)
}

// Delete removes the key from the cache.
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// Exists reports whether key is present in the cache.
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, key).Result()
	return result > 0, err
}

// HealthChecks returns the standard liveness/readiness pair; readiness pings
// Redis.
func (r *RedisCache) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(r.name, func(ctx context.Context) error {
		client := r.client
		if client == nil {
			return fmt.Errorf("redis client is not initialized")
		}
		return client.Ping(ctx).Err()
	})
}
