# CacheKit Context

`cachekit` provides caching infrastructure and lifecycle management for cache backends (e.g., Redis).
It is part of the core infrastructure layer of the application.

## Glossary

- **CacheKit**: The module name for caching infrastructure.
- **Cache**: A component that implements the `core.Component` lifecycle and provides caching functionality.
- **RedisCache**: The specific implementation of `Cache` using Redis.

## Architecture

- Uses `core.Component` for lifecycle management (`Start`, `Stop`, `Ready`, `HealthChecks`).
- Liveness checks are lightweight no-ops to prevent pod restart storms during transient network issues.
- Readiness checks actively ping the cache backend with a timeout to temporarily stop routing traffic.
