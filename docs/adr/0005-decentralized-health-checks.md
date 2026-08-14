# Health checks stay decentralized

## Status

Accepted

## Context

Each adapter (`RedisCache`, `PostgresDB`, `HTTPServer`, `AsynqClient`, `AsynqServer`) implements a near-identical one-line `HealthChecks()` that delegates to `healthkit.StandardChecks`. A future reviewer will see four copies and propose centralizing the construction on `core.Lifecycle`.

## Decision

Health-policy construction does not move onto `core.Lifecycle`. `healthkit.StandardChecks` remains the primitive, and each adapter keeps its one-liner. The per-adapter variation — a backend ping (`RedisCache`, `PostgresDB`) versus a readiness-channel wait (`HTTPServer`, `AsynqClient`, `AsynqServer`) — is exactly what the seam exists to express, and the duplicated readiness `select` that made the boilerplate look collapsible is removed by `Lifecycle.WaitReady` (ADR-0003).
