package cachekit

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client    *redis.Client
	options   *redis.Options
	readyChan chan struct{}
	name      string
	logger    *slog.Logger
	mu        sync.RWMutex
}

type RedisOption func(*RedisCache)

func NewRedisCache(options ...RedisOption) *RedisCache {
	cache := &RedisCache{
		name:      "redis-cache",
		readyChan: make(chan struct{}),
		options: &redis.Options{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
	}

	for _, option := range options {
		option(cache)
	}

	return cache
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

func (r *RedisCache) Start(ctx context.Context) error {
	client := redis.NewClient(r.options)

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	r.mu.Lock()
	r.client = client
	r.mu.Unlock()

	close(r.readyChan)
	<-ctx.Done()
	return nil
}

func (r *RedisCache) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

func (r *RedisCache) Ready() <-chan struct{} {
	return r.readyChan
}

func (r *RedisCache) Client() *redis.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
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
	return []healthkit.Check{
		{
			Name: r.name + "-liveness",
			Kind: healthkit.Liveness,
			Fn: func(ctx context.Context) error {
				return nil
			},
		},
		{
			Name:    r.name + "-readiness",
			Kind:    healthkit.Readiness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				client := r.Client()
				if client == nil {
					return fmt.Errorf("redis client is not initialized")
				}
				return client.Ping(ctx).Err()
			},
		},
	}
}

var _ core.Component = (*RedisCache)(nil)
var _ core.Readyable = (*RedisCache)(nil)
