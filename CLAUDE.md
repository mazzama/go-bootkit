# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Structure

Go Boot Kit is a modular application framework for building Go microservices. The project uses a multi-module Go workspace structure:

```
go-bootkit/
├── core/           # Core interfaces and runner (github.com/mazzama/go-bootkit/core)
├── serverkit/      # HTTP server with chi router (depends on core)
├── databasekit/    # PostgreSQL via pgxpool (depends on core)
├── cachekit/       # Redis client (depends on core)
└── healthkit/      # Health check aggregator (inside core/)
```

Each kit is a separate Go module with its own `go.mod`. All kits depend on `core` which defines the foundational interfaces.

## Architecture

### Component Interface Pattern

All kits implement the `core.Component` interface:

```go
type Component interface {
    Name() string
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

Components that have a startup/ready signal also implement `core.Readyable`:

```go
type Readyable interface {
    Ready() <-chan struct{}
}
```

### Application Runner

The `core.ApplicationRunner` manages component lifecycle:
- Starts all components concurrently via `errgroup`
- Optionally enforces start deadlines via `Ready()` channels
- Handles graceful shutdown on SIGINT/SIGTERM
- Configurable shutdown timeout (default 15s)

### Health Check System

`healthkit.Aggregator` supports three probe types (Kubernetes-style):
- `Liveness`: Is the component alive?
- `Readiness`: Is the component ready to serve traffic?
- `Startup`: Did the component complete startup?

Components expose health checks via `HealthChecks() []healthkit.Check`. The WebServer automatically registers these and exposes HTTP endpoints at `/health/liveness`, `/health/readiness`, `/health/startup`.

### Functional Options Pattern

All constructors use the functional options pattern:
```go
server := serverkit.NewWebServer("api", ":8080",
    serverkit.WithWebServerLogger(logger),
    serverkit.WithHealthAggregator(health),
)
```

## Development Commands

### Build and Dependencies
```bash
# Sync dependencies for all modules
go work sync

# Build a specific module
cd core && go build
cd serverkit && go build
# etc.
```

### Running Tests
```bash
# Run tests for all modules
go work test ./...

# Run tests for specific module
cd serverkit && go test ./...
```

## Module Dependencies

- **core**: Zero external dependencies (only stdlib and golang.org/x/sync)
- **serverkit**: Depends on `core`, uses `chi/v5` router and `httplog/v3`
- **databasekit**: Depends on `core`, uses `pgx/v5` for PostgreSQL
- **cachekit**: Depends on `core`, uses `go-redis/v9`

When modifying core, bump its version and update dependents' `go.mod` files accordingly.
