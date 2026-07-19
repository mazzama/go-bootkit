# DatabaseKit Context

`databasekit` provides database connection management, transaction management, and lifecycle integration.
It is part of the core infrastructure layer of the application.

## Glossary

- **DatabaseKit**: The module name for database infrastructure.
- **PostgresDB**: The component handling the PostgreSQL connection pool and lifecycle.
- **TxManager**: A component that manages transactions and nested savepoints using `context.Context`.
- **Querier**: An interface representing common query methods between `pgxpool.Pool` and `pgx.Tx`.
- **TxProvider**: An interface extending `Querier` that provides a `Begin(ctx)` method, allowing `TxManager` to start transactions without being coupled to `pgxpool.Pool`.
## Architecture

- `PostgresDB` is purely a lifecycle component (`Start`, `Stop`, `Ready`, `HealthChecks`, `SetLogger`) and exposes the raw connection pool via `Pool()`.
- `TxManager` provides `WithTx(ctx, fn)` for transaction boundaries and automatically handles nested transactions using database savepoints.
- Context propagation is used to thread active transactions through function calls.
- Liveness checks are lightweight no-ops to prevent pod restart storms during transient network issues.
- Readiness checks actively ping the database backend with a timeout to temporarily stop routing traffic.
