package core

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// NeverReadyComponent implements Component but never signals ready
type NeverReadyComponent struct {
	name string
}

func (n *NeverReadyComponent) Name() string {
	return n.name
}

func (n *NeverReadyComponent) Start(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (n *NeverReadyComponent) Stop(ctx context.Context) error {
	return nil
}

func (n *NeverReadyComponent) Ready() <-chan struct{} {
	ch := make(chan struct{})
	return ch // Never closed
}

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

func TestApplicationRunner_GracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	comp := &TestMockComponent{
		name:    "test-svc",
		readyCh: make(chan struct{}),
	}

	go func() {
		runner := NewApplicationRunner(
			WithServices(comp),
			WithShutdownTimeout(100*time.Millisecond),
		)
		_ = runner.Run(ctx)
	}()

	// Let service start
	time.Sleep(50 * time.Millisecond)
	cancel()

	// Wait for graceful shutdown
	time.Sleep(200 * time.Millisecond)
}

func TestApplicationRunner_StartDeadline(t *testing.T) {
	ctx := context.Background()

	slowComp := &NeverReadyComponent{
		name: "slow-svc",
	}

	runner := NewApplicationRunner(
		WithServices(slowComp),
		WithStartDeadline(50*time.Millisecond),
	)

	err := runner.Run(ctx)
	if err == nil {
		t.Error("Run() should return error when start deadline exceeded")
	}
}
