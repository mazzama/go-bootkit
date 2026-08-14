package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/mazzama/go-bootkit/cachekit"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/databasekit"
	"github.com/mazzama/go-bootkit/serverkit"
	"github.com/mazzama/go-bootkit/workerkit"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const serviceName = "orders-example"

func main() {
	if err := run(); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Configuration.
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}

	// 2. Trace-correlated logger. Composing standard slog.NewJSONHandler with
	//    core.NewTraceHandler so logs with a span in context carry trace_id/span_id.
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	traced := core.NewTraceHandler(logHandler)
	logger := slog.New(traced.WithAttrs([]slog.Attr{
		slog.String("service.name", serviceName),
	}))
	slog.SetDefault(logger)

	// 3. OpenTelemetry tracing to stdout, so the trace IDs above are real.
	shutdownTracer, err := setupTracing(serviceName)
	if err != nil {
		return err
	}
	defer func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracer(shCtx); err != nil {
			logger.Error("failed to flush traces", "error", err)
		}
	}()

	// 4. Infrastructure components. The runner starts these; their pools/clients
	//    are nil until then, which is why the TxManager reads the pool lazily.
	db, err := databasekit.NewPostgresDB(cfg.DBConnStr,
		databasekit.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	cache, err := cachekit.NewRedisCache(cfg.RedisAddr,
		cachekit.WithPassword(cfg.RedisPassword),
		cachekit.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	// 5. Domain wiring. The TxManager uses the database's TxProvider, which
	//    lazily waits for the connection pool to become ready when called.
	txManager := databasekit.NewTxManager(db.TxProvider())
	productRepo := NewProductRepository(txManager)
	orderRepo := NewOrderRepository(txManager)
	service := NewOrderService(txManager, productRepo, orderRepo, cache, logger)
	handler := NewHandler(service, logger)

	// Set up background worker (client + server)
	workerRedis := workerkit.RedisConfig{Addr: cfg.RedisAddr, Password: cfg.RedisPassword}
	asyncClient := workerkit.NewAsynqClient("notification-client", workerRedis)
	asyncServer := workerkit.NewAsynqServer(
		"notification-worker",
		workerRedis,
		workerkit.ServerConfig{Concurrency: 5},
	)

	// Register handlers
	processor := NewNotificationProcessor(logger)
	asyncServer.HandleFunc("notification:send", processor.Process)

	runner := core.NewApplicationRunner(
		core.WithLogger(logger),
		core.WithServices(db, cache, asyncClient, asyncServer),
	)

	// 6. HTTP server. NewDefaultRouter returns a chi.Router pre-configured
	//    with middleware, health probes, request logging, and panic recovery.
	//    Mount application routes, then wrap in OpenTelemetry.
	router := serverkit.NewDefaultRouter(runner.HealthAggregator(), logger)
	handler.Routes(router)

	otelHandler := otelhttp.NewHandler(router, "http.server")
	server, err := serverkit.NewHTTPServer(serviceName, cfg.HTTPAddr, otelHandler, serverkit.WithLogger(logger))
	if err != nil {
		return err
	}

	// 7. Run. The runner starts db, cache, and server; auto-registers their
	//    health checks; and shuts everything down gracefully on SIGINT/SIGTERM.
	core.WithServices(server)(runner)

	logger.Info("starting orders example", "http_addr", cfg.HTTPAddr)
	return runner.Run(context.Background())
}
