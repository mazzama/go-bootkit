package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/hibiken/asynq"
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

	// 2. Trace-correlated logger. NewLogger wraps a JSON handler with the
	//    TraceHandler, so any log emitted with a span in its context carries
	//    trace_id/span_id.
	logger := core.NewLogger(core.WithLogLevel(slog.LevelInfo))
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
	db, err := databasekit.NewPostgresDB(
		databasekit.WithConnectionString(cfg.DBConnStr),
		databasekit.WithLogger(logger),
	)
	if err != nil {
		return err
	}

	cache, err := cachekit.NewRedisCache(
		cachekit.WithAddress(cfg.RedisAddr),
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
	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword}
	asyncClient := workerkit.NewAsynqClient("notification-client", redisOpt)
	asyncServer := workerkit.NewAsynqServer(
		"notification-worker",
		redisOpt,
		asynq.Config{Concurrency: 5},
	)

	// Register handlers
	processor := NewNotificationProcessor(logger)
	asyncServer.Mux().HandleFunc("notification:send", processor.Process)

	runner := core.NewApplicationRunner(
		core.WithLogger(logger),
		core.WithServices(db, cache, asyncClient, asyncServer),
	)

	// 6. HTTP server. NewDefaultHandler gives a chi router with health probes,
	//    request logging, and panic recovery already wired. We mount the example
	//    routes onto it, then wrap the whole router in the OpenTelemetry HTTP
	//    middleware. Wrapping at the server level (rather than router.Use) avoids
	//    chi's "middleware after routes" panic, since the default handler has
	//    already registered its health routes.
	router := serverkit.NewDefaultHandler(runner.HealthAggregator(), logger)
	handler.Routes(router)

	var httpHandler http.Handler = otelhttp.NewHandler(router, "http.server")
	server, err := serverkit.NewHTTPServer(serviceName, cfg.HTTPAddr, httpHandler, serverkit.WithLogger(logger))
	if err != nil {
		return err
	}

	// 7. Run. The runner starts db, cache, and server; auto-registers their
	//    health checks; and shuts everything down gracefully on SIGINT/SIGTERM.
	core.WithServices(server)(runner)

	logger.Info("starting orders example", "http_addr", cfg.HTTPAddr)
	return runner.Run(context.Background())
}
