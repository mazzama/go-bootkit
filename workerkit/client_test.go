package workerkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/mazzama/go-bootkit/workerkit"
)

func TestAsynqClient_LifecycleAndEnqueue(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := workerkit.RedisConfig{Addr: mr.Addr()}
	client := workerkit.NewAsynqClient("test-client", redisOpt)

	if client.Name() != "test-client" {
		t.Errorf("expected test-client, got %s", client.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.Start(ctx)
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-client.Ready():
	case <-timer.C:
		t.Fatal("client did not become ready")
	}
	// Test EnqueueContext path
	fwTaskCtx := workerkit.Task{Type: "test:ctx", Payload: nil}
	infoCtx, err := client.EnqueueContext(ctx, fwTaskCtx, workerkit.WithProcessIn(time.Hour))
	if err != nil {
		t.Fatalf("EnqueueContext failed: %v", err)
	}
	if infoCtx.Type != "test:ctx" {
		t.Errorf("unexpected task type: %s", infoCtx.Type)
	}

	// Test Enqueue with all options to cover toAsynqOpts branches
	optsTask := workerkit.Task{Type: "test:opts", Payload: nil}
	_, err = client.Enqueue(optsTask,
		workerkit.WithQueue("high"),
		workerkit.WithMaxRetry(5),
		workerkit.WithDeadline(time.Now().Add(time.Hour)),
		workerkit.WithProcessIn(time.Minute),
		workerkit.WithProcessAt(time.Now().Add(time.Hour)),
		workerkit.WithUnique(10*time.Minute),
		workerkit.WithTimeout(5*time.Minute),
		workerkit.WithRetention(24*time.Hour),
		workerkit.WithGroup("test-group"),
	)
	if err != nil {
		t.Fatalf("Enqueue with options failed: %v", err)
	}

	// Also test the Enqueuer interface path.
	ftask := workerkit.Task{Type: "test:framework", Payload: []byte("fw-payload")}
	fwInfo, fwErr := client.Enqueue(ftask)
	if fwErr != nil {
		t.Fatalf("Enqueue (framework Task) failed: %v", fwErr)
	}
	if fwInfo.Type != "test:framework" {
		t.Errorf("unexpected fw task type: %s", fwInfo.Type)
	}

	err = client.Stop(ctx)
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestAsynqClient_Enqueue_NotReady(t *testing.T) {
	redisOpt := workerkit.RedisConfig{Addr: "localhost:9999"}
	client := workerkit.NewAsynqClient("test-client", redisOpt)

	task := workerkit.Task{Type: "test:task"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.EnqueueContext(ctx, task)
	if err == nil {
		t.Error("expected error for EnqueueContext when not ready")
	}
}
