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

// Option configures a MemoryCache at construction time.
type Option func(*MemoryCache)

var _ cachekit.Cache = (*MemoryCache)(nil)

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

// Get retrieves the value at key and unmarshals it into dest. A cache miss
// returns an error satisfying errors.Is(err, cachekit.ErrCacheMiss).
func (c *MemoryCache) Get(_ context.Context, key string, dest any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.data[key]
	if !ok {
		return fmt.Errorf("%s: %w", key, cachekit.ErrCacheMiss)
	}
	return c.codecOrDefault().Unmarshal([]byte(v), dest)
}

// Set stores the value under key, serialized with the configured codec.
// TTL is not supported — entries persist until deleted or Reset.
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

// Delete removes the key from the cache. Deleting a missing key is a no-op.
func (c *MemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

// Exists reports whether key is present in the cache.
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
