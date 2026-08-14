# Readiness gate ownership

## Status

Accepted

## Context

Every adapter that embeds `core.Lifecycle` needs the same invariant: "wait until connected, honour the caller's deadline." Before this ADR the blocking wait was hand-rolled in four places — `AsynqClient.EnqueueContext`, `readyQuerier.wait`, `TxManager.WithTx`, and `asynqHealthChecks` — each with its own slightly different `select` over `Ready()` and `ctx.Done()`.

## Decision

The select is extracted to a single `core.WaitReady(ctx, ready <-chan struct{}) error` free function, with a `Lifecycle.WaitReady(ctx)` method for adapters that embed it. Callers that hold a `Readyable` (which exposes only the channel) call the free function; adapters that embed `Lifecycle` call the method.

The *gate sites* stay put because they express distinct contracts:

- `readyQuerier.wait` — the lazy wait: a query through `TxManager` blocks until the pool connects, up to `ctx`'s deadline, returning `pool not ready: <ctx.Err()>`.
- `TxManager.WithTx` — the same wait before `Begin`.

These are deliberately not collapsed into `txProviderAdapter`: the adapter's role is surface-narrowing (`PostgresDB` keeps its query methods private), not readiness. `QuerierFromContext` returning a raw `*pgxpool.Pool` (non-`Readyable`, already connected) must skip the gate entirely, which the type-assertion already encodes. The non-blocking probes — `AsynqClient.Enqueue` and `HTTPServer.HealthChecks` — are a different contract (fail-fast, no deadline) and remain `select` statements over `Ready()`.

## Rejected

- Gate moved into `txProviderAdapter`: would make direct-adapter use block (lazy) instead of fail-fast with `database pool is not initialized`, changing the nil-pool contract.
- Resolver-owned gating in `QuerierFromContext`: writes the gate twice — once for the resolver, once for `WithTx`.
- `IsReady() bool` probe: reintroduces a check-then-wait race.
