package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
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

func TestApplicationRunner_Run_Success(t *testing.T) {
	comp := &TestMockComponent{name: "test-service", readyCh: make(chan struct{})}

	runner := NewApplicationRunner(
		WithServices(comp),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runner.Run(ctx)
	// Error may be nil or context.DeadlineExceeded (possibly wrapped)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() = %v, want context.DeadlineExceeded or nil", err)
	}
}

func TestApplicationRunner_Run_ComponentError(t *testing.T) {
	comp := &TestMockComponent{
		name:       "failing-service",
		startError: fmt.Errorf("start failed"),
		readyCh:    make(chan struct{}),
	}

	runner := NewApplicationRunner(WithServices(comp))

	ctx := context.Background()
	err := runner.Run(ctx)

	if err == nil {
		t.Error("Run() = nil, want error")
	}
	if !strings.Contains(err.Error(), "failing-service") {
		t.Errorf("error should contain component name, got %v", err)
	}
}

func TestApplicationRunner_GracefulShutdown(t *testing.T) {
	stopCalled := false
	comp := &StopTrackingComponent{
		TestMockComponent: TestMockComponent{
			name:    "shutdown-test",
			readyCh: make(chan struct{}),
		},
	 onStopCalled: func() {
			stopCalled = true
		},
	}

	runner := NewApplicationRunner(
		WithServices(comp),
		WithShutdownTimeout(1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	errCh := make(chan error, 1)
	go func() { errCh <- runner.Run(ctx) }()

	// Wait for start, then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-errCh
	// Run returns nil when context is canceled (implementation filters out context.Canceled)
	if err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
	if !stopCalled {
		t.Error("Stop() was not called on component")
	}
}

// StopTrackingComponent wraps TestMockComponent to track Stop calls
type StopTrackingComponent struct {
	TestMockComponent
	onStopCalled func()
}

func (s *StopTrackingComponent) Stop(ctx context.Context) error {
	if s.onStopCalled != nil {
		s.onStopCalled()
	}
	return s.TestMockComponent.Stop(ctx)
}

func TestApplicationRunner_StartDeadline(t *testing.T) {
	comp := &NeverReadyComponent{
		name: "slow-service",
	}

	runner := NewApplicationRunner(
		WithServices(comp),
		WithStartDeadline(50*time.Millisecond),
	)

	ctx := context.Background()
	err := runner.Run(ctx)

	if err == nil {
		t.Error("Run() should fail with start deadline exceeded")
	}
	if !strings.Contains(err.Error(), "start deadline exceeded") {
		t.Errorf("error should mention deadline, got %v", err)
	}
}
