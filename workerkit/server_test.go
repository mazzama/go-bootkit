package workerkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/mazzama/go-bootkit/workerkit"
)

func TestAsynqServer_Lifecycle(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := workerkit.RedisConfig{Addr: mr.Addr()}
	server := workerkit.NewAsynqServer("test-server", redisOpt, workerkit.ServerConfig{Concurrency: 1})

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

	if stopErr := server.Stop(ctx); stopErr != nil {
		t.Errorf("Stop failed: %v", stopErr)
	}
}

func TestAsynqServer_HandleFunc_ProcessesTask(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := workerkit.RedisConfig{Addr: mr.Addr()}
	server := workerkit.NewAsynqServer("test-server", redisOpt, workerkit.ServerConfig{Concurrency: 1})

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
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-server.Ready():
	case <-timer.C:
		t.Fatal("server did not become ready")
	}

	client := workerkit.NewAsynqClient("test-client", redisOpt)
	clientCh := make(chan error, 1)
	go func() {
		clientCh <- client.Start(ctx)
	}()
	timer = time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-client.Ready():
	case <-timer.C:
		t.Fatal("client did not become ready")
	}

	task := workerkit.Task{Type: "notification:send", Payload: []byte(`{"to":"user@example.com"}`)}
	if _, enqErr := client.Enqueue(task); enqErr != nil {
		t.Fatalf("Enqueue failed: %v", enqErr)
	}

	handlerTimer := time.NewTimer(5 * time.Second)
	defer handlerTimer.Stop()
	select {
	case gotTask := <-got:
		if gotTask.Type != "notification:send" {
			t.Errorf("expected task type notification:send, got %q", gotTask.Type)
		}
		if string(gotTask.Payload) != `{"to":"user@example.com"}` {
			t.Errorf("expected payload to match, got %q", string(gotTask.Payload))
		}
	case <-handlerTimer.C:
		t.Fatal("handler did not receive task within timeout")
	}

	if stopErr := client.Stop(ctx); stopErr != nil {
		t.Errorf("client Stop failed: %v", stopErr)
	}
	if stopErr := server.Stop(ctx); stopErr != nil {
		t.Errorf("server Stop failed: %v", stopErr)
	}

	// Start returns ctx.Err() once the context is cancelled; drain both
	// goroutines so they don't outlive the test.
	cancel()
	waitErr := func(ch <-chan error, name string) {
		waitTimer := time.NewTimer(time.Second)
		defer waitTimer.Stop()
		select {
		case <-ch:
		case <-waitTimer.C:
			t.Errorf("%s did not return after cancel", name)
		}
	}
	waitErr(errCh, "server.Start")
	waitErr(clientCh, "client.Start")
}
