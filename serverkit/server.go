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
}

type WebServerOption func(*WebServer)

func NewWebServer(name, addr string, options ...WebServerOption) *WebServer {
	router := chi.NewRouter()

	ws := &WebServer{
		name:       name,
		addr:       addr,
		router:     router,
		readyCh:    make(chan struct{}),
		shutdownCh: make(chan struct{}),
	}

	for _, option := range options {
		option(ws)
	}

	ws.setupMiddleware()
	ws.setupHealthEndpoints()
	ws.server = &http.Server{
		Addr:    addr,
		Handler: router,
	}

	return ws
}

func WithWebServerLogger(logger *slog.Logger) WebServerOption {
	return func(ws *WebServer) {
		ws.logger = logger
	}
}

func WithCustomMiddleware(middlewares ...func(http.Handler) http.Handler) WebServerOption {
	return func(ws *WebServer) {
		ws.customMiddlewares = append(ws.customMiddlewares, middlewares...)
	}
}

func WithHealthAggregator(health *healthkit.Aggregator) WebServerOption {
	return func(ws *WebServer) {
		if health != nil {
			ws.health = health
		}
	}
}

func (ws *WebServer) Name() string {
	return ws.name
}

func (ws *WebServer) Router() *chi.Mux {
	return ws.router
}

func (ws *WebServer) Health() *healthkit.Aggregator {
	return ws.health
}

func (ws *WebServer) Ready() <-chan struct{} {
	return ws.readyCh
}

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
		time.Sleep(100 * time.Millisecond)
		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get(fmt.Sprintf("http://%s/health/liveness", ws.addr))
		if err == nil {
			resp.Body.Close()
			close(ws.readyCh)
			if ws.logger != nil {
				ws.logger.Info("Web server ready", "name", ws.name)
			}
		} else {
			if ws.logger != nil {
				ws.logger.Error("Web server not ready", "name", ws.name, "error", err)
			}
		}
	}()

	<-ctx.Done()
	return ctx.Err()
}

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
	ws.router.Use(middleware.Timeout(60 * time.Second))

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
