package workerkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/workerkit"
)

func TestAsynqServer_Lifecycle(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := asynq.RedisClientOpt{Addr: mr.Addr()}
	server := workerkit.NewAsynqServer("test-server", redisOpt, asynq.Config{Concurrency: 1})

	if server.Name() != "test-server" {
		t.Errorf("expected test-server, got %s", server.Name())
	}

	if server.Mux() == nil {
		t.Error("expected non-nil mux")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Verify ready
	select {
	case <-server.Ready():
	case <-time.After(time.Second):
		t.Fatal("server did not become ready")
	}

	err = server.Stop(ctx)
	if err != nil {
		t.Errorf("Stop failed: %v", err)
	}
}

func TestAsynqServer_HandleFunc_ProcessesTask(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := asynq.RedisClientOpt{Addr: mr.Addr()}
	server := workerkit.NewAsynqServer("test-server", redisOpt, asynq.Config{Concurrency: 1})

	got := make(chan workerkit.Task, 1)
	server.HandleFunc("notification:send", func(ctx context.Context, task workerkit.Task) error {
		got <- task
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()
	select {
	case <-server.Ready():
	case <-time.After(time.Second):
		t.Fatal("server did not become ready")
	}

	client := workerkit.NewAsynqClient("test-client", redisOpt)
	clientCh := make(chan error, 1)
	go func() {
		clientCh <- client.Start(ctx)
	}()
	select {
	case <-client.Ready():
	case <-time.After(time.Second):
		t.Fatal("client did not become ready")
	}

	task := workerkit.Task{Type: "notification:send", Payload: []byte(`{"to":"user@example.com"}`)}
	if _, err := client.Enqueue(task); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	select {
	case gotTask := <-got:
		if gotTask.Type != "notification:send" {
			t.Errorf("expected task type notification:send, got %q", gotTask.Type)
		}
		if string(gotTask.Payload) != `{"to":"user@example.com"}` {
			t.Errorf("expected payload to match, got %q", string(gotTask.Payload))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not receive task within timeout")
	}

	if err := client.Stop(ctx); err != nil {
		t.Errorf("client Stop failed: %v", err)
	}
	if err := server.Stop(ctx); err != nil {
		t.Errorf("server Stop failed: %v", err)
	}
}
