# CacheKit Context

`cachekit` provides caching infrastructure and lifecycle management for cache backends (e.g., Redis).
It is part of the core infrastructure layer of the application.

## Glossary

- **CacheKit**: The module name for caching infrastructure.
- **Cache**: A component that implements the `core.Component` lifecycle and provides caching functionality.
- **RedisCache**: The specific implementation of `Cache` using Redis.

## Architecture

- `RedisCache` embeds `core.Lifecycle` for robust start/stop handling, and implements `Readyable` and `HealthCheckProvider`. Health checks are delegated to `healthkit.StandardChecks`.
- `healthkit.StandardChecks` provides lightweight no-op liveness checks and timed backend-ping readiness checks.
