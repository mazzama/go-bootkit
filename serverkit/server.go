package serverkit

import (
	"context"
	"encoding/json"
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

// HTTPServer wraps an http.Server as a core.Component: it binds the listener
// during Start, reports readiness, and gracefully shuts down on Stop.
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

// HTTPServerOption configures an HTTPServer at construction time.
type HTTPServerOption func(*HTTPServer)

// RouterOptions holds the tunable knobs for NewDefaultRouter.
type RouterOptions struct {
	Timeout time.Duration
	// TrustedProxies are CIDR prefixes of reverse proxies in front of this
	// server. When set, the router resolves the client IP from
	// X-Forwarded-For (right-to-left, skipping trusted hops). When empty,
	// the router falls back to the TCP peer address — safe by default:
	// client-supplied forwarding headers are never trusted.
	TrustedProxies []string
	Middlewares    []func(http.Handler) http.Handler
}

// RouterOption configures RouterOptions at router construction time.
type RouterOption func(*RouterOptions)

// Default HTTP server timeouts. These guard against slow clients and leaked
// keepalive connections exhausting the server under load.
const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultReadTimeout       = 15 * time.Second
	defaultWriteTimeout      = 15 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultMaxHeaderBytes    = 1 << 20 // 1MB
)

var _ core.Component = (*HTTPServer)(nil)

// NewHTTPServer creates an HTTPServer with sensible default timeouts. The
// handler is served on addr once Start is called.
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

		var lc net.ListenConfig
		listener, listenErr := lc.Listen(ctx, "tcp", srv.addr)
		if listenErr != nil {
			return nil, fmt.Errorf("failed to create listener: %w", listenErr)
		}
		srv.listener = listener
		srv.server.Addr = listener.Addr().String()

		go func() {
			if serveErr := srv.server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				logger.Error("HTTP server error", "name", srv.name, "error", serveErr)
			}
		}()

		logger.Info("HTTP server ready", "name", srv.name, "addr", srv.server.Addr)

		return func(stopCtx context.Context) error {
			logger.Info("Stopping HTTP server", "name", srv.name)

			if shutdownErr := srv.server.Shutdown(stopCtx); shutdownErr != nil {
				logger.Error("Error shutting down HTTP server", "name", srv.name, "error", shutdownErr)
				return shutdownErr
			}

			logger.Info("HTTP server stopped", "name", srv.name)
			return nil
		}, nil
	})

	return srv, nil
}

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

// WithTrustedProxies sets the CIDR prefixes of reverse proxies in front of
// this server. The client IP is then resolved from the X-Forwarded-For
// header, walking right-to-left and skipping trusted hops (secure against
// spoofed headers). When not set, the router trusts only the TCP peer
// address and ignores forwarding headers entirely.
//
// Invalid CIDRs fail fast at option-application time so a misconfigured
// proxy list can never silently degrade to trusting attacker-supplied
// headers.
func WithTrustedProxies(prefixes ...string) RouterOption {
	return func(o *RouterOptions) {
		for _, p := range prefixes {
			if _, _, err := net.ParseCIDR(p); err != nil {
				panic(fmt.Sprintf("serverkit: invalid trusted proxy CIDR %q: %v", p, err))
			}
		}
		o.TrustedProxies = append(o.TrustedProxies, prefixes...)
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

// NewDefaultRouter builds a chi router with request logging, request ID,
// safe client-IP resolution, timeout middleware, and the standard health
// routes. Extra middleware can be injected via WithMiddleware.
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
			return strings.HasPrefix(req.URL.Path, "/health") && respStatus < 500
		},
	}))

	router.Use(middleware.RequestID)
	// Resolve the client IP without trusting client-supplied forwarding
	// headers (GHSA-9g5q-2w5x-hmxf, GHSA-3fxj-6jh8-hvhx,
	// GHSA-rjr7-jggh-pgcp — the deprecated middleware.RealIP is vulnerable
	// to spoofing). Trusted proxies are enumerated via WithTrustedProxies;
	// with none configured, the TCP peer address is used and no header is
	// ever trusted. The resolved IP is available to handlers via
	// middleware.GetClientIP(r.Context()); r.RemoteAddr is never mutated.
	if len(options.TrustedProxies) > 0 {
		router.Use(middleware.ClientIPFromXFF(options.TrustedProxies...))
	} else {
		router.Use(middleware.ClientIPFromRemoteAddr)
	}

	// User-supplied middleware injected here
	for _, mw := range options.Middlewares {
		router.Use(mw)
	}

	router.Use(middleware.Timeout(options.Timeout))

	// Setup health routes if aggregator is provided
	MountHealthRoutes(router, health)

	router.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, http.StatusNotFound, "NOT_FOUND", "Resource not found")
	})

	router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed")
	})

	return router
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload, err := json.Marshal(map[string]any{
		"error": map[string]any{"code": code, "message": message},
	})
	if err != nil {
		_, _ = w.Write([]byte(`{"error":{"code":"INTERNAL","message":"failed to encode error response"}}`))
		return
	}
	_, _ = w.Write(payload)
}

// WithLogger replaces the server's logger. Defaults to slog.Default().
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

// Name returns the component name.
func (s *HTTPServer) Name() string {
	return s.name
}

// HealthChecks returns the standard liveness/readiness pair; readiness waits
// on the listener being bound.
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
