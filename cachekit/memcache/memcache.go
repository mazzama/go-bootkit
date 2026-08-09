package memcache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mazzama/go-bootkit/cachekit"
)

// MemoryCache is an in-memory implementation of cachekit.Cache for use in tests.
// It does not support TTL expiration — entries persist until explicitly deleted
// or the cache is reset.
type MemoryCache struct {
	mu    sync.Mutex
	data  map[string]string
	codec cachekit.Codec
}

var _ cachekit.Cache = (*MemoryCache)(nil)

type Option func(*MemoryCache)

// WithCodec sets a custom Codec for MemoryCache.
func WithCodec(codec cachekit.Codec) Option {
	return func(c *MemoryCache) {
		if codec != nil {
			c.codec = codec
		}
	}
}

// New creates a ready-to-use MemoryCache.
func New(opts ...Option) *MemoryCache {
	c := &MemoryCache{
		data: make(map[string]string),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *MemoryCache) codecOrDefault() cachekit.Codec {
	if c.codec != nil {
		return c.codec
	}
	return cachekit.DefaultCodec
}

func (c *MemoryCache) Get(_ context.Context, key string, dest any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	if !ok {
		return fmt.Errorf("cache miss: %s", key)
	}
	return c.codecOrDefault().Unmarshal([]byte(v), dest)
}

func (c *MemoryCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := c.codecOrDefault().Marshal(value)
	if err != nil {
		return err
	}
	c.data[key] = string(b)
	return nil
}

func (c *MemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

func (c *MemoryCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.data[key]
	return ok, nil
}

// Reset clears all entries. Useful in test teardown.
func (c *MemoryCache) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = make(map[string]string)
}
