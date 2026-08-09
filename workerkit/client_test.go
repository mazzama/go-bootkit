package workerkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/workerkit"
)

func TestAsynqClient_LifecycleAndEnqueue(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := asynq.RedisClientOpt{Addr: mr.Addr()}
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

	select {
	case <-client.Ready():
	case <-time.After(time.Second):
		t.Fatal("client did not become ready")
	}

	task := asynq.NewTask("test:task", []byte("payload"))
	info, err := client.EnqueueWithAsynq(task)
	if err != nil {
		t.Fatalf("EnqueueWithAsynq failed: %v", err)
	}
	if info.Type != "test:task" {
		t.Errorf("unexpected task type: %s", info.Type)
	}

	infoCtx, err := client.EnqueueContextWithAsynq(ctx, asynq.NewTask("test:ctx", nil))
	if err != nil {
		t.Fatalf("EnqueueContextWithAsynq failed: %v", err)
	}
	if infoCtx.Type != "test:ctx" {
		t.Errorf("unexpected task type: %s", infoCtx.Type)
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
	redisOpt := asynq.RedisClientOpt{Addr: "localhost:9999"}
	client := workerkit.NewAsynqClient("test-client", redisOpt)

	task := workerkit.Task{Type: "test:task"}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.EnqueueContext(ctx, task)
	if err == nil {
		t.Error("expected error for EnqueueContext when not ready")
	}
}
