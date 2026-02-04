package core

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestMockComponent implements Component for testing
type TestMockComponent struct {
	name       string
	startError error
	stopError  error
	readyCh    chan struct{}
}

func (m *TestMockComponent) Name() string {
	return m.name
}

func (m *TestMockComponent) Start(ctx context.Context) error {
	if m.startError != nil {
		return m.startError
	}
	if m.readyCh != nil {
		close(m.readyCh)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (m *TestMockComponent) Stop(ctx context.Context) error {
	if m.stopError != nil {
		return m.stopError
	}
	return nil
}

func (m *TestMockComponent) Ready() <-chan struct{} {
	return m.readyCh
}

func TestComponentInterface(t *testing.T) {
	// Compile-time interface check
	var _ Component = (*TestMockComponent)(nil)

	t.Run("Name returns component name", func(t *testing.T) {
		comp := &TestMockComponent{name: "test-component"}
		if got := comp.Name(); got != "test-component" {
			t.Errorf("Name() = %v, want %v", got, "test-component")
		}
	})

	t.Run("Start returns error when configured", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := fmt.Errorf("start failed")
		comp := &TestMockComponent{startError: expectedErr}

		if err := comp.Start(ctx); err != expectedErr {
			t.Errorf("Start() error = %v, want %v", err, expectedErr)
		}
	})

	t.Run("Start blocks until context canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		comp := &TestMockComponent{}

		started := make(chan struct{})
		go func() {
			comp.Start(ctx)
			close(started)
		}()

		cancel()
		<-started
	})

	t.Run("Stop returns error when configured", func(t *testing.T) {
		expectedErr := fmt.Errorf("stop failed")
		comp := &TestMockComponent{stopError: expectedErr}

		if err := comp.Stop(context.Background()); err != expectedErr {
			t.Errorf("Stop() error = %v, want %v", err, expectedErr)
		}
	})
}

func TestReadyableInterface(t *testing.T) {
	// Compile-time interface check
	comp := &TestMockComponent{readyCh: make(chan struct{})}
	var _ Readyable = comp

	t.Run("Ready returns ready channel", func(t *testing.T) {
		readyCh := make(chan struct{})
		comp := &TestMockComponent{readyCh: readyCh}

		if got := comp.Ready(); got != readyCh {
			t.Errorf("Ready() channel != expected channel")
		}
	})

	t.Run("Ready closes channel on Start", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		readyCh := make(chan struct{})
		comp := &TestMockComponent{readyCh: readyCh}

		go comp.Start(ctx)

		select {
		case <-readyCh:
			// Success - channel closed
		case <-time.After(50 * time.Millisecond):
			t.Error("Ready channel not closed after Start")
		}
	})

	t.Run("Ready returns nil when not configured", func(t *testing.T) {
		comp := &TestMockComponent{}
		if got := comp.Ready(); got != nil {
			t.Errorf("Ready() = %v, want nil", got)
		}
	})
}
