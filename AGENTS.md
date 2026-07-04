# Go Boot Kit - Agent Guide

This file provides comprehensive information for AI coding agents working on the Go Boot Kit project.

---

## Project Overview

**Go Boot Kit** is a modular microservices framework for Go that provides production-ready components for building scalable applications. It implements a component-based architecture with elegant lifecycle management, Kubernetes-native health checks, and clean abstractions for common infrastructure concerns.

- **Language**: Go 1.24.6
- **Architecture**: Multi-module Go workspace
- **Total Codebase**: ~742 lines across 7 Go files (concise and focused)
- **Repository**: `github.com/mazzama/go-bootkit`

---

## Project Structure

The project uses a **multi-module Go workspace** structure:

```
go-bootkit/
├── go.work                 # Workspace definition
├── go.work.sum             # Workspace checksums
├── core/                   # Core interfaces and lifecycle management
│   ├── go.mod              # Module: github.com/mazzama/go-bootkit/core
│   ├── component.go        # Component and Readyable interfaces (13 lines)
│   ├── runner.go           # ApplicationRunner orchestration (117 lines)
│   ├── component_test.go   # Tests for component interfaces
│   └── healthkit/
│       └── health.go       # Health check aggregator (117 lines)
├── serverkit/              # HTTP server with chi router
│   ├── go.mod              # Module: github.com/mazzama/go-bootkit/serverkit
│   ├── go.sum              # Dependencies
│   └── server.go           # WebServer implementation (191 lines)
├── databasekit/            # PostgreSQL client via pgxpool
│   ├── go.mod              # Module: github.com/mazzama/go-bootkit/databasekit
│   ├── go.sum              # Dependencies
│   └── database.go         # PostgresDB implementation (151 lines)
├── cachekit/               # Redis client
│   ├── go.mod              # Module: github.com/mazzama/go-bootkit/cachekit
│   ├── go.sum              # Dependencies
│   └── cache.go            # RedisCache implementation (153 lines)
└── docs/
    └── plans/              # Implementation plans for CI/CD, tests, docs
```

### Module Dependencies

```
core (zero external deps)
  ↑
  ├── serverkit → chi/v5, httplog/v3
  ├── databasekit → pgx/v5
  └── cachekit → go-redis/v9
```

---

## Technology Stack

### Core Dependencies

| Module | External Dependencies |
|--------|----------------------|
| `core` | `golang.org/x/sync` (errgroup) |
| `serverkit` | `github.com/go-chi/chi/v5`, `github.com/go-chi/httplog/v3` |
| `databasekit` | `github.com/jackc/pgx/v5` |
| `cachekit` | `github.com/redis/go-redis/v9` |

### Standard Library Usage

- `context` - Request-scoped values and cancellation
- `log/slog` - Structured logging
- `net/http` - HTTP server
- `os/signal` - Signal handling
- `sync` - Mutex, RWMutex, WaitGroup
- `sync/atomic` - Atomic operations

---

## Architecture Patterns

### 1. Component Interface Pattern

All infrastructure components implement the `core.Component` interface:

```go
type Component interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

Components with startup signals implement `core.Readyable`:

```go
type Readyable interface {
    Ready() <-chan struct{}
}
```

### 2. Functional Options Pattern

All constructors use functional options for configuration:

```go
// Core runner options
runner := core.NewApplicationRunner(
    core.WithServices(server, db, cache),
    core.WithLogger(logger),
    core.WithShutdownTimeout(30*time.Second),
    core.WithStartDeadline(10*time.Second),
)

// Server options
server := serverkit.NewWebServer("api", ":8080",
    serverkit.WithWebServerLogger(logger),
    serverkit.WithHealthAggregator(health),
    serverkit.WithCustomMiddleware(authMiddleware),
)
```

### 3. Health Check Pattern

Kubernetes-style probe aggregation with three probe types:

```go
type Kind int
const (
    Liveness Kind = iota   // Is the component alive?
    Readiness              // Is the component ready to serve?
    Startup                // Has startup completed?
)
```

Components expose health checks via `HealthChecks()` method:

```go
func (db *PostgresDB) HealthChecks() []healthkit.Check {
    return []healthkit.Check{
        {Name: "db-liveness", Kind: healthkit.Liveness, Fn: pingFn},
        {Name: "db-readiness", Kind: healthkit.Readiness, Fn: readyFn},
    }
}
```

### 4. Application Runner Pattern

The `ApplicationRunner` orchestrates component lifecycle:
- Starts all components concurrently via `errgroup`
- Enforces start deadlines via `Ready()` channels
- Handles graceful shutdown on SIGINT/SIGTERM
- Configurable shutdown timeout (default 15s)

---

## Build and Development Commands

### Workspace Commands

```bash
# Sync dependencies across all modules
go work sync

# Build all modules
go work build

# Run tests for all modules
go work test ./...

# Run tests with race detector
go work test -race ./...
```

### Module-Specific Commands

```bash
# Core module
cd core && go test ./...
cd core && go build

# Server module
cd serverkit && go test ./...
cd serverkit && go build

# Database module
cd databasekit && go test ./...
cd databasekit && go build

