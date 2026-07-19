package serverkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type HTTPServer struct {
	core.Lifecycle

	name     string
	addr     string
	handler  http.Handler
	listener net.Listener
	server   *http.Server
	logger   *slog.Logger
}

type HTTPServerOption func(*HTTPServer)

func NewHTTPServer(name, addr string, handler http.Handler, options ...HTTPServerOption) *HTTPServer {
	srv := &HTTPServer{
		name:    name,
		addr:    addr,
		handler: handler,
		logger:  slog.Default(),
	}

	for _, option := range options {
		option(srv)
	}

	srv.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	srv.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(), error) {
		logger := srv.logger

		logger.Info("Starting HTTP server", "name", srv.name, "addr", srv.addr)

		listener, err := net.Listen("tcp", srv.addr)
		if err != nil {
			return nil, fmt.Errorf("failed to create listener: %w", err)
		}
		srv.listener = listener
		srv.server.Addr = listener.Addr().String()

		go func() {
			if err := srv.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("HTTP server error", "name", srv.name, "error", err)
			}
		}()

		logger.Info("HTTP server ready", "name", srv.name, "addr", srv.server.Addr)

		return func() {
			logger.Info("Stopping HTTP server", "name", srv.name)

			// Context with timeout for graceful shutdown
			stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			if err := srv.server.Shutdown(stopCtx); err != nil {
				logger.Error("Error shutting down HTTP server", "name", srv.name, "error", err)
			} else {
				logger.Info("HTTP server stopped", "name", srv.name)
			}
		}, nil
	})

	return srv
}

func NewDefaultHandler(health *healthkit.Aggregator, logger *slog.Logger) http.Handler {
	router := chi.NewRouter()

	if logger == nil {
		logger = slog.Default()
	}

	// Register default request logging middleware
	router.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		RecoverPanics: true,
		Schema:        httplog.SchemaECS,
	}))

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(60 * time.Second))

	// Setup health routes if aggregator is provided
	if health != nil {
		router.Get("/health/liveness", health.Handler(healthkit.Liveness))
		router.Get("/health/readiness", health.Handler(healthkit.Readiness))
		router.Get("/health/startup", health.Handler(healthkit.Startup))
		router.Get("/health", health.Handler(healthkit.Liveness))
	}

	return router
}

func WithLogger(logger *slog.Logger) HTTPServerOption {
	return func(s *HTTPServer) {
		s.logger = logger
	}
}

func (s *HTTPServer) Name() string {
	return s.name
}

func (s *HTTPServer) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(s.name, func(ctx context.Context) error {
		select {
		case <-s.Ready():
			return nil
		default:
			return fmt.Errorf("server not ready")
		}
	})
}

var _ core.Component = (*HTTPServer)(nil)
var _ core.Readyable = (*HTTPServer)(nil)
