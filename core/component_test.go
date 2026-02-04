package core

import (
	"context"
	"testing"
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
	// Verify TestMockComponent satisfies Component interface
	var _ Component = (*TestMockComponent)(nil)
}

func TestReadyableInterface(t *testing.T) {
	// Verify TestMockComponent satisfies Readyable interface when readyCh is set
	comp := &TestMockComponent{readyCh: make(chan struct{})}
	var _ Readyable = comp
}
