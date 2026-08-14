package cachekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/mazzama/go-bootkit/core/retry"
	"github.com/redis/go-redis/v9"
)

// Codec defines the interface for cache value serialization and deserialization.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// JSONCodec is the default Codec implementation using encoding/json.
type JSONCodec struct{}

func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// DefaultCodec is the default JSON codec used by RedisCache and MemoryCache.
var DefaultCodec Codec = JSONCodec{}

// ErrCacheMiss is returned by Cache implementations when a requested key is not found.
var ErrCacheMiss = errors.New("cache miss")

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
type RedisOption func(*RedisCache)

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

func WithName(name string) RedisOption {
	return func(r *RedisCache) {
		r.name = name
	}
}

func WithLogger(logger *slog.Logger) RedisOption {
	return func(r *RedisCache) {
		r.logger = logger
	}
}

func WithConnectRetry(maxAttempts int, backoff time.Duration) RedisOption {
	return func(r *RedisCache) {
		if maxAttempts > 0 && backoff > 0 {
			r.retryAttempts = maxAttempts
			r.retryBackoff = backoff
		}
	}
}

func WithPassword(password string) RedisOption {
	return func(r *RedisCache) {
		r.options.Password = password
	}
}

func WithDB(db int) RedisOption {
	return func(r *RedisCache) {
		r.options.DB = db
	}
}

func WithUsername(username string) RedisOption {
	return func(r *RedisCache) {
		r.options.Username = username
	}
}

func WithCodec(codec Codec) RedisOption {
	return func(r *RedisCache) {
		if codec != nil {
			r.codec = codec
		}
	}
}

func (r *RedisCache) Name() string {
	return r.name
}

func (r *RedisCache) codecOrDefault() Codec {
	if r.codec != nil {
		return r.codec
	}
	return DefaultCodec
}

func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	b, err := r.codecOrDefault().Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, b, expiration).Err()
}

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

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Exists(ctx, key).Result()
	return result > 0, err
}

func (r *RedisCache) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(r.name, func(ctx context.Context) error {
		client := r.client
		if client == nil {
			return fmt.Errorf("redis client is not initialized")
		}
		return client.Ping(ctx).Err()
	})
}

var _ core.Component = (*RedisCache)(nil)
var _ core.Readyable = (*RedisCache)(nil)
var _ Cache = (*RedisCache)(nil)
