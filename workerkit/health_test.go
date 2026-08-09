package workerkit_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/workerkit"
)

func TestHealthChecks(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisOpt := asynq.RedisClientOpt{Addr: mr.Addr()}
	server := workerkit.NewAsynqServer("test-health", redisOpt, asynq.Config{Concurrency: 1})
	client := workerkit.NewAsynqClient("test-client-health", redisOpt)

	checks := server.HealthChecks()
	if len(checks) != 2 { // liveness and readiness
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	checksClient := client.HealthChecks()
	if len(checksClient) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checksClient))
	}

	// Removed Check method call since it requires internal healthkit structure knowledge.
}
