package serverkit

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

func TestNewHTTPServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv, _ := NewHTTPServer("test-server", ":8080", handler)

	if srv.Name() != "test-server" {
		t.Errorf("expected name 'test-server', got %q", srv.Name())
	}
}

func TestWithLogger(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, _ := NewHTTPServer("test-server", ":8080", handler, WithLogger(logger))

	if srv.logger != logger {
		t.Error("expected logger to match WithLogger option")
	}
}

func TestTimeoutDefaults(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv, _ := NewHTTPServer("test-server", ":8080", handler)

	if srv.server.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("expected ReadHeaderTimeout 5s, got %v", srv.server.ReadHeaderTimeout)
	}
	if srv.server.ReadTimeout != 15*time.Second {
		t.Errorf("expected ReadTimeout 15s, got %v", srv.server.ReadTimeout)
	}
	if srv.server.WriteTimeout != 15*time.Second {
		t.Errorf("expected WriteTimeout 15s, got %v", srv.server.WriteTimeout)
	}
	if srv.server.IdleTimeout != 60*time.Second {
		t.Errorf("expected IdleTimeout 60s, got %v", srv.server.IdleTimeout)
	}
	if srv.server.MaxHeaderBytes != 1<<20 {
		t.Errorf("expected MaxHeaderBytes 1MB, got %d", srv.server.MaxHeaderBytes)
	}
}

func TestTimeoutOverrides(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv, _ := NewHTTPServer("test-server", ":8080", handler,
		WithReadHeaderTimeout(1*time.Second),
		WithReadTimeout(2*time.Second),
		WithWriteTimeout(3*time.Second),
		WithIdleTimeout(4*time.Second),
		WithMaxHeaderBytes(2048),
	)

	if srv.server.ReadHeaderTimeout != 1*time.Second {
		t.Errorf("expected ReadHeaderTimeout 1s, got %v", srv.server.ReadHeaderTimeout)
	}
	if srv.server.ReadTimeout != 2*time.Second {
		t.Errorf("expected ReadTimeout 2s, got %v", srv.server.ReadTimeout)
	}
	if srv.server.WriteTimeout != 3*time.Second {
		t.Errorf("expected WriteTimeout 3s, got %v", srv.server.WriteTimeout)
	}
	if srv.server.IdleTimeout != 4*time.Second {
		t.Errorf("expected IdleTimeout 4s, got %v", srv.server.IdleTimeout)
	}
	if srv.server.MaxHeaderBytes != 2048 {
		t.Errorf("expected MaxHeaderBytes 2048, got %d", srv.server.MaxHeaderBytes)
	}
}

func TestHealthChecks(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv, _ := NewHTTPServer("health-server", "127.0.0.1:0", handler)

	checks := srv.HealthChecks()
	if len(checks) != 2 {
		t.Fatalf("expected 2 health checks, got %d", len(checks))
	}

	// Liveness
	if checks[0].Name != "health-server-liveness" {
		t.Errorf("expected liveness name 'health-server-liveness', got %q", checks[0].Name)
	}
	if checks[0].Kind != healthkit.Liveness {
		t.Error("expected liveness kind")
	}
	if err := checks[0].Fn(t.Context()); err != nil {
		t.Errorf("expected liveness check to pass, got error: %v", err)
	}

	// Readiness before start
	if checks[1].Name != "health-server-readiness" {
		t.Errorf("expected readiness name 'health-server-readiness', got %q", checks[1].Name)
	}
	if checks[1].Kind != healthkit.Readiness {
		t.Error("expected readiness kind")
	}
	if err := checks[1].Fn(t.Context()); err == nil {
		t.Error("expected readiness check to fail before start")
	}

	// Simulate ready by starting it in the background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = srv.Start(ctx)
	}()

	select {
	case <-srv.Ready():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for server to be ready")
	}

	if err := checks[1].Fn(t.Context()); err != nil {
		t.Errorf("expected readiness check to pass after start, got error: %v", err)
	}
}

