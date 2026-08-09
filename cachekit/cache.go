package cachekit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
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

// Cache is the interface for generic cache operations. RedisCache satisfies it;
// for tests, use memcache.New() from the cachekit/memcache sub-package.
type Cache interface {
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
		var err error

		if cache.retryAttempts > 0 {
			for attempt := 0; attempt < cache.retryAttempts; attempt++ {
				client = redis.NewClient(cache.options)
				if err = client.Ping(ctx).Err(); err == nil {
					break
				}
				_ = client.Close()
				if attempt < cache.retryAttempts-1 {
					jitter := time.Duration(0)
					if half := int64(cache.retryBackoff / 2); half > 0 {
						jitter = time.Duration(rand.Int64N(half))
					}
					backoff := cache.retryBackoff*(1<<attempt) + jitter

					timer := time.NewTimer(backoff)
					select {
					case <-ctx.Done():
						timer.Stop()
						return nil, ctx.Err()
					case <-timer.C:
					}
				}
			}
			if err != nil {
				return nil, fmt.Errorf("failed to connect to Redis after retries: %w", err)
			}
		} else {
			client = redis.NewClient(cache.options)
			if err = client.Ping(ctx).Err(); err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("failed to connect to Redis: %w", err)
			}
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

func (r *RedisCache) Client() *redis.Client {
	return r.client
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
	return r.Client().Set(ctx, key, b, expiration).Err()
}

func (r *RedisCache) Get(ctx context.Context, key string, dest any) error {
	str, err := r.Client().Get(ctx, key).Result()
	if err != nil {
		return err
	}
	return r.codecOrDefault().Unmarshal([]byte(str), dest)
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.Client().Del(ctx, key).Err()
}

func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.Client().Exists(ctx, key).Result()
	return result > 0, err
}

func (r *RedisCache) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(r.name, func(ctx context.Context) error {
		client := r.Client()
		if client == nil {
			return fmt.Errorf("redis client is not initialized")
		}
		return client.Ping(ctx).Err()
	})
}

var _ core.Component = (*RedisCache)(nil)
var _ core.Readyable = (*RedisCache)(nil)
var _ Cache = (*RedisCache)(nil)
