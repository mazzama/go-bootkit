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

	"github.com/mazzama/go-bootkit/core/healthkit"
)

func TestNewHTTPServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := NewHTTPServer("test-server", ":8080", handler)

	if srv.Name() != "test-server" {
		t.Errorf("expected name 'test-server', got %q", srv.Name())
	}
}

func TestWithLogger(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewHTTPServer("test-server", ":8080", handler, WithLogger(logger))

	if srv.logger != logger {
		t.Error("expected logger to match WithLogger option")
	}
}

func TestHealthChecks(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	srv := NewHTTPServer("health-server", "127.0.0.1:0", handler)

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
		_, _ = w.Write([]byte("ok")) //nolint:errcheck
	})
	srv := NewHTTPServer("start-stop-server", "127.0.0.1:0", handler)

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
	resp.Body.Close()
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

func TestNewDefaultHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	agg := healthkit.NewAggregator(0)
	handler := NewDefaultHandler(agg, logger)

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

	// Verify the injected logger was used by httplog middleware
	if !strings.Contains(buf.String(), "/health/liveness") {
		t.Errorf("expected injected logger to capture the request, got: %s", buf.String())
	}
}
