## Problem Statement

The go-bootkit architecture contains several shallow modules and concrete couplings that hinder testability and leak implementation details. Repositories depend on a concrete `TxManager` (making unit tests difficult without Docker), the router is exposed as an interface that lies (`http.Handler` when callers expect `*chi.Mux`), the cache interface is too shallow and leaks codec logic into callers, the logger is a pass-through that forces callers to wire resource attributes manually, and the application runner lacks isolated shutdown logic for testing.

## Solution

Deepen the architectural seams across the core and kit packages. Abstract `TxManager` into a `QuerierResolver` port, return an honest `chi.Router` from `serverkit`, push JSON marshaling down into `cachekit.Cache`, natively inject base resource attributes into `core.Logger`, and isolate `ApplicationRunner` lifecycle phases into unexported orchestrator methods.

## User Stories

1. As a developer writing tests, I want repositories to depend on a `QuerierResolver` interface, so that I can unit test domain logic using a mock or in-memory querier without requiring a live Postgres database via testcontainers.
2. As a framework consumer, I want `serverkit.NewDefaultHandler` to return an `http.Handler` with chi as the internal implementation, so that my wiring code only imports chi for the type assertion at the route-mounting point.
3. As a framework consumer, I want to configure the router timeout via a functional option, so that I can tune request timeouts for my specific service load rather than relying on a hardcoded 60s timeout.
4. As a framework consumer, I want `cachekit.Cache.Get` and `Set` to handle encoding, so that I don't have to write repetitive `json.Marshal`/`Unmarshal` boilerplate in every service method that touches the cache.
5. As an operator reviewing logs, I want trace IDs and base resource attributes (`service.name`, `service.version`, `deployment.environment`) automatically injected at the root of every log record, so that I can correlate logs efficiently across services and environments.
6. As a framework consumer, I want to configure base logger attributes using native functional options on `core.NewLogger`, so that I don't need to manually construct and wrap custom handlers.
7. As a framework maintainer, I want the `ApplicationRunner`'s shutdown logic extracted into a dedicated orchestrator method, so that I can easily test the shutdown budget and ordering logic in isolation.

## Implementation Decisions

- **Modules Modified**: `core`, `databasekit`, `serverkit`, `cachekit`, and example services (`examples/orders`).
- **Database Seam**: Introduced `QuerierResolver` interface in `databasekit`. Modified repositories in `examples/orders` to depend on `QuerierResolver` instead of the concrete `*TxManager`. The `*pgxpool.Pool` natively implements `TxProvider`, so bespoke lazy adapters were removed from tests.
- **Router Seam**: `serverkit.NewDefaultHandler` returns `http.Handler` (chi is the internal implementation). `WithRouterTimeout` exposed as a functional option. Callers type-assert to `*chi.Mux` at wiring time.
- **Cache Seam**: Deepened `cachekit.Cache` interface to `Get(ctx, key, dest any)` and `Set(ctx, key, value any)`. Modified `RedisCache` and `MemoryCache` implementations to own the JSON codec, eliminating boilerplate in `OrderService`.
- **Logger Seam**: Extended `core.LoggerConfig` to accept `ServiceName`, `Version`, and `Environment`. Updated `NewLogger` to construct these attributes and inject them into the `TraceHandler` directly.
- **Runner Seam**: Extracted `healthWiring`, `startSupervisor`, and `shutdownOrchestrator` methods in `core.ApplicationRunner`.

## Testing Decisions

- Good tests should verify external behavior rather than implementation details.
- Repositories can now be unit tested by mocking `QuerierResolver`, skipping `testcontainers` for basic logic validation.
- Existing tests for `serverkit` and `core` must pass without regressions.
- Prior art: Tests in `examples/orders/handler_test.go` bind directly to a database pool instead of using bespoke lazy providers, verifying that the `TxProvider` interface correctly resolves from a live `pgxpool.Pool`.

## Out of Scope

- A unified configuration seam (`ConfigSource` port) is deferred until a second consumer exists (following the YAGNI principle).
- DAG-based shutdown ordering (rejected as over-engineered; sticking to sequential shutdown for now).

## Further Notes

- All changes respect the "ponytail" constraints: minimizing new abstractions, keeping the shortest working diff, and avoiding unnecessary dependencies. Code is smaller, seams are deeper.
