package serverkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httplog/v3"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

// Default HTTP server timeouts. These guard against slow clients and leaked
// keepalive connections exhausting the server under load.
const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultMaxHeaderBytes    = 1 << 20 // 1MB
)

type HTTPServer struct {
	core.Lifecycle

	name              string
	addr              string
	handler           http.Handler
	listener          net.Listener
	server            *http.Server
	logger            *slog.Logger
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	maxHeaderBytes    int
}

type HTTPServerOption func(*HTTPServer)

func NewHTTPServer(name, addr string, handler http.Handler, options ...HTTPServerOption) (*HTTPServer, error) {
	if addr == "" {
		return nil, errors.New("http server address cannot be empty")
	}
	if handler == nil {
		return nil, errors.New("http server handler cannot be nil")
	}

	srv := &HTTPServer{
		name:              name,
		addr:              addr,
		handler:           handler,
		logger:            slog.Default(),
		readHeaderTimeout: defaultReadHeaderTimeout,
		readTimeout:       defaultReadTimeout,
		writeTimeout:      defaultWriteTimeout,
		idleTimeout:       defaultIdleTimeout,
		maxHeaderBytes:    defaultMaxHeaderBytes,
	}

	for _, option := range options {
		option(srv)
	}

	srv.server = &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: srv.readHeaderTimeout,
		ReadTimeout:       srv.readTimeout,
		WriteTimeout:      srv.writeTimeout,
		IdleTimeout:       srv.idleTimeout,
		MaxHeaderBytes:    srv.maxHeaderBytes,
	}

	srv.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
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

		return func(stopCtx context.Context) error {
			logger.Info("Stopping HTTP server", "name", srv.name)

			if err := srv.server.Shutdown(stopCtx); err != nil {
				logger.Error("Error shutting down HTTP server", "name", srv.name, "error", err)
				return err
			}

			logger.Info("HTTP server stopped", "name", srv.name)
			return nil
		}, nil
	})

	return srv, nil
}

type RouterOptions struct {
	Timeout     time.Duration
	Middlewares []func(http.Handler) http.Handler
}

type RouterOption func(*RouterOptions)

// WithRouterTimeout sets the request timeout middleware duration.
func WithRouterTimeout(d time.Duration) RouterOption {
	return func(o *RouterOptions) {
		if d > 0 {
			o.Timeout = d
		}
	}
}

// WithMiddleware appends custom middlewares to the router options.
func WithMiddleware(middlewares ...func(http.Handler) http.Handler) RouterOption {
	return func(o *RouterOptions) {
		o.Middlewares = append(o.Middlewares, middlewares...)
	}
}

// MountHealthRoutes registers the standard health probe endpoints on the given
// router. Consumers who build their own chi router can call this directly
// instead of using NewDefaultRouter.
func MountHealthRoutes(mux chi.Router, health *healthkit.Aggregator) {
	if health == nil {
		return
	}
	mux.Get("/health/liveness", health.Handler(healthkit.Liveness))
	mux.Get("/health/readiness", health.Handler(healthkit.Readiness))
	mux.Get("/health/startup", health.Handler(healthkit.Startup))
	mux.Get("/health", health.Handler(healthkit.Liveness))
}

func NewDefaultRouter(health *healthkit.Aggregator, logger *slog.Logger, opts ...RouterOption) chi.Router {
	options := RouterOptions{Timeout: 60 * time.Second}
	for _, opt := range opts {
		opt(&options)
	}

	router := chi.NewRouter()

	if logger == nil {
		logger = slog.Default()
	}

	// Register default request logging middleware
	router.Use(httplog.RequestLogger(logger, &httplog.Options{
		Level:         slog.LevelInfo,
		RecoverPanics: true,
		Schema:        httplog.SchemaECS,
		Skip: func(req *http.Request, respStatus int) bool {
			return strings.HasPrefix(req.URL.Path, "/health")
		},
	}))

	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)

	// User-supplied middleware injected here
	for _, mw := range options.Middlewares {
		router.Use(mw)
	}

	router.Use(middleware.Timeout(options.Timeout))

	// Setup health routes if aggregator is provided
	MountHealthRoutes(router, health)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"Resource not found"}}`))
	})

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"error":{"code":"METHOD_NOT_ALLOWED","message":"Method not allowed"}}`))
	})

	return router
}

func WithLogger(logger *slog.Logger) HTTPServerOption {
	return func(s *HTTPServer) {
		s.logger = logger
	}
}

// WithReadHeaderTimeout sets the maximum time allowed to read request headers.
func WithReadHeaderTimeout(d time.Duration) HTTPServerOption {
	return func(s *HTTPServer) {
		if d > 0 {
			s.readHeaderTimeout = d
		}
	}
}

// WithReadTimeout sets the maximum time allowed to read the entire request.
func WithReadTimeout(d time.Duration) HTTPServerOption {
	return func(s *HTTPServer) {
		if d > 0 {
			s.readTimeout = d
		}
	}
}

// WithWriteTimeout sets the maximum time allowed to write the response.
func WithWriteTimeout(d time.Duration) HTTPServerOption {
	return func(s *HTTPServer) {
		if d > 0 {
			s.writeTimeout = d
		}
	}
}

// WithIdleTimeout sets the maximum time to wait for the next request on a
// keepalive connection.
func WithIdleTimeout(d time.Duration) HTTPServerOption {
	return func(s *HTTPServer) {
		if d > 0 {
			s.idleTimeout = d
		}
	}
}

// WithMaxHeaderBytes sets the maximum size of request headers.
func WithMaxHeaderBytes(n int) HTTPServerOption {
	return func(s *HTTPServer) {
		if n > 0 {
			s.maxHeaderBytes = n
		}
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
