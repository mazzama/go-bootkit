package core

import (
	"log/slog"
	"testing"
	"time"
)

func TestNewApplicationRunner(t *testing.T) {
	logger := slog.Default()

	runner := NewApplicationRunner(
		WithLogger(logger),
		WithShutdownTimeout(30*time.Second),
		WithStartDeadline(10*time.Second),
	)

	if runner == nil {
		t.Fatal("NewApplicationRunner() = nil, want non-nil")
	}
	if runner.logger != logger {
		t.Error("WithLogger() not applied")
	}
	if runner.shutdownTimeout != 30*time.Second {
		t.Errorf("WithShutdownTimeout() = %v, want 30s", runner.shutdownTimeout)
	}
	if runner.startDeadline != 10*time.Second {
		t.Errorf("WithStartDeadline() = %v, want 10s", runner.startDeadline)
	}
}

func TestNewApplicationRunner_Defaults(t *testing.T) {
	runner := NewApplicationRunner()

	if runner.shutdownTimeout != 15*time.Second {
		t.Errorf("default shutdownTimeout = %v, want 15s", runner.shutdownTimeout)
	}
	if runner.logger != nil {
		t.Error("default logger should be nil")
	}
}
