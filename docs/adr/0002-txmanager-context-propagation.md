# ADR 0002: Transaction Context Propagation and Nested Savepoints

## Status

Accepted

## Context

When interacting with a database, services frequently need to perform multiple operations within a single transaction to maintain atomicity. In complex codebases, functions that execute database queries are often reused across different workflows. Some of these workflows might require their own transactional boundaries, while others might already be executing within an active transaction and should reuse it.

Passing transaction objects (`*pgx.Tx`) explicitly through function signatures introduces tight coupling to the database layer and pollutes domain logic interfaces. Moreover, dealing with nested transactions (e.g., calling a transactional function from within another transactional function) is non-trivial since PostgreSQL does not natively support true nested transactions, only `SAVEPOINT`s.

## Decision

1. **Context-Based Propagation**: We will use `context.Context` to propagate the active transaction throughout the call stack. This ensures that repository and domain methods do not need to alter their signatures (e.g., passing `Tx` explicitly).
2. **Querier Interface**: We will define a `Querier` interface that abstracts both `pgxpool.Pool` and `pgx.Tx`. Functions that execute queries will depend on the `Querier` interface rather than concrete types.
3. **TxManager Component**: We introduce a `TxManager` component that provides a `WithTx(ctx, fn)` method to wrap business logic.
4. **Nested Transactions via Savepoints**: When `WithTx` is called, it inspects the provided `context.Context` to see if a transaction is already active. If one is, it creates a database `SAVEPOINT` instead of starting a new transaction. If the nested function succeeds, the savepoint is released. If it fails, the transaction rolls back to that savepoint, preserving the outer transaction's integrity.

## Consequences

- **Pros**:
    - **Clean Interfaces**: Repositories only need to accept a `context.Context` to participate in transactions.
    - **Reusability**: Functions can be freely composed without worrying about whether they are part of a larger transaction or need to start their own.
    - **Safety**: Automated savepoint management prevents partial failures from corrupting outer transactions.
- **Cons**:
    - **Implicit State**: Developers must remember that the transaction is hidden inside the context.
    - **Performance Overhead**: Frequent creation and release of savepoints incurs a slight performance penalty on the database compared to a single flat transaction.
