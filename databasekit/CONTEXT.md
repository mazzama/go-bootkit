# DatabaseKit Context

`databasekit` provides database connection management, transaction management, and lifecycle integration.
It is part of the core infrastructure layer of the application.

## Glossary

- **DatabaseKit**: The module name for database infrastructure.
- **PostgresDB**: The component handling the PostgreSQL connection pool and lifecycle.
- **TxManager**: A component that manages transactions and nested savepoints using `context.Context`.
- **Querier**: An interface representing common query methods between `pgxpool.Pool` and `pgx.Tx`.
- **TxProvider**: An interface extending `Querier` that provides a `Begin(ctx)` method, allowing `TxManager` to start transactions without being coupled to `pgxpool.Pool`. The single public entry point to `PostgresDB`; returns an internal adapter that delegates to the pool.
## Architecture

- `PostgresDB` embeds `core.Lifecycle` for robust start/stop handling, implements `HealthCheckProvider`, and exposes access through `TxProvider()` only. Querier methods (`Exec`, `Query`, `QueryRow`, `Begin`) are private — all database access flows through the `TxManager` seam. Health checks are delegated to `healthkit.StandardChecks`. Connection retry uses `core/retry.Do`.
- `TxManager` provides `WithTx(ctx, fn)` for transaction boundaries and automatically handles nested transactions using database savepoints.
- Context propagation is used to thread active transactions through function calls.
- `healthkit.StandardChecks` is used to provide lightweight no-op liveness checks and timed backend-ping readiness checks.
