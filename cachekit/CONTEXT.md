# CacheKit Context

`cachekit` provides caching infrastructure and lifecycle management for cache backends (e.g., Redis).
It is part of the core infrastructure layer of the application.

## Glossary

- **CacheKit**: The module name for caching infrastructure.
- **Cache**: Interface for generic cache operations (`Get`, `Set`, `Delete`, `Exists`). Decouples consumers from Redis. A `Get` on a missing key returns an error satisfying `errors.Is(err, ErrCacheMiss)`.
- **ErrCacheMiss**: Sentinel error returned (or wrapped) by every `Cache` adapter on a cache miss. Consumers distinguish a miss from a real failure with `errors.Is(err, cachekit.ErrCacheMiss)` — no adapter-specific string matching.
- **RedisCache**: The specific implementation of `Cache` using Redis.
- **MemoryCache**: In-memory test adapter in `cachekit/memcache`. Implements `Cache` without external dependencies.

## Architecture

- `RedisCache` embeds `core.Lifecycle` for robust start/stop handling, and implements `HealthCheckProvider`. Health checks are delegated to `healthkit.StandardChecks`. Connection retry uses `core/retry.Do`.
- `healthkit.StandardChecks` provides lightweight no-op liveness checks and timed backend-ping readiness checks.
