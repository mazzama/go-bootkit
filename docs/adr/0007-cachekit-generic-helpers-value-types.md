# Cachekit generic helpers and value types

## Status
Accepted

## Context
`cachekit` operations currently rely on the `any` (`interface{}`) type for deserialization (`Get(ctx, key, dest any)`). This forces consumers to write unmarshaling boilerplate, instantiate empty structs, pass pointers, and lose compile-time type safety. With the introduction of Go 1.18+ generics, we wanted to modernize the cache layer.

However, Go does not support generic methods on interfaces. To introduce generics without breaking the `Cache` component boundary or bloating dependency graphs (e.g., injecting `Cache[User]`, `Cache[Session]` everywhere), we needed a structural workaround. Furthermore, handling generic unmarshaling dynamically for both value types and pointer types introduces severe reflection overhead.

## Decision
We will use **Package-Level Generic Helpers** over a non-generic interface.
- We add `cachekit.Get[T any](ctx context.Context, c Cache, key string) (T, error)` and `cachekit.Set[T any](...)`.
- The core `Cache` interface itself remains non-generic and stable.

We will **Enforce Value Types** in these generic helpers, prioritizing raw performance over developer guardrails (adhering to our "Ponytail" standard). 
- The helper will allocate using `var zero T` and pass `&zero` to the underlying cache interface. 
- Using reflection to dynamically detect and allocate pointer types (to make `Get[*User]` safe) is explicitly rejected to avoid a reflection tax on high-throughput cache reads.
- Developers MUST pass value types (e.g., `Get[User]`). Passing a pointer type will result in a runtime panic or failure during unmarshaling, which will be strictly documented as a known boundary.

## Consequences
- Single-line, type-safe cache retrieval for downstream consumers.
- The dependency injection graph remains clean (one unified `Cache` component).
- Excellent performance (zero reflection overhead inside the generic wrapper).
- **Risk:** Developers passing pointer types will encounter panics. This failure mode must be documented and caught in local testing.