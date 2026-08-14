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
An embeddable struct that provides robust, unified lifecycle management (`Start`, `Stop`, `Ready`, `WaitReady`) for infrastructure adapters. It handles concurrent readiness signals and release-exactly-once semantics. `Lifecycle` implements `Readyable`.

**WaitReady**:
The single owner of the "wait until connected, honour the deadline" invariant. A free function `WaitReady(ctx, ready <-chan struct{}) error` for callers holding a `Readyable`, plus a `Lifecycle.WaitReady(ctx)` method for adapters that embed it. Callers no longer hand-roll a `select` over `Ready()` and `ctx.Done()`.
_Avoid_: IsReady, AwaitReady

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

`TraceHandler` injects `trace_id`/`span_id` at the root of every record before
applying any attrs or groups. Attributes configured at setup time (as above)
land at the root; attributes added later via `logger.With(...)` are applied in
the order `WithGroup`/`WithAttrs` are called, so nested groups keep their
structure.
