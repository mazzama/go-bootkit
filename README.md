# Go Boot Kit

[![codecov](https://codecov.io/github/mazzama/go-bootkit/branch/master/graph/badge.svg?token=GNL2NI4D17)](https://codecov.io/github/mazzama/go-bootkit)

A production-ready Go application framework that integrates multiple components for building robust web services. It enforces a strict separation of infrastructure lifecycles from application domain logic.

## Features

- **Modular Architecture**: Independent modules (`core`, `cachekit`, `databasekit`, `serverkit`) that can be used together or independently.
- **Unified Lifecycle**: `core.AppRunner` orchestrates startups, graceful shutdowns, and coordinates health checks automatically.
- **Functional Options**: Configure components easily and safely with idiomatic functional options.
- **Trace-Correlated Logging**: Built-in `TraceHandler` automatically injects `trace_id` and `span_id` from OpenTelemetry into JSON logs.
- **Transaction Management**: `TxManager` handles nested database transactions seamlessly via `context.Context` and savepoints.
- **Auto-Wired Health Checks**: Any component that exposes health checks is automatically wired to the health endpoints (`/live`, `/ready`, `/startup`).

## Components

- **Core**: Base interfaces (`Component`), `AppRunner` for lifecycle orchestration, `Lifecycle` primitive for embeddable start/stop semantics, `healthkit` for probes (`StandardChecks`), and `logger` for structured JSON.
- **CacheKit**: Redis cache integration using `redis/go-redis/v9`.
- **DatabaseKit**: PostgreSQL integration using `jackc/pgx/v5` and `TxManager` for context-propagated transactions.
- **ServerKit**: HTTP server wrapped around `go-chi/chi/v5` with panic recovery, CORS, and graceful shutdown.

## Quick Start Example

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/mazzama/go-bootkit/cachekit"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/databasekit"
	"github.com/mazzama/go-bootkit/serverkit"
)

func main() {
	// 1. Setup structured trace-correlated logger
	logger := core.NewLogger(core.LoggerConfig{
		Level: slog.LevelInfo,
	})
	slog.SetDefault(logger)

	// 2. Initialize Infrastructure Components
	db := databasekit.NewPostgresDB(
		databasekit.WithConnString(os.Getenv("DB_CONN_STR")),
		databasekit.WithLogger(logger),
	)
	
	cache := cachekit.NewRedisCache(
		cachekit.WithAddress(os.Getenv("REDIS_ADDR")),
		cachekit.WithLogger(logger),
	)
	
	// Create transaction manager
	txManager := databasekit.NewTxManager(db.Pool())

	// 3. Setup HTTP Server and Routes
	healthAggregator := healthkit.NewAggregator(5 * time.Second)
	handler := serverkit.NewDefaultHandler(healthAggregator, logger)
	
	// Add custom routes (type assertion since we know the default is a chi router)
	mux := handler.(*chi.Mux)
	mux.Get("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := serverkit.NewHTTPServer("api", ":8080", handler, serverkit.WithLogger(logger))

	// 4. Run Application
	runner := core.NewApplicationRunner(
		core.WithLogger(logger),
		core.WithHealthAggregator(healthAggregator),
		core.WithServices(db, cache, server),
	)
	
	slog.Info("starting application")
	if err := runner.Run(context.Background()); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
```

## Transaction Management

The `DatabaseKit` provides a `TxManager` that allows transactions to be propagated seamlessly through the `context.Context`. This allows repositories and services to be composed without altering their signatures to accept explicit `*pgx.Tx` parameters.

```go
func (s *Service) DoComplexWork(ctx context.Context) error {
	return s.txManager.WithTx(ctx, func(ctx context.Context) error {
		// Both operations share the same transaction. 
		// If WithTx is nested inside another WithTx, it creates a SAVEPOINT!
		if err := s.repo1.Update(ctx, data); err != nil {
			return err
		}
		return s.repo2.Create(ctx, log)
	})
}
```

## Development

See individual module `CONTEXT.md` files for deeper domain architecture decisions and glossaries:
- [Core](./core/CONTEXT.md)
- [CacheKit](./cachekit/CONTEXT.md)
- [DatabaseKit](./databasekit/CONTEXT.md)
- [ServerKit](./serverkit/CONTEXT.md)

### Running tests

```bash
go test -v ./...
```
*(Requires a running Docker daemon for `testcontainers-go` integration tests)*

## License

This project is licensed under the MIT License – see the [LICENSE](./LICENSE) file for details.
