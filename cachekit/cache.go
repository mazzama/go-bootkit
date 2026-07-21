package cachekit

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	core.Lifecycle

	client  *redis.Client
	options *redis.Options
	name    string
	logger  *slog.Logger
}

type RedisOption func(*RedisCache)

func NewRedisCache(options ...RedisOption) (*RedisCache, error) {
	cache := &RedisCache{
		name: "redis-cache",
		options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
	}

	for _, option := range options {
		option(cache)
	}

	if cache.options.Addr == "" {
		return nil, fmt.Errorf("redis address cannot be empty")
	}

	cache.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		client := redis.NewClient(cache.options)

		if err := client.Ping(ctx).Err(); err != nil {
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

func WithAddress(addr string) RedisOption {
	return func(r *RedisCache) {
		r.options.Addr = addr
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

func (r *RedisCache) Name() string {
	return r.name
}

func (r *RedisCache) Client() *redis.Client {
	return r.client
}

func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.Client().Set(ctx, key, value, expiration).Err()
}

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return r.Client().Get(ctx, key).Result()
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
