// Package serverkit provides an HTTP server component with health checks.
//
// The WebServer implements the core.Component interface and integrates with
// the healthkit package to provide Kubernetes-style health endpoints. It uses
// the chi router for HTTP routing and includes built-in middleware for
// request logging, recovery, and timeouts.
//
// # Features
//
//   - Chi router with customizable middleware chain
//   - Built-in health endpoints (liveness, readiness, startup)
//   - Structured logging integration (slog)
//   - Graceful shutdown
//   - Automatic readiness probing
//
// # Health Endpoints
//
// The WebServer automatically registers health checks and exposes HTTP endpoints:
//   - GET /health/liveness - Returns 200 if server is running
//   - GET /health/readiness - Returns 200 when server is ready to accept connections
//   - GET /health/startup - Returns 200 when server has completed startup
//   - GET /health - Alias for liveness
//
// # Middleware
//
// Default middleware (in order):
//  1. Request logging (if logger provided)
//  2. Request ID header
//  3. Real IP extraction
//  4. Panic recovery
//  5. Request timeout (60 seconds)
//  6. Custom middleware (via WithCustomMiddleware)
//
// Example:
//
//	server := serverkit.NewWebServer("api", ":8080",
//	    serverkit.WithWebServerLogger(logger),
//	    serverkit.WithHealthAggregator(health),
//	)
//
//	// Register routes
//	server.Router().Get("/users", usersHandler)
//
//	// Run with ApplicationRunner
//	runner := core.NewApplicationRunner(
//	    core.WithServices(server),
//	)
package serverkit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/httplog/v3"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

// WebServer is an HTTP server component that implements core.Component.
//
// The WebServer wraps an http.Server with a chi.Mux router, providing
// built-in health check endpoints and middleware. It automatically
// probes its own readiness by checking the liveness endpoint after startup.
//
// # Lifecycle
//
// On Start, the WebServer begins listening on the configured address and
// spawns a goroutine that probes the /health/liveness endpoint. When the
// probe succeeds, the Ready() channel is closed.
//
// On Stop, the server gracefully shuts down, waiting for active connections
// to complete (up to the context deadline).
//
// # Health Integration
//
// If a healthkit.Aggregator is provided via WithHealthAggregator, the server
// registers its own health checks. Custom health checks can be registered
// by accessing the aggregator via Health().
//
// Example:
//
//	server := NewWebServer("api", ":8080",
//	    NewWebServerLogger(logger),
//	    WithHealthAggregator(health),
//	)
//	server.Router().Get("/api/users", usersHandler)
type WebServer struct {
	name              string
	addr              string
	server            *http.Server
	router            *chi.Mux
	health            *healthkit.Aggregator
	logger            *slog.Logger
	readyCh           chan struct{}
	shutdownCh        chan struct{}
	customMiddlewares []func(http.Handler) http.Handler
	requestTimeout    time.Duration
}

// WebServerOption is a functional option for configuring a WebServer.
//
// Options are applied in the order provided to NewWebServer. See the With*
// functions for available options.
type WebServerOption func(*WebServer)

// NewWebServer creates a new HTTP server component.
//
// The server is initialized with a chi.Mux router and default middleware.
// Health endpoints are registered automatically. The server begins listening
// when Start is called.
//
// The name parameter is used for logging and health check names. The addr
// parameter is the listen address (e.g., ":8080", "0.0.0.0:8080").
//
// Example:
//
//	server := NewWebServer("api", ":8080",
//	    WithWebServerLogger(logger),
//	    WithHealthAggregator(health),
//	)
func NewWebServer(name, addr string, options ...WebServerOption) *WebServer {
	router := chi.NewRouter()

	ws := &WebServer{
		name:           name,
		addr:           addr,
		router:         router,
		readyCh:        make(chan struct{}),
		shutdownCh:     make(chan struct{}),
		requestTimeout: 60 * time.Second,
	}

	for _, option := range options {
		option(ws)
	}

	if ws.health == nil {
		ws.health = healthkit.NewAggregator(0)
	}

	ws.setupMiddleware()
	ws.setupHealthEndpoints()
	ws.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	return ws
}

// WithWebServerLogger sets the structured logger for the server.
//
// The logger is used for request logging and lifecycle events. If provided,
// middleware is added to log each HTTP request with ECS-formatted fields.
//
// Example:
//
//	logger := slog.Default()
//	server := NewWebServer("api", ":8080",
//	    WithWebServerLogger(logger),
//	)
func WithWebServerLogger(logger *slog.Logger) WebServerOption {
	return func(ws *WebServer) {
		ws.logger = logger
	}
}

// WithCustomMiddleware adds custom middleware to the server's middleware chain.
//
// Custom middleware are applied after the default middleware (logging, request ID,
// real IP, recovery, timeout) but before the route handlers. Multiple
// WithCustomMiddleware options can be provided.
//
// Example:
//
//	server := NewWebServer("api", ":8080",
//	    WithCustomMiddleware(authMiddleware, corsMiddleware),
//	)
func WithCustomMiddleware(middlewares ...func(http.Handler) http.Handler) WebServerOption {
	return func(ws *WebServer) {
		ws.customMiddlewares = append(ws.customMiddlewares, middlewares...)
	}
}

// WithHealthAggregator sets the health check aggregator.
//
// If provided, the server registers its health checks with this aggregator
// instead of creating a new one. This allows multiple components to share
// the same health check aggregation.
//
// Example:
//
//	health := healthkit.NewAggregator(5 * time.Second)
//	server := NewWebServer("api", ":8080",
//	    WithHealthAggregator(health),
//	)
func WithHealthAggregator(health *healthkit.Aggregator) WebServerOption {
	return func(ws *WebServer) {
		if health != nil {
			ws.health = health
		}
	}
}

