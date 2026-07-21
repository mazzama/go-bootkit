package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLifecycle_ReadyAfterConnect(t *testing.T) {
	connectCalled := false
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		connectCalled = true
		return func(context.Context) error { return nil }, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- lc.Start(ctx)
	}()

	select {
	case <-lc.Ready():
		assert.True(t, connectCalled)
	case <-time.After(1 * time.Second):
		t.Fatal("Ready() did not fire")
	}

	cancel()
	err := <-errCh
	assert.ErrorIs(t, err, context.Canceled)
}

func TestLifecycle_StartBlocksUntilCtxCancel(t *testing.T) {
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- lc.Start(ctx)
	}()

	<-lc.Ready()

	// Ensure it's still blocking
	select {
	case <-errCh:
		t.Fatal("Start returned before context cancellation")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}

	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(1 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}

func TestLifecycle_ConnectErrorPath(t *testing.T) {
	expectedErr := errors.New("connect failed")
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return nil, expectedErr
	})

	ctx := context.Background()
	err := lc.Start(ctx)
	assert.ErrorIs(t, err, expectedErr)

	// Ensure Ready is not closed
	select {
	case <-lc.Ready():
		t.Fatal("Ready() fired despite connect error")
	default:
		// Expected
	}
}

func TestLifecycle_StopReleasesOnce(t *testing.T) {
	stopCount := 0
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error {
			stopCount++
			return nil
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = lc.Start(ctx)
	}()

	<-lc.Ready()

	err := lc.Stop(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, stopCount)

	// Call stop again
	err = lc.Stop(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, stopCount, "Stop closure should only be called once")
}

func TestLifecycle_StopSafeWhenNeverStarted(t *testing.T) {
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	})

	err := lc.Stop(context.Background())
	assert.NoError(t, err)

	// Ready should never fire
	select {
	case <-lc.Ready():
		t.Fatal("Ready() fired on never-started instance")
	default:
	}
}

func TestLifecycle_StopSafeWhenHalfStarted(t *testing.T) {
	stopInvoked := make(chan struct{})
	connectStarted := make(chan struct{})

	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		close(connectStarted)
		// block a little bit to ensure Stop() is called during connect
		time.Sleep(50 * time.Millisecond)
		return func(context.Context) error {
			close(stopInvoked)
			return nil
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- lc.Start(ctx)
	}()

	<-connectStarted

	// Call stop while it's blocked in connect
	err := lc.Stop(context.Background())
	assert.NoError(t, err)

	// It should invoke stop immediately once Start's Connect returns
	select {
	case <-stopInvoked:
		// success
	case <-time.After(1 * time.Second):
		t.Fatal("stop function not invoked for half-started instance")
	}

	errStart := <-errCh
	assert.NoError(t, errStart) // we return nil if it was stopped before connect finished
}

func TestLifecycle_StopReturnsClosureError(t *testing.T) {
	expectedErr := errors.New("close failed")
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return func(context.Context) error {
			return expectedErr
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = lc.Start(ctx)
	}()

	<-lc.Ready()

	err := lc.Stop(context.Background())
	assert.ErrorIs(t, err, expectedErr)
}

func TestLifecycle_StopForwardsContext(t *testing.T) {
	type ctxKey struct{}
	sentinel := "shutdown-marker"

	var receivedCtx context.Context
	lc := NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return func(stopCtx context.Context) error {
			receivedCtx = stopCtx
			return nil
		}, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = lc.Start(ctx)
	}()

	<-lc.Ready()

	shutdownCtx := context.WithValue(context.Background(), ctxKey{}, sentinel)
	err := lc.Stop(shutdownCtx)
	assert.NoError(t, err)
	assert.Equal(t, sentinel, receivedCtx.Value(ctxKey{}), "Stop should forward its context to the stop closure")
}

func TestLifecycle_StartNilConnectReturnsError(t *testing.T) {
	lc := Lifecycle{}
	err := lc.Start(context.Background())
	assert.Error(t, err)
	assert.Equal(t, "core.Lifecycle: Connect closure is not set", err.Error())
}
