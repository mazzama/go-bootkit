# Framework-native query results

## Status

Accepted

Supersedes ADR-0004.

## Context

The `Querier`/`TxProvider` seam leaked pgx through every return type — `pgconn.CommandTag`, `pgx.Rows`, `pgx.Row`, `pgx.Tx` — so repositories still `import pgx/v5` despite the seam's advertised decoupling. ADR-0004 deferred fixing this because `*pgxpool.Pool` satisfied `TxProvider` natively, and losing that fit seemed to buy nothing.

## Decision

The seam is now pgx-free. `Querier` returns framework-native types:

- `Exec(ctx, sql, args...) (int64, error)` — folds `pgconn.CommandTag` into rows-affected.
- `Query(ctx, sql, args...) (Rows, error)` — `Rows` is a `Next`/`Scan`/`Err`/`Close` iterator.
- `QueryRow(ctx, sql, args...) Row` — `Row` is a bare `Scan(dest...) error`; the error is deferred to `Scan` (readiness, no-rows, driver error).

`TxProvider.Begin` returns the framework `Tx`, and `Tx` embeds `TxProvider` plus `Commit`/`Rollback`. `pgx.ErrNoRows` and `pgx.ErrTxClosed` are rewritten to `databasekit.ErrNoRows`/`ErrTxClosed` inside the adapters.

pgx now lives only in unexported adapters (`pgxRowAdapter`, `pgxRowsAdapter`, `pgxTxAdapter`, `poolAdapter`). The raw pool's native fit is dropped: `NewPoolProvider(*pgxpool.Pool) TxProvider` wraps it at wiring time, and `check_test.go` asserts the adapters satisfy the seam instead of the pool.

## Considered options

- **`Get(ctx, sql, args, dest...)`** (fold `QueryRow` + `Scan` into one call): collapses the idiom but forces `args []any` ceremony and re-shapes every call site for no leverage over the existing deferred-`Scan` pattern.
- **Capability interfaces** (`Batcher`/`Copier`/`RawUnwrapper` via type assertion): re-exposes driver surface speculatively and moves errors from compile time to run time.

## Consequences

- Repositories no longer import pgx; the pgx dependency is confined to the wiring and adapter layer.
- `NewPoolProvider` is required wherever a raw pool is handed to `TxManager` (test setup, non-`Lifecycle` deployments).
- `Exec` loses `CommandTag` fidelity (command string, `INSERT ... RETURNING` OID). Nothing in the repo uses it; a future caller needing it reopens this decision.
