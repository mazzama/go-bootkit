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
