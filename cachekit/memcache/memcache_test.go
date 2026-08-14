package memcache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/cachekit"
	"github.com/mazzama/go-bootkit/cachekit/memcache"
)

func TestMemoryCache(t *testing.T) {
	cache := memcache.New()
	ctx := context.Background()

	// Test Exists on empty
	exists, err := cache.Exists(ctx, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatalf("expected foo to not exist")
	}

	// Test Set and Get string
	err = cache.Set(ctx, "foo", "bar", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var dest string
	err = cache.Get(ctx, "foo", &dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dest != "bar" {
		t.Fatalf("expected 'bar', got %q", dest)
	}

	// Test Exists on existing key
	exists, err = cache.Exists(ctx, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatalf("expected foo to exist")
	}

	// Test Get with unmarshalable dest
	var badDest chan int
	err = cache.Get(ctx, "foo", &badDest)
	if err == nil {
		t.Fatalf("expected error when unmarshaling into bad dest")
	}

	// Test Set and Get struct
	type Data struct {
		Name string
	}
	err = cache.Set(ctx, "data", Data{Name: "test"}, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var destData Data
	err = cache.Get(ctx, "data", &destData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if destData.Name != "test" {
		t.Fatalf("expected 'test', got %q", destData.Name)
	}

	// Test Delete
	err = cache.Delete(ctx, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exists, _ = cache.Exists(ctx, "foo")
	if exists {
		t.Fatalf("expected foo to be deleted")
	}

	// Test Reset
	cache.Reset()
	exists, _ = cache.Exists(ctx, "data")
	if exists {
		t.Fatalf("expected cache to be empty after reset")
	}

	// Test Get on miss
	err = cache.Get(ctx, "missing", &dest)
	if err == nil {
		t.Fatalf("expected error on cache miss")
	}
	if !errors.Is(err, cachekit.ErrCacheMiss) {
		t.Fatalf("expected errors.Is(err, cachekit.ErrCacheMiss), got: %v", err)
	}

	// Test Set with unmarshalable value (e.g. channel) to cover error path
	err = cache.Set(ctx, "unmarshalable", make(chan int), time.Minute)
	if err == nil {
		t.Fatalf("expected error when setting unmarshalable value")
	}
}

type customCodec struct{}

func (customCodec) Marshal(v any) ([]byte, error) {
	return []byte("prefix:" + v.(string)), nil
}

func (customCodec) Unmarshal(data []byte, dest any) error {
	*(dest.(*string)) = string(data[7:])
	return nil
}

func TestMemoryCacheWithCustomCodec(t *testing.T) {
	cache := memcache.New(memcache.WithCodec(customCodec{}))
	ctx := context.Background()

	err := cache.Set(ctx, "k", "val", time.Minute)
	if err != nil {
		t.Fatalf("unexpected set error: %v", err)
	}

	var res string
	err = cache.Get(ctx, "k", &res)
	if err != nil {
		t.Fatalf("unexpected get error: %v", err)
	}
	if res != "val" {
		t.Fatalf("expected 'val', got %q", res)
	}
}