func TestStartAndStop(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv, _ := NewHTTPServer("start-stop-server", "127.0.0.1:0", handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// Wait for server to be ready
	select {
	case <-srv.Ready():
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for server to be ready")
	}

	// Test requesting the handler
	resp, err := http.Get("http://" + srv.server.Addr)
	if err != nil {
		t.Fatalf("failed to make HTTP request to server: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("expected 'ok', got %q", body)
	}

	// Cancel the context to stop the Start loop (simulating ApplicationRunner behavior)
	cancel()

	// Start should exit with context.Canceled error
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled from Start, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Start did not exit after context cancellation")
	}

	// Stop the server
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer stopCancel()
	if err := srv.Stop(stopCtx); err != nil {
		t.Errorf("expected Stop to succeed, got %v", err)
	}
}

func TestNewDefaultRouter(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	agg := healthkit.NewAggregator(0)
	handler := NewDefaultRouter(agg, logger)

	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	}

	// Verify health routes are registered by calling the handler directly
	req := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK from health endpoint, got %d", rec.Code)
	}

	// Verify health routes are skipped by the logger
	if buf.Len() > 0 {
		t.Errorf("expected health probes to not be logged, got: %s", buf.String())
	}

	// Verify normal routes ARE logged
	handler.Get("/test-route", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req = httptest.NewRequest(http.MethodGet, "/test-route", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !strings.Contains(buf.String(), "/test-route") {
		t.Errorf("expected injected logger to capture normal requests, got: %s", buf.String())
	}
}

func TestDefaultRouter_NotFound(t *testing.T) {
	handler := NewDefaultRouter(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 Not Found, got %d", rec.Code)
	}

	expectedType := "application/json"
	if got := rec.Header().Get("Content-Type"); got != expectedType {
		t.Errorf("expected Content-Type %q, got %q", expectedType, got)
	}

	expectedBody := `{"error":{"code":"NOT_FOUND","message":"Resource not found"}}`
	if strings.TrimSpace(rec.Body.String()) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, rec.Body.String())
	}
}

func TestDefaultRouter_MethodNotAllowed(t *testing.T) {
	handler := NewDefaultRouter(nil, nil)

	// Mount a GET route
	handler.Get("/test", func(w http.ResponseWriter, r *http.Request) {})

	// Request it with POST
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", rec.Code)
	}

	expectedType := "application/json"
	if got := rec.Header().Get("Content-Type"); got != expectedType {
		t.Errorf("expected Content-Type %q, got %q", expectedType, got)
	}

	expectedBody := `{"error":{"code":"METHOD_NOT_ALLOWED","message":"Method not allowed"}}`
	if strings.TrimSpace(rec.Body.String()) != expectedBody {
		t.Errorf("expected body %q, got %q", expectedBody, rec.Body.String())
	}
}

func TestMountHealthRoutes(t *testing.T) {
	agg := healthkit.NewAggregator(0)
	router := chi.NewRouter()
	MountHealthRoutes(router, agg)

	// Verify all health endpoints are mounted
	endpoints := []string{"/health/liveness", "/health/readiness", "/health/startup", "/health"}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodGet, ep, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200 OK, got %d", ep, rec.Code)
		}
	}
}

func TestMountHealthRoutes_NilAggregator(t *testing.T) {
	router := chi.NewRouter()
	MountHealthRoutes(router, nil) // should not panic

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 with nil aggregator, got %d", rec.Code)
	}
}

func TestNewHTTPServerValidation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	tests := []struct {
		name    string
		addr    string
		handler http.Handler
		wantErr bool
	}{
		{
			name:    "valid",
			addr:    ":8080",
			handler: handler,
			wantErr: false,
		},
		{
			name:    "empty addr",
			addr:    "",
			handler: handler,
			wantErr: true,
		},
		{
			name:    "nil handler",
			addr:    ":8080",
			handler: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHTTPServer("test", tt.addr, tt.handler)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHTTPServer() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewDefaultRouterWithMiddleware(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	agg := healthkit.NewAggregator(0)

	recordingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Custom-Middleware", "applied")
			next.ServeHTTP(w, r)
		})
	}

	handler := NewDefaultRouter(agg, logger, WithMiddleware(recordingMiddleware))

	req := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK from health endpoint, got %d", rec.Code)
	}

	if rec.Header().Get("X-Custom-Middleware") != "applied" {
		t.Errorf("expected custom middleware to be applied, got header: %s", rec.Header().Get("X-Custom-Middleware"))
	}
}
func TestWithRouterTimeout(t *testing.T) {
	opts := &RouterOptions{}
	WithRouterTimeout(5 * time.Second)(opts)
	if opts.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", opts.Timeout)
	}

	// Should not set if d <= 0
	WithRouterTimeout(0)(opts)
	if opts.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", opts.Timeout)
	}
}
