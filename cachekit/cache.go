// Package cachekit provides a Redis cache component.
//
// The RedisCache type wraps go-redis to provide a core.Component implementation
// with health checks and lifecycle management. It supports common cache operations
// like Get, Set, Delete, and Exists.
//
// # Features
//
//   - Connection pooling via go-redis
//   - Kubernetes-style health checks (liveness/readiness)
//   - Thread-safe client access
//   - Common cache operations (Get, Set, Delete, Exists)
//
// # Health Checks
//
// The component provides health checks that verify connectivity and readiness:
//   - Liveness: Verifies Redis connectivity by pinging the server
//   - Readiness: Verifies the component has completed startup and is ready
//
// # Connection Options
//
// Redis connection options include address, password, database number, and username.
// These can be configured via functional options.
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithAddress("localhost:6379"),
//	    cachekit.WithPassword("secret"),
//	    cachekit.WithDB(0),
//	    cachekit.WithName("session-cache"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Register health checks
//	health.Register(cache.HealthChecks()...)
//
//	// Use with ApplicationRunner
//	runner := core.NewApplicationRunner(
//	    core.WithServices(cache),
//	)
package cachekit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/redis/go-redis/v9"
)

// RedisCache is a Redis cache component.
//
// The RedisCache wraps a go-redis Client to provide cache operations with
// lifecycle management. It implements the Component and Readyable interfaces
// for integration with ApplicationRunner.
//
// # Lifecycle
//
// On creation (NewRedisCache), the component establishes a connection to Redis
// and verifies connectivity with a ping. The Start method signals readiness
// by closing the ready channel. The Stop method closes the Redis connection.
//
// # Thread Safety
//
// All methods are thread-safe. The Client() method uses an RWMutex to protect
// access to the underlying client reference. The go-redis Client is also
// thread-safe and can be used concurrently.
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithAddress("localhost:6379"),
//	    cachekit.WithName("session-cache"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Set a value
//	err = cache.Set(ctx, "key", "value", 5*time.Minute)
//
//	// Get a value
//	val, err := cache.Get(ctx, "key")
//
//	// Check if key exists
//	exists, err := cache.Exists(ctx, "key")
//
//	// Delete a key
//	err = cache.Delete(ctx, "key")
type RedisCache struct {
	name      string
	client    *redis.Client
	options   *redis.Options
	mu        sync.RWMutex
	readyChan chan struct{}
}

// RedisOption is a functional option for configuring a RedisCache.
//
// Options are applied in the order provided to NewRedisCache. See the With*
// functions for available options.
type RedisOption func(*RedisCache)

// NewRedisCache creates a new Redis cache component.
//
// The component establishes a connection to Redis during creation and verifies
// connectivity with a ping. If the connection cannot be established, an error
// is returned.
//
// Default configuration:
//   - Address: localhost:6379
//   - Password: (empty)
//   - DB: 0
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithAddress("localhost:6379"),
//	    cachekit.WithPassword("secret"),
//	    cachekit.WithDB(0),
//	    cachekit.WithName("session-cache"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
func NewRedisCache(ctx context.Context, options ...RedisOption) (*RedisCache, error) {
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

	cache.client = redis.NewClient(cache.options)

	if err := cache.client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return cache, nil
}

// WithName sets the cache component name.
//
// The name is used for logging, health check names, and component identification.
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithName("session-cache"),
//	)
func WithName(name string) RedisOption {
	return func(r *RedisCache) {
		r.name = name
	}
}

// WithAddress sets the Redis server address.
//
// The address should be in host:port format (e.g., "localhost:6379").
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithAddress("redis.example.com:6379"),
//	)
func WithAddress(addr string) RedisOption {
	return func(r *RedisCache) {
		r.options.Addr = addr
	}
}

// WithPassword sets the Redis authentication password.
//
// Use this for Redis servers that require authentication.
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithPassword("my-secret-password"),
//	)
func WithPassword(password string) RedisOption {
	return func(r *RedisCache) {
		r.options.Password = password
	}
}

// WithDB sets the Redis database number.
//
// Redis supports multiple logical databases (0-15 by default).
// The default database is 0.
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithDB(1), // Use database 1
//	)
func WithDB(db int) RedisOption {
	return func(r *RedisCache) {
		r.options.DB = db
	}
}

// WithUsername sets the Redis ACL username.
//
// Use this for Redis 6+ servers with ACL enabled.
//
// Example:
//
//	cache, err := cachekit.NewRedisCache(ctx,
//	    cachekit.WithUsername("app-user"),
//	    cachekit.WithPassword("app-password"),
//	)
func WithUsername(username string) RedisOption {
	return func(r *RedisCache) {
		r.options.Username = username
	}
}

