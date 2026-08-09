package cachekit

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

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

func TestWithConnectRetry(t *testing.T) {
	cache := &RedisCache{options: defaultOptions()}
	WithConnectRetry(3, 100)(cache)
	if cache.retryAttempts != 3 {
		t.Errorf("expected 3, got %d", cache.retryAttempts)
	}
	if cache.retryBackoff != 100 {
		t.Errorf("expected 100, got %d", cache.retryBackoff)
	}

	// Should not set if maxAttempts or backoff <= 0
	WithConnectRetry(0, 100)(cache)
	if cache.retryAttempts != 3 {
		t.Errorf("expected 3, got %d", cache.retryAttempts)
	}
	WithConnectRetry(5, 0)(cache)
	if cache.retryAttempts != 3 {
		t.Errorf("expected 3, got %d", cache.retryAttempts)
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
func TestWithCodec(t *testing.T) {
	cache := &RedisCache{}
	codec := JSONCodec{}
	WithCodec(codec)(cache)
	if cache.codec == nil {
		t.Error("expected codec to be set")
	}
}
func TestJSONCodec(t *testing.T) {
	codec := JSONCodec{}
	type item struct {
		Name string `json:"name"`
	}
	b, err := codec.Marshal(item{Name: "bootkit"})
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}
	var res item
	if err := codec.Unmarshal(b, &res); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if res.Name != "bootkit" {
		t.Fatalf("expected bootkit, got %s", res.Name)
	}
	if err := codec.Unmarshal([]byte("invalid json"), &res); err == nil {
		t.Fatal("expected unmarshal error for invalid json")
	}
}

func TestCodecOrDefault(t *testing.T) {
	cache := &RedisCache{}
	if cache.codecOrDefault() != DefaultCodec {
		t.Error("expected DefaultCodec when cache.codec is nil")
	}
	c := JSONCodec{}
	cache.codec = c
	if cache.codecOrDefault() != c {
		t.Error("expected custom codec when set")
	}
}

func TestWithLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cache := &RedisCache{}
	WithLogger(logger)(cache)
	if cache.logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestRedisCacheConnectRetryFailure(t *testing.T) {
	cache, err := NewRedisCache("127.0.0.1:1", WithConnectRetry(2, 1*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ctx := t.Context()
	err = cache.Start(ctx)
	if err == nil {
		t.Fatal("expected error connecting to invalid redis address")
	}
	if !strings.Contains(err.Error(), "failed to connect to Redis after retries") {
		t.Errorf("unexpected error msg: %v", err)
	}
}
func TestRedisCacheConnectRetrySmallBackoff(t *testing.T) {
	cache, err := NewRedisCache("127.0.0.1:1", WithConnectRetry(2, 1*time.Nanosecond))
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ctx := t.Context()
	err = cache.Start(ctx)
	if err == nil {
		t.Fatal("expected error connecting to invalid redis address")
	}
}


func TestRedisCacheConnectRetryContextCanceled(t *testing.T) {
	cache, err := NewRedisCache("127.0.0.1:1", WithConnectRetry(5, 50*time.Millisecond))
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = cache.Start(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestRedisCacheConnectNoRetryFailure(t *testing.T) {
	cache, err := NewRedisCache("127.0.0.1:1")
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ctx := t.Context()
	err = cache.Start(ctx)
	if err == nil {
		t.Fatal("expected error connecting to invalid redis address")
	}
	if !strings.Contains(err.Error(), "failed to connect to Redis") {
		t.Errorf("unexpected error msg: %v", err)
	}
}
func TestRedisCacheSetMarshalError(t *testing.T) {
	cache := &RedisCache{}
	err := cache.Set(t.Context(), "key", make(chan int), 0)
	if err == nil {
		t.Fatal("expected marshal error")
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

func TestNewRedisCacheValidation(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{
			name:    "valid address",
			addr:    "localhost:6379",
			wantErr: false,
		},
		{
			name:    "empty address",
			addr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRedisCache(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRedisCache() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
