package serverkit

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

func newTestAggregator() *healthkit.Aggregator {
	return healthkit.NewAggregator(0)
}

func TestNewWebServer(t *testing.T) {
	agg := newTestAggregator()
	ws := NewWebServer("test-server", ":8080", WithHealthAggregator(agg))

	if ws.Name() != "test-server" {
		t.Errorf("expected name 'test-server', got %q", ws.Name())
	}
	if ws.Router() == nil {
		t.Error("expected router to be non-nil")
	}
	if ws.Health() == nil {
		t.Error("expected health aggregator to be non-nil")
	}
	if ws.Health() != agg {
		t.Error("expected health aggregator to be the one provided via option")
	}
}

func TestWithWebServerLogger(t *testing.T) {
	agg := newTestAggregator()
	logger := slog.Default()
	ws := NewWebServer("log-server", ":8080",
		WithHealthAggregator(agg),
		WithWebServerLogger(logger),
	)

	if ws.logger == nil {
		t.Error("expected logger to be set")
	}
	if ws.logger != logger {
		t.Error("expected logger to match the one provided")
	}
}

func TestWithCustomMiddleware(t *testing.T) {
	agg := newTestAggregator()

	var called atomic.Bool
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called.Store(true)
			next.ServeHTTP(w, r)
		})
	}

	ws := NewWebServer("mw-server", ":8080",
		WithHealthAggregator(agg),
		WithCustomMiddleware(mw),
	)

	ws.Router().Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	ws.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !called.Load() {
		t.Error("expected custom middleware to be called")
	}
}

func TestHealthEndpointsRegistered(t *testing.T) {
	agg := newTestAggregator()
	ws := NewWebServer("health-server", ":8080", WithHealthAggregator(agg))

	endpoints := []string{
		"/health",
		"/health/liveness",
		"/health/readiness",
		"/health/startup",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, ep, nil)
			rec := httptest.NewRecorder()
			ws.Router().ServeHTTP(rec, req)

			// We just check it doesn't 404/405
			if rec.Code == http.StatusNotFound || rec.Code == http.StatusMethodNotAllowed {
				t.Errorf("endpoint %s returned %d, expected it to be registered", ep, rec.Code)
			}
		})
	}
}

func TestLivenessEndpointReturns200(t *testing.T) {
	agg := newTestAggregator()
	ws := NewWebServer("liveness-server", ":8080", WithHealthAggregator(agg))

	req := httptest.NewRequest(http.MethodGet, "/health/liveness", nil)
	rec := httptest.NewRecorder()
	ws.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if body != "ok" {
		t.Errorf("expected body 'ok', got %q", body)
	}
}

func TestReadinessEndpointReturns503BeforeStart(t *testing.T) {
	agg := newTestAggregator()
	ws := NewWebServer("readiness-server", ":8080", WithHealthAggregator(agg))

	req := httptest.NewRequest(http.MethodGet, "/health/readiness", nil)
	rec := httptest.NewRecorder()
	ws.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestWithHealthAggregatorIgnoresNil(t *testing.T) {
	agg := newTestAggregator()
	ws := NewWebServer("nil-health-server", ":8080",
		WithHealthAggregator(agg),
		WithHealthAggregator(nil), // should not replace existing aggregator
	)

	if ws.Health() == nil {
		t.Error("expected health aggregator to remain non-nil after nil override")
	}
	if ws.Health() != agg {
		t.Error("expected health aggregator to still be the original one")
	}
}

func TestStartAndStop(t *testing.T) {
	agg := newTestAggregator()
	ws := NewWebServer("start-stop-server", "127.0.0.1:0",
		WithHealthAggregator(agg),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ws.Start(ctx)
	}()

	// Wait for context to expire
	<-ctx.Done()

	// Verify Start returns context error
	startErr := <-errCh
	if startErr != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded from Start, got %v", startErr)
	}

	// Verify liveness endpoint works while server is up (before Stop)
	resp, err := http.Get("http://" + ws.server.Addr + "/health/liveness")
	if err == nil {
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	// Stop should not error
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := ws.Stop(stopCtx); err != nil {
		t.Errorf("expected Stop to succeed, got %v", err)
	}
}