# Cache module
cd cachekit && go test ./...
cd cachekit && go build
```

### Go Version

The project requires **Go 1.24.6**.

---

## Testing Strategy

### Current Test Coverage

| Module | Test Files | Coverage |
|--------|-----------|----------|
| `core` | `component_test.go` | Partial (component interfaces) |
| `core/healthkit` | None | None |
| `serverkit` | None | None |
| `databasekit` | None | None |
| `cachekit` | None | None |

### Test Conventions

- Use table-driven tests for multiple scenarios
- Mock external dependencies (PostgreSQL, Redis)
- Include compile-time interface checks: `var _ Interface = (*Type)(nil)`
- Test files follow Go naming: `*_test.go`

### Running Tests

```bash
# All tests
go work test ./...

# Verbose with coverage
go work test -v -cover ./...

# Race detection
go work test -race ./...

# Specific package
cd core && go test -v
```

---

## Code Style Guidelines

### Idiomatic Go Conventions

1. **Context as First Parameter**
   ```go
   func (db *PostgresDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
   ```

2. **Error Wrapping**
   ```go
   return fmt.Errorf("failed to connect to database: %w", err)
   ```

3. **Error Checking with `errors.Is()`**
   ```go
   if err := svc.Start(svcCtx); err != nil && !errors.Is(err, context.Canceled) {
       return fmt.Errorf("%s: %w", svc.Name(), err)
   }
   ```

4. **Defer for Cleanup**
   ```go
   db.mu.RLock()
   defer db.mu.RUnlock()
   return db.pool
   ```

5. **Interface Compliance Checks**
   ```go
   var _ core.Component = (*PostgresDB)(nil)
   var _ core.Readyable = (*PostgresDB)(nil)
   ```

### Naming Conventions

- **Interfaces**: Noun (e.g., `Component`, `Readyable`)
- **Struct Types**: Descriptive (e.g., `ApplicationRunner`, `PostgresDB`)
- **Constructor Options**: `WithXxx` (e.g., `WithLogger`, `WithServices`)
- **Private Fields**: Use unexported fields with accessor methods

### Thread Safety

- Use `sync.RWMutex` for protecting shared state
- Use `atomic.Value` for cache storage
- Use channels for lifecycle signaling

---

## Configuration

### Default Values

| Component | Default | Configurable |
|-----------|---------|--------------|
| Shutdown timeout | 15s | `WithShutdownTimeout()` |
| Start deadline | None | `WithStartDeadline()` |
| Health check timeout | 300ms | `Check.Timeout` |
| Request timeout | 60s | Hardcoded in serverkit |
| DB connection string | `postgres://postgres:postgres@localhost:5432/postgres` | `WithConnectionString()` |
| Redis address | `localhost:6379` | `WithAddress()` |

---

## Security Considerations

1. **Connection Strings**: Hardcoded defaults for local development - should be overridden in production
2. **Timeouts**: All health checks have configurable timeouts to prevent DoS
3. **Graceful Shutdown**: Components receive shutdown timeout context for proper cleanup
4. **Middleware**: Server includes panic recovery middleware

---

## Health Endpoints

The WebServer automatically exposes these HTTP endpoints:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Alias for liveness |
| `GET /health/liveness` | Returns 200 if server is running |
| `GET /health/readiness` | Returns 200 when ready to accept traffic |
| `GET /health/startup` | Returns 200 when startup is complete |

Response codes:
- `200 OK` - All checks passed
- `503 Service Unavailable` - One or more checks failed

---

## Planned Work

The `docs/plans/` directory contains implementation plans for:

1. **Test Suite** (`2025-02-04-test-suite.md`) - Achieve 90% test coverage
2. **CI/CD Pipeline** (`2025-02-04-ci-cd-pipeline.md`) - GitHub Actions workflows
3. **Godoc Comments** (`2025-02-04-godoc-comments.md`) - API documentation

These are **not yet implemented** and represent planned work.

---

## Key Files for Understanding

1. `core/component.go` - Start here for understanding the component abstraction
2. `core/runner.go` - Application orchestration and lifecycle
3. `core/healthkit/health.go` - Health check aggregation
4. `serverkit/server.go` - HTTP server implementation
5. `databasekit/database.go` - PostgreSQL wrapper
6. `cachekit/cache.go` - Redis wrapper
7. `OVERVIEW.md` - Comprehensive architectural overview
8. `CLAUDE.md` - Quick reference for Claude Code

---

## Common Tasks

### Adding a New Component

1. Create a struct implementing `core.Component`
2. Optionally implement `core.Readyable` if startup signaling is needed
3. Use functional options pattern for configuration
4. Add interface compliance checks
5. Implement `HealthChecks()` for health check integration

### Adding Tests

1. Create `*_test.go` files in the same package
2. Use table-driven tests where applicable
3. Mock external dependencies
4. Include compile-time interface checks

### Modifying Core

When modifying `core`:
1. Bump the version in `core/go.mod` if breaking changes
2. Update dependent modules' `go.mod` files
3. Run `go work sync`

---

## License

MIT License - See `LICENSE` file for details.
