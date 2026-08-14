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


**Readyable**:
An exported interface (`Ready() &lt;-chan struct{}`) signalling that a component has an observable readiness channel. `Lifecycle` implements `Readyable`. The runner and `TxManager` accept `Readyable` rather than duck-typing. New adapters should implement `Readyable` for compile-time safety.

**Lifecycle**:
An embeddable struct that provides robust, unified lifecycle management (`Start`, `Stop`, `Ready`) for infrastructure adapters. It handles concurrent readiness signals and release-exactly-once semantics. `Lifecycle` implements `Readyable`.

**StandardChecks**:
A builder function in `healthkit` returning standard `Liveness` (nop) and `Readiness` (timed) checks for infrastructure components.

**retry**:
A `core/retry` sub-package providing `retry.Do(ctx, maxAttempts, baseBackoff, fn)` — exponential-backoff retry with jitter shared by infrastructure adapters that tolerate transient backend unavailability during startup.

**TraceHandler**:
An `slog.Handler` middleware that wraps any base handler to automatically inject OpenTelemetry `trace_id` and `span_id` from `context.Context` into log records.

## Patterns

### Trace-Correlated Logger Setup

Configure a standard `slog.Logger` with OpenTelemetry trace correlation by composing `slog.NewJSONHandler` (or any `slog.Handler`) with `core.NewTraceHandler`:

```go
handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
traced := core.NewTraceHandler(handler)
logger := slog.New(traced.WithAttrs([]slog.Attr{
    slog.String("service.name", serviceName),
}))
```
