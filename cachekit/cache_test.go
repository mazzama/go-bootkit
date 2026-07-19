package cachekit

import (
	"testing"

	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/redis/go-redis/v9"
)

func TestWithName(t *testing.T) {
	cache := &RedisCache{name: "default"}
	WithName("custom")(cache)
	if cache.name != "custom" {
		t.Errorf("expected 'custom', got %q", cache.name)
	}
}

func TestWithAddress(t *testing.T) {
	cache := &RedisCache{options: defaultOptions()}
	WithAddress("redis:6380")(cache)
	if cache.options.Addr != "redis:6380" {
		t.Errorf("expected 'redis:6380', got %q", cache.options.Addr)
	}
}

func TestWithPassword(t *testing.T) {
	cache := &RedisCache{options: defaultOptions()}
	WithPassword("secret")(cache)
	if cache.options.Password != "secret" {
		t.Errorf("expected 'secret', got %q", cache.options.Password)
	}
}

func TestWithDB(t *testing.T) {
	cache := &RedisCache{options: defaultOptions()}
	WithDB(3)(cache)
	if cache.options.DB != 3 {
		t.Errorf("expected 3, got %d", cache.options.DB)
	}
}

func TestWithUsername(t *testing.T) {
	cache := &RedisCache{options: defaultOptions()}
	WithUsername("admin")(cache)
	if cache.options.Username != "admin" {
		t.Errorf("expected 'admin', got %q", cache.options.Username)
	}
}

func TestName(t *testing.T) {
	cache := &RedisCache{name: "my-cache"}
	if cache.Name() != "my-cache" {
		t.Errorf("expected 'my-cache', got %q", cache.Name())
	}
}

func TestHealthChecksReturnsTwoChecks(t *testing.T) {
	cache := &RedisCache{
		name: "test-redis",
	}

	checks := cache.HealthChecks()
	if len(checks) != 2 {
		t.Fatalf("expected 2 health checks, got %d", len(checks))
	}

	if checks[0].Name != "test-redis-liveness" {
		t.Errorf("expected 'test-redis-liveness', got %q", checks[0].Name)
	}
	if checks[0].Kind != healthkit.Liveness {
		t.Errorf("expected Liveness kind")
	}

	if checks[1].Name != "test-redis-readiness" {
		t.Errorf("expected 'test-redis-readiness', got %q", checks[1].Name)
	}
	if checks[1].Kind != healthkit.Readiness {
		t.Errorf("expected Readiness kind")
	}
}

func TestHealthCheckLivenessReturnsNil(t *testing.T) {
	cache := &RedisCache{
		name: "test-redis",
	}

	checks := cache.HealthChecks()
	err := checks[0].Fn(t.Context())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestHealthCheckReadinessReturnsErrorWhenClientNil(t *testing.T) {
	cache := &RedisCache{
		name: "test-redis",
	}

	checks := cache.HealthChecks()
	err := checks[1].Fn(t.Context())
	if err == nil {
		t.Fatal("expected error when client is nil")
	}
	if err.Error() != "redis client is not initialized" {
		t.Errorf("unexpected error: %v", err)
	}
}

func defaultOptions() *redis.Options {
	return &redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	}
}
