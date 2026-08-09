# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go Boot Kit is a multi-module Go library providing reusable components for building backend applications. Each module is independently versioned and can be imported separately.

## Module Structure

```
core/           - Foundation module with Component interface and ApplicationRunner
  component.go  - Component and HealthCheckProvider interfaces
  runner.go     - ApplicationRunner for orchestrating services with graceful shutdown
  retry/        - Exponential-backoff retry with jitter (shared by databasekit, cachekit)
  healthkit/    - Health check aggregator (Liveness, Readiness, Startup probes)

cachekit/       - Redis cache component implementing core.Component
databasekit/    - PostgreSQL component implementing core.Component
serverkit/      - HTTP server component (chi router) implementing core.Component
workerkit/      - Redis-backed background job processor (asynq) implementing core.Component

## Key Architecture Patterns

### Component Interface
All services implement `core.Component`:
```go
type Component interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

Components that embed `core.Lifecycle` automatically support readiness gating
(`Ready() <-chan struct{}`). The runner discovers readiness through a type
assertion on the `Ready()` method — no separate exported interface needed.

### Functional Options Pattern
All components use functional options for configuration:
```go
// Example: databasekit
db, err := databasekit.NewPostgresDB(connStr,
    databasekit.WithMaxConns(20),
    databasekit.WithConnectRetry(5, 100*time.Millisecond),
)
```

### Application Runner
The `core.ApplicationRunner` orchestrates multiple services:
- Starts all services concurrently using errgroup
- Supports start deadlines for components that expose `Ready() <-chan struct{}`
- Handles graceful shutdown on SIGINT/SIGTERM
- Runs Stop() on all services sequentially in reverse order during shutdown

### Enqueuer Seam (workerkit)
The `Enqueuer` interface decouples services from asynq:
```go
type Enqueuer interface {
    Enqueue(task Task, opts ...EnqueueOption) (*TaskInfo, error)
    EnqueueContext(ctx context.Context, task Task, opts ...EnqueueOption) (*TaskInfo, error)
}
```
Inject `workerkit.NewAsynqClient(...)` in production, `memqueue.New()` in tests.
Mirrors the `cachekit.Cache` / `memcache.MemoryCache` pattern.

### Retry (core/retry)
`retry.Do(ctx, maxAttempts, baseBackoff, fn)` provides exponential-backoff retry
with jitter and context cancellation. Used by `databasekit` and `cachekit` during
connection setup to tolerate transient backend unavailability.
## Commands

```bash
# Build a specific module
go build ./...

# Run tests for a module
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests for a specific package
go test -v ./healthkit

## Adding New Components

1. Create a new directory for the component (e.g., `newkit/`)
2. Create a `go.mod` with module path `github.com/mazzama/go-bootkit/newkit`
3. Import `github.com/mazzama/go-bootkit/core` for the Component interface
4. Implement the `Component` interface
5. Embed `core.Lifecycle` for start/stop/ready semantics
6. Provide `HealthChecks() []healthkit.Check` method for health integration
7. Add a CONTEXT.md with glossary and architecture notes
8. Add the module to CONTEXT-MAP.md and Makefile lint targets

## Dependencies

- `github.com/go-chi/chi/v5` - HTTP router (serverkit)
- `github.com/go-chi/httplog/v3` - Request logging (serverkit)
- `github.com/redis/go-redis/v9` - Redis client (cachekit)
- `github.com/jackc/pgx/v5` - PostgreSQL driver (databasekit)
- `golang.org/x/sync/errgroup` - Concurrent error handling (core)

## Go Version

Requires Go 1.24.6+

## Everyday development tools

The following tools are always-on for every session in this repo.

### Context7 MCP — library documentation

Use the Context7 MCP server to fetch current documentation whenever the task involves a library, framework, SDK, API, CLI tool, or cloud service — even well-known ones (chi, pgx, go-redis, etc.). Your training data may not reflect recent changes.

Steps:
1. Call `resolve-library-id` with the library name and the question.
2. Pick the best match by exact name, description relevance, snippet count, source reputation, and benchmark score.
3. Call `query-docs` with the selected library ID and the full question. If the question spans multiple distinct concepts, make a separate `query-docs` call per concept.
4. Answer using the fetched docs.

Do not use for: refactoring, writing scripts from scratch, debugging business logic, code review, or general programming concepts.

### Caveman — terse output (always-on, full mode)

Respond terse like smart caveman — drop articles, filler, pleasantries. Fragments OK. Technical terms exact. Code unchanged. Pattern: `[thing] [action] [reason]. [next step].`

Auto-clarity exception: drop to normal prose for security warnings, irreversible action confirmations, multi-step sequences where ambiguity risks misread, or when the user is confused. Resume caveman after.

Available commands: `/caveman lite`, `/caveman full`, `/caveman ultra`, `/caveman-commit`, `/caveman-review`, `/caveman-compress`.

### Ponytail — lazy senior dev (always-on, full mode)

Before writing any code, stop at the first rung that holds:

1. Does this need to be built at all? (YAGNI)
2. Does it already exist in this codebase? Reuse it.
3. Does the standard library already do this? Use it.
4. Does a native platform feature cover it? Use it.
5. Does an already-installed dependency solve it? Use it.
6. Can this be one line? Make it one line.
7. Only then: write the minimum code that works.

Rules: no unrequested abstractions, no avoidable dependencies, no boilerplate. Deletion over addition. Boring over clever. Fewest files possible. Shortest working diff wins. Mark deliberate simplifications with a `ponytail:` comment naming the ceiling and upgrade path.

Not lazy about: understanding the problem, input validation at trust boundaries, error handling, security, anything explicitly requested.

Available commands: `/ponytail lite`, `/ponytail full`, `/ponytail ultra`, `/ponytail off`.

