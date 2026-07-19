package serverkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type HTTPServer struct {
	name       string
	addr       string
	handler    http.Handler
	listener   net.Listener
	server     *http.Server
	readyCh    chan struct{}
	shutdownCh chan struct{}
	logger     *slog.Logger
	mu         sync.RWMutex
}

type HTTPServerOption func(*HTTPServer)

func NewHTTPServer(name, addr string, handler http.Handler, options ...HTTPServerOption) *HTTPServer {
	srv := &HTTPServer{
		name:       name,
		addr:       addr,
		handler:    handler,
		readyCh:    make(chan struct{}),
		shutdownCh: make(chan struct{}),
		logger:     slog.Default(),
	}

	for _, option := range options {
		option(srv)
	}

	srv.server = &http.Server{
		Addr:    addr,
		Handler: handler,
	}

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

func (s *HTTPServer) Ready() <-chan struct{} {
	return s.readyCh
}

func (s *HTTPServer) Start(ctx context.Context) error {
	s.mu.RLock()
	logger := s.logger
	s.mu.RUnlock()

	logger.Info("Starting HTTP server", "name", s.name, "addr", s.addr)

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	// Update the actual address in case a random port (:0) was used
	s.server.Addr = listener.Addr().String()

	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server error", "name", s.name, "error", err)
		}
	}()

	close(s.readyCh)
	logger.Info("HTTP server ready", "name", s.name, "addr", s.server.Addr)

	<-ctx.Done()
	return ctx.Err()
}

func (s *HTTPServer) Stop(ctx context.Context) error {
	s.mu.RLock()
	logger := s.logger
	s.mu.RUnlock()

	logger.Info("Stopping HTTP server", "name", s.name)

	if err := s.server.Shutdown(ctx); err != nil {
		logger.Error("Error shutting down HTTP server", "name", s.name, "error", err)
		return err
	}

	close(s.shutdownCh)
	logger.Info("HTTP server stopped", "name", s.name)
	return nil
}



func (s *HTTPServer) HealthChecks() []healthkit.Check {
	return []healthkit.Check{
		{
			Name: s.name + "-liveness",
			Kind: healthkit.Liveness,
			Fn: func(ctx context.Context) error {
				return nil
			},
		},
		{
			Name: s.name + "-readiness",
			Kind: healthkit.Readiness,
			Fn: func(ctx context.Context) error {
				select {
				case <-s.readyCh:
					return nil
				default:
					return fmt.Errorf("server not ready")
				}
			},
		},
	}
}

var _ core.Component = (*HTTPServer)(nil)
var _ core.Readyable = (*HTTPServer)(nil)