// WithRequestTimeout sets the per-request timeout for the server's timeout middleware.
//
// If d is zero or negative, the default timeout (60 seconds) is retained.
// The timeout applies to all incoming requests via the chi Timeout middleware.
//
// Example:
//
//	server := NewWebServer("api", ":8080",
//	    WithRequestTimeout(30 * time.Second),
//	)
func WithRequestTimeout(d time.Duration) WebServerOption {
	return func(ws *WebServer) {
		if d > 0 {
			ws.requestTimeout = d
		}
	}
}

// Name returns the server's identifier.
func (ws *WebServer) Name() string {
	return ws.name
}

// Router returns the chi.Mux router for registering routes.
//
// Use this method to add routes, middleware, and route groups to the server.
//
// Example:
//
//	server.Router().Get("/users", usersHandler)
//	server.Router().Route("/api", func(r chi.Router) {
//	    r.Get("/items", itemsHandler)
//	    r.Post("/items", createItemHandler)
//	})
func (ws *WebServer) Router() *chi.Mux {
	return ws.router
}

// Health returns the health check aggregator.
//
// Use this to register additional health checks or access the aggregator
// for custom health endpoints.
//
// Example:
//
//	server.Health().Register(healthkit.Check{
//	    Name: "custom",
//	    Kind: healthkit.Liveness,
//	    Fn: func(ctx context.Context) error {
//	        // check logic
//	        return nil
//	    },
//	})
func (ws *WebServer) Health() *healthkit.Aggregator {
	return ws.health
}

// Ready returns a channel that closes when the server is ready.
//
// The channel closes after the server successfully starts and the
// liveness probe succeeds. If the server fails to start or the probe
// fails, the channel may never close.
func (ws *WebServer) Ready() <-chan struct{} {
	return ws.readyCh
}

// Start begins serving HTTP requests.
//
// Start launches the HTTP server in a goroutine and spawns another goroutine
// that probes the /health/liveness endpoint. When the probe succeeds, the
// Ready() channel is closed.
//
// Start blocks until the provided context is canceled, at which point it
// returns (the actual shutdown happens in the Stop method).
//
// Example:
//
//	// Typically called by ApplicationRunner
//	err := server.Start(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
func (ws *WebServer) Start(ctx context.Context) error {
	if ws.logger != nil {
		ws.logger.Info("Starting web server", "name", ws.name, "addr", ws.addr)
	}

	go func() {
		if err := ws.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			if ws.logger != nil {
				ws.logger.Error("Web server error", "name", ws.name, "error", err)
			}
			return
		}
	}()

	go func() {
		client := &http.Client{Timeout: 1 * time.Second}
		for attempt := 1; attempt <= 10; attempt++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}

			resp, err := client.Get(fmt.Sprintf("http://%s/health/liveness", ws.addr))
			if err == nil {
				resp.Body.Close()
				close(ws.readyCh)
				if ws.logger != nil {
					ws.logger.Info("Web server ready", "name", ws.name, "attempt", attempt)
				}
				return
			}
		}

		if ws.logger != nil {
			ws.logger.Warn("Web server readiness probe failed after all attempts", "name", ws.name, "attempts", 10)
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}

// Stop gracefully shuts down the server.
//
// Stop initiates a graceful shutdown, waiting for active connections to complete
// up to the context deadline. After the deadline, remaining connections are
// forcefully closed.
//
// This method is typically called by ApplicationRunner during application
// shutdown.
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	if err := server.Stop(ctx); err != nil {
//	    log.Printf("Shutdown error: %v", err)
//	}
func (ws *WebServer) Stop(ctx context.Context) error {
	if ws.logger != nil {
		ws.logger.Info("Stopping web server", "name", ws.name)
	}

	if err := ws.server.Shutdown(ctx); err != nil {
		if ws.logger != nil {
			ws.logger.Error("Error shutting down web server", "name", ws.name, "error", err)
		}
		return err
	}

	close(ws.shutdownCh)
	if ws.logger != nil {
		ws.logger.Info("Web server stopped", "name", ws.name)
	}
	return nil
}

func (ws *WebServer) setupMiddleware() {
	if ws.logger != nil {
		ws.router.Use(httplog.RequestLogger(ws.logger, &httplog.Options{
			Level:         slog.LevelInfo,
			RecoverPanics: true,
			Schema:        httplog.SchemaECS,
		}))
	}

	ws.router.Use(middleware.RequestID)
	ws.router.Use(middleware.RealIP)
	ws.router.Use(middleware.Recoverer)
	ws.router.Use(middleware.Timeout(ws.requestTimeout))

	for _, mw := range ws.customMiddlewares {
		ws.router.Use(mw)
	}
}

func (ws *WebServer) setupHealthEndpoints() {
	ws.health.Register(healthkit.Check{
		Name: "server",
		Kind: healthkit.Liveness,
		Fn: func(ctx context.Context) error {
			return nil
		},
	},
		healthkit.Check{
			Name: "server",
			Kind: healthkit.Readiness,
			Fn: func(ctx context.Context) error {
				select {
				case <-ws.readyCh:
					return nil
				default:
					return fmt.Errorf("server not ready")
				}
			},
		})

	ws.router.Get("/health/liveness", ws.health.Handler(healthkit.Liveness))
	ws.router.Get("/health/readiness", ws.health.Handler(healthkit.Readiness))
	ws.router.Get("/health/startup", ws.health.Handler(healthkit.Startup))
	ws.router.Get("/health", ws.health.Handler(healthkit.Liveness))
}
