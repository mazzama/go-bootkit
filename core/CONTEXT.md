# Core

Core application lifecycle primitives and runner infrastructure.

## Language

**ApplicationRunner**:
A primitive that manages the startup, readiness checks, and graceful shutdown of a collection of services.
_Avoid_: Orchestrator, ProcessManager, AppRunner

**HealthCheckProvider**:
An interface that allows a service component to declare its internal liveness and readiness checks.
_Avoid_: HealthChecker, HealthCheckSource

**Lifecycle**:
An embeddable struct that provides robust, unified lifecycle management (`Start`, `Stop`, `Ready`) for infrastructure adapters. It handles concurrent readiness signals and release-exactly-once semantics.

**StandardChecks**:
A builder function in `healthkit` returning standard `Liveness` (nop) and `Readiness` (timed) checks for infrastructure components.