// Name returns the cache component's identifier.
func (r *RedisCache) Name() string {
	return r.name
}

// Start begins the cache component's operation.
//
// Start closes the ready channel to signal that the component is ready,
// then blocks until the context is canceled.
//
// This method is typically called by ApplicationRunner.
func (r *RedisCache) Start(ctx context.Context) error {
	close(r.readyChan)
	<-ctx.Done()
	return nil
}

// Stop gracefully shuts down the cache component.
//
// Stop closes the Redis client connection. Pending operations may be
// interrupted. The context deadline determines how long to wait.
//
// This method is typically called by ApplicationRunner during shutdown.
func (r *RedisCache) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Ready returns a channel that closes when the cache is ready.
//
// The channel closes when Start is called, signaling that the component
// has completed initialization and is ready to serve requests.
func (r *RedisCache) Ready() <-chan struct{} {
	return r.readyChan
}

// Client returns the underlying redis.Client.
//
// Use this method to access the client directly for operations not covered
// by the component's methods. Access is thread-safe.
//
// Example:
//
//	client := cache.Client()
//	// Use client for advanced operations
//	pipeliner := client.Pipeline()
//	pipeliner.Get(ctx, "key1")
//	pipeliner.Get(ctx, "key2")
//	cmders, err := pipeliner.Exec(ctx)
func (r *RedisCache) Client() *redis.Client {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.client
}

// Set stores a value in the cache with an expiration time.
//
// The expiration time specifies how long the key should persist. Use
// redis.KeepTTL (0) to keep the existing TTL, or a negative value to
// persist the key indefinitely.
//
// Example:
//
//	// Store with 5 minute expiration
//	err := cache.Set(ctx, "session:123", userData, 5*time.Minute)
//
//	// Store indefinitely
//	err := cache.Set(ctx, "config:version", "1.0", -1)
func (r *RedisCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return r.Client().Set(ctx, key, value, expiration).Err()
}

// Get retrieves a value from the cache by key.
//
// Returns redis.Nil error if the key does not exist. Use errors.Is to check:
//
//	val, err := cache.Get(ctx, "key")
//	if err == redis.Nil {
//	    // Key doesn't exist
//	} else if err != nil {
//	    // Other error
//	}
//
// Example:
//
//	val, err := cache.Get(ctx, "session:123")
//	if err == redis.Nil {
//	    fmt.Println("Session not found")
//	} else if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Session data: %s\n", val)
func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return r.Client().Get(ctx, key).Result()
}

// Delete removes a key from the cache.
//
// Returns nil if the key was deleted or didn't exist.
//
// Example:
//
//	if err := cache.Delete(ctx, "session:123"); err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println("Session deleted")
func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.Client().Del(ctx, key).Err()
}

// Exists checks if a key exists in the cache.
//
// Returns true if the key exists, false if it doesn't or on error.
//
// Example:
//
//	exists, err := cache.Exists(ctx, "session:123")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	if exists {
//	    fmt.Println("Session exists")
//	} else {
//	    fmt.Println("Session not found")
//	}
func (r *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := r.Client().Exists(ctx, key).Result()
	return result > 0, err
}

// HealthChecks returns health checks for the cache component.
//
// The returned checks include liveness and readiness probes:
//   - Liveness: Verifies Redis connectivity by pinging the server
//   - Readiness: Verifies the component has signaled ready via the Ready() channel
//
// These checks should be registered with a healthkit.Aggregator:
//
//	health.Register(cache.HealthChecks()...)
//
// Example:
//
//	health := healthkit.NewAggregator(5 * time.Second)
//	health.Register(cache.HealthChecks()...)
//	router.Get("/health/liveness", health.Handler(healthkit.Liveness))
func (r *RedisCache) HealthChecks() []healthkit.Check {
	return []healthkit.Check{
		{
			Name:    r.name + "-liveness",
			Kind:    healthkit.Liveness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				return r.client.Ping(ctx).Err()
			},
		},
		{
			Name:    r.name + "-readiness",
			Kind:    healthkit.Readiness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				if r.client == nil {
					return fmt.Errorf("redis client is not initialized")
				}
				select {
				case <-r.Ready():
					return nil
				case <-ctx.Done():
					return ctx.Err()
				default:
					return fmt.Errorf("redis is not ready")
				}
			},
		},
	}
}

var _ core.Component = (*RedisCache)(nil)
var _ core.Readyable = (*RedisCache)(nil)
