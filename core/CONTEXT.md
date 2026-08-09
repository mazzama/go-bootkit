# Core

Core application lifecycle primitives and runner infrastructure.

## Language

**ApplicationRunner**:
A primitive that manages the startup, readiness checks, and graceful shutdown of a collection of services.
_Avoid_: Orchestrator, ProcessManager, AppRunner

**HealthAggregator**:
A runner-managed health check aggregator. Auto-collects health checks from registered components during `Run` and exposes an HTTP-compatible handler via `HealthAggregator()`.

**HealthCheckProvider**:
An interface that allows a service component to declare its internal liveness and readiness checks.
_Avoid_: HealthChecker, HealthCheckSource

**Lifecycle**:
An embeddable struct that provides robust, unified lifecycle management (`Start`, `Stop`, `Ready`) for infrastructure adapters. It handles concurrent readiness signals and release-exactly-once semantics. Every infrastructure component embeds `Lifecycle` — the runner discovers readiness through a type assertion on `Ready() <-chan struct{}` without requiring a separate exported interface.

**StandardChecks**:
A builder function in `healthkit` returning standard `Liveness` (nop) and `Readiness` (timed) checks for infrastructure components.

**retry**:
A `core/retry` sub-package providing `retry.Do(ctx, maxAttempts, baseBackoff, fn)` — exponential-backoff retry with jitter shared by infrastructure adapters that tolerate transient backend unavailability during startup.
