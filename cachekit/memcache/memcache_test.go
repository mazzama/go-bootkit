package memcache_test

import (
	"context"
	"testing"
	"time"

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
}
