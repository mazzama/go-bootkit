package core

import (
	"context"
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

func TestWithServices(t *testing.T) {
	comp1 := &TestMockComponent{name: "service1", readyCh: make(chan struct{})}
	comp2 := &TestMockComponent{name: "service2", readyCh: make(chan struct{})}

	runner := NewApplicationRunner(
		WithServices(comp1),
		WithServices(comp2),
	)

	if len(runner.services) != 2 {
		t.Errorf("WithServices() added %d services, want 2", len(runner.services))
	}
}

func TestApplicationRunner_Run_StartsServices(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	comp1 := &TestMockComponent{
		name:    "svc1",
		readyCh: make(chan struct{}),
	}
	comp2 := &TestMockComponent{
		name:    "svc2",
		readyCh: make(chan struct{}),
	}

	go func() {
		runner := NewApplicationRunner(WithServices(comp1, comp2))
		_ = runner.Run(ctx)
	}()

	// Wait a bit for services to start
	time.Sleep(50 * time.Millisecond)
}
