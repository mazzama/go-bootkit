# Go taste

## Architecture & design
- Prefers explicit constructor wiring ("natural DI") over reflection/magic; constructors should return the instance plus a cleanup/shutdown function. Confidence: 0.9
- Prefers using native types directly (e.g., `*sqlx.DB`, `*redis.Client`) instead of hiding them behind unnecessary interfaces — no over-engineering. Confidence: 0.9
- Prefers transaction management with `sqlx` and `context.Context` that safely handles nested transactions without deadlocks or leaking connections. Confidence: 0.9
- Prefers `log/slog` observability that correctly extracts OpenTelemetry `trace_id`/`span_id` from context, safe under high concurrency. Confidence: 0.9
- Prefers Kubernetes readiness probes that actively ping dependencies (PostgreSQL, Redis) with timeouts. Confidence: 0.9
- Prefers graceful shutdown of the server and all connections on OS interruption signals. Confidence: 0.9
- Prefers a flat directory structure for Go services. Confidence: 0.7
- Prefers to question whether a proposed framework abstraction should instead be created in the application service layer — wants framework modules justified by multiple adapters/consumers rather than added speculatively. Confidence: 0.5

## Tooling & project layout
- Prefers `goose` for database migrations. Confidence: 0.8
- Prefers struct-based environment/configuration handling over ad-hoc env parsing. Confidence: 0.8
- Prefers integration tests that exercise the handler layer using dbcheck, written in BDD style. Confidence: 0.8
- Prefers each example service to live in its own Go module under `examples/`. Confidence: 0.7
