package serverkit

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/go-chi/chi/v5"
)

// Task 14: Test ServerKit - WebServer Creation

func TestNewWebServer(t *testing.T) {
	logger := slog.Default()
	health := healthkit.NewAggregator(5 * time.Second)

	server := NewWebServer("test-server", ":8080",
		WithWebServerLogger(logger),
		WithHealthAggregator(health),
	)

	if server == nil {
		t.Fatal("NewWebServer() = nil, want non-nil")
	}
	if server.name != "test-server" {
		t.Errorf("name = %v, want 'test-server'", server.name)
	}
	if server.addr != ":8080" {
		t.Errorf("addr = %v, want ':8080'", server.addr)
	}
	if server.logger != logger {
		t.Error("logger not set")
	}
	if server.health != health {
		t.Error("health aggregator not set")
	}
	if server.router == nil {
		t.Error("router not initialized")
	}
}

func TestNewWebServer_Defaults(t *testing.T) {
	health := healthkit.NewAggregator(0)
	server := NewWebServer("test", ":8080", WithHealthAggregator(health))

	if server.health != health {
		t.Error("health aggregator should be set when provided")
	}
	if server.logger != nil {
		t.Error("logger should be nil by default")
	}
}

// Task 15: Test ServerKit - Component Interface

func TestWebServer_ComponentInterface(t *testing.T) {
	var _ core.Component = (*WebServer)(nil)
	var _ core.Readyable = (*WebServer)(nil)
}

func TestWebServer_Name(t *testing.T) {
	server := NewWebServer("test-name", ":8080", WithHealthAggregator(healthkit.NewAggregator(0)))
	if server.Name() != "test-name" {
		t.Errorf("Name() = %v, want 'test-name'", server.Name())
	}
}

// Task 16: Test ServerKit - Router Access

func TestWebServer_Router(t *testing.T) {
	server := NewWebServer("test", ":8080", WithHealthAggregator(healthkit.NewAggregator(0)))

	router := server.Router()
	if router == nil {
		t.Fatal("Router() = nil, want non-nil")
	}

	// Verify it's a chi.Mux
	var _ chi.Router = router
}

func TestWebServer_Health(t *testing.T) {
	health := healthkit.NewAggregator(5 * time.Second)
	server := NewWebServer("test", ":8080",
		WithHealthAggregator(health),
	)

	if server.Health() != health {
		t.Error("Health() should return configured aggregator")
	}
}

// Task 17: Test ServerKit - Ready Channel

func TestWebServer_Ready(t *testing.T) {
	server := NewWebServer("test", ":8081", WithHealthAggregator(healthkit.NewAggregator(0)))

	readyCh := server.Ready()
	if readyCh == nil {
		t.Fatal("Ready() = nil, want channel")
	}

	// Start server in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Start(ctx)
	}()

	// Wait for ready signal or timeout
	select {
	case <-readyCh:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Ready() channel not closed within timeout")
	}
}

// Task 18: Test ServerKit - Start and Stop

func TestWebServer_Start_Stop(t *testing.T) {
	server := NewWebServer("test", ":8082", WithHealthAggregator(healthkit.NewAggregator(0)))

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- server.Start(ctx) }()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is responding
	resp, err := http.Get("http://localhost:8082/health/liveness")
	if err != nil {
		t.Fatalf("server not responding: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health endpoint returned %d, want 200", resp.StatusCode)
	}

	// Stop server
	cancel()
	err = <-errCh
	if err != context.Canceled {
		t.Errorf("Start() = %v, want context.Canceled", err)
	}
}

func TestWebServer_Stop(t *testing.T) {
	server := NewWebServer("test", ":8083", WithHealthAggregator(healthkit.NewAggregator(0)))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = server.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()

	err := server.Stop(stopCtx)
	if err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}

	cancel()
}

// Task 19: Test ServerKit - Health Endpoints

func TestWebServer_HealthEndpoints(t *testing.T) {
	server := NewWebServer("test", ":8084", WithHealthAggregator(healthkit.NewAggregator(0)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/health/liveness", http.StatusOK, "ok"},
		{"/health/readiness", http.StatusOK, "ok"},
		{"/health/startup", http.StatusOK, "ok"},
		{"/health", http.StatusOK, "ok"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get("http://localhost:8084" + tt.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tt.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("%s status = %d, want %d", tt.path, resp.StatusCode, tt.wantStatus)
			}

			body, _ := io.ReadAll(resp.Body)
			if string(body) != tt.wantBody {
				t.Errorf("%s body = %q, want %q", tt.path, string(body), tt.wantBody)
			}
		})
	}
}

// Task 20: Test ServerKit - Custom Middleware

func TestWebServer_CustomMiddleware(t *testing.T) {
	middlewareCalled := false
	customMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			middlewareCalled = true
			next.ServeHTTP(w, r)
		})
	}

	server := NewWebServer("test", ":8085",
		WithCustomMiddleware(customMW),
		WithHealthAggregator(healthkit.NewAggregator(0)),
	)

	// Add a test route
	server.Router().Get("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:8085/test")
	if err != nil {
		t.Fatalf("GET /test: %v", err)
	}
	resp.Body.Close()

	if !middlewareCalled {
		t.Error("custom middleware was not called")
	}
}
