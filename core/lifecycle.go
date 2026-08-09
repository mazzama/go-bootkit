package core

import (
	"context"
	"errors"
	"sync"
)

// Lifecycle provides an embeddable primitive for managing component lifecycle.
// It handles readiness signalling, blocking until context cancellation, and
// guarantees exactly-once stopping semantics.
type Lifecycle struct {
	// Connect is called by Start to acquire the resource. It should return a
	// stop closure to release the resource, and an error if connection fails.
	// The stop closure receives the shutdown context so it can respect the
	// caller's deadline, and returns an error if resource release fails.
	Connect func(ctx context.Context) (stop func(ctx context.Context) error, err error)

	mu        sync.Mutex
	ready     chan struct{}
	readyOnce sync.Once
	stopFunc  func(context.Context) error
	stopped   bool
}

// NewLifecycle creates a new Lifecycle with the provided connect function.
// It is returned as a value to be easily embedded in other structs.
func NewLifecycle(connect func(ctx context.Context) (func(context.Context) error, error)) Lifecycle {
	return Lifecycle{
		Connect: connect,
	}
}

func (l *Lifecycle) initReady() {
	l.readyOnce.Do(func() {
		l.ready = make(chan struct{})
	})
}

// Start executes the Connect closure. On success, it signals ready and blocks
// until ctx.Done(). On failure, it returns the error and leaves the ready channel unsignalled.
func (l *Lifecycle) Start(ctx context.Context) error {
	l.initReady()

	if l.Connect == nil {
		return errors.New("core.Lifecycle: Connect closure is not set")
	}

	stop, err := l.Connect(ctx)
	if err != nil {
		return err
	}

	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		if stop != nil {
			return stop(ctx)
		}
		return nil
	}
	l.stopFunc = stop
	l.mu.Unlock()

	close(l.ready)

	<-ctx.Done()
	return ctx.Err()
}

// Stop invokes the stop closure returned by Connect at most once.
// It forwards the provided context to the stop closure so the closure
// can respect the caller's shutdown deadline. It returns the closure's error.
// It is safe to call on a never-started or half-started instance.
func (l *Lifecycle) Stop(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.stopped {
		return nil
	}
	l.stopped = true

	if l.stopFunc != nil {
		return l.stopFunc(ctx)
	}
	return nil
}

// Ready returns a channel that is closed when the component has successfully connected.
func (l *Lifecycle) Ready() <-chan struct{} {
	l.initReady()
	return l.ready
}

var _ Readyable = (*Lifecycle)(nil)
