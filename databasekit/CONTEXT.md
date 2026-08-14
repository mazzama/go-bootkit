# DatabaseKit Context

`databasekit` provides database connection management, transaction management, and lifecycle integration.
It is part of the core infrastructure layer of the application.

## Glossary

- **DatabaseKit**: The module name for database infrastructure.
- **PostgresDB**: The component handling the PostgreSQL connection pool and lifecycle.
- **TxManager**: A component that manages transactions and nested savepoints using `context.Context`.
- **Querier**: The pgx-free query surface (`Exec`, `Query`, `QueryRow`). Callers never import pgx. Result types are framework-native (`Row`, `Rows`; `Exec` returns `int64`).
- **TxProvider**: Extends `Querier` with `Begin(ctx) (Tx, error)`. The single public entry point to `PostgresDB`; returns an internal adapter that delegates to the pool. A raw `*pgxpool.Pool` no longer satisfies it — wrap it with `NewPoolProvider` at wiring time (see ADR-0006).
- **QuerierResolver**: The seam repositories depend on to resolve a `Querier` from context. Satisfied by `TxManager`. The `Querier`/`TxProvider`/`Tx` interfaces are pgx-free; pgx lives only in the unexported adapters — see ADR-0006.
- **ReadyGate**: The readiness guard that blocks a query or `Begin` until the pool is connected. Logic is `core.WaitReady`; the gate sites are `readyQuerier` (lazy query wait) and `TxManager.WithTx` (before `Begin`). Deliberately not moved into `txProviderAdapter` — see ADR-0003.
## Architecture

- `PostgresDB` embeds `core.Lifecycle` for robust start/stop handling, implements `HealthCheckProvider`, and exposes access through `TxProvider()` only. Querier methods (`Exec`, `Query`, `QueryRow`, `Begin`) are private — all database access flows through the `TxManager` seam. Health checks are delegated to `healthkit.StandardChecks`. Connection retry uses `core/retry.Do`. pgx is confined to the unexported adapters (`pgxRowAdapter`, `pgxRowsAdapter`, `pgxTxAdapter`, `poolAdapter`).
- `TxManager` provides `WithTx(ctx, fn)` for transaction boundaries and automatically handles nested transactions using database savepoints.
- Context propagation is used to thread active transactions through function calls.
- `healthkit.StandardChecks` is used to provide lightweight no-op liveness checks and timed backend-ping readiness checks.
