# Querier seam exposes pgx types

## Status

Deprecated — superseded by ADR-0006

## Context

`databasekit.Querier` and `TxProvider` return pgx-native types — `pgconn.CommandTag`, `pgx.Rows`, `pgx.Row`, `pgx.Tx` — so repositories still `import pgx/v5`. A future reviewer will see this as a leak across the seam and propose framework-native row types.

## Decision

The seam deliberately exposed pgx types. Two real adapters justified it: `*pgxpool.Pool` satisfied `TxProvider` natively, and `txProviderAdapter` wrapped `PostgresDB` to keep its query methods private. Replacing the return types with framework-native `Row`/`Rows`/`Result` would break the pool's native fit and require wrapping the pool, expanding the interface to solve a problem that had not yet occurred.

Revisit only when a non-pgx backend or a pure in-process fake must sit behind the seam — at which point the second adapter makes the wrapping real rather than hypothetical.

## Superseded by

ADR-0006. The trigger arrived: the seam now supports an in-process fake, and the pgx leak is removed in favor of framework-native result types.
