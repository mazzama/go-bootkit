package serverkit

import (
	"log/slog"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
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
