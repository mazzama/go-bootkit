# workerkit

The `workerkit` module integrates `hibiken/asynq` (a Redis-backed background job processor) with the Bootkit framework. It splits Asynq into two `core.Component` implementations: `AsynqClient` for enqueueing jobs, and `AsynqServer` for processing jobs.

## Architecture

By separating Client and Server into two components:
1. **HTTP Nodes** can embed just the `AsynqClient` component. This saves resources as they don't spin up worker goroutines.
2. **Worker Nodes** can embed just the `AsynqServer` component.
3. **Monoliths** can embed both.

Both components integrate with `core.Lifecycle` to honor graceful shutdown and context timeouts, and with `healthkit` to report liveness/readiness.

## Usage

### 1. Initialize the Components

```go
import (
	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/workerkit"
)

// Configure Redis connection
redisOpt := asynq.RedisClientOpt{Addr: "localhost:6379"}

// Create Client for enqueueing tasks
client := workerkit.NewAsynqClient("asynq-client", redisOpt)

// Create Server for processing tasks
server := workerkit.NewAsynqServer(
	"asynq-server",
	redisOpt,
	workerkit.WithConcurrency(20),
)

// Define your task handlers
server.Mux().HandleFunc("email:deliver", HandleEmailDelivery)

// Pass to the application runner
runner := core.NewApplicationRunner(
	core.WithServices(client, server),
)
```

### 2. Enqueue Tasks

Use the client component (which behaves like `asynq.Client`) to enqueue tasks asynchronously:

```go
task := asynq.NewTask("email:deliver", []byte(`{"to": "user@example.com"}`))

// EnqueueContext respects the client's Lifecycle Readiness state
info, err := client.EnqueueContext(ctx, task)
```

### 3. Process Tasks

The handler function simply needs to match `asynq`'s signature:

```go
func HandleEmailDelivery(ctx context.Context, t *asynq.Task) error {
    // Deserialize payload and do work
    return nil // returning an error automatically triggers retries
}
```

## Options

`NewAsynqServer` accepts `WithConcurrency(n)` for simple tuning, but also `WithAsynqConfig(cfg)` which allows you to pass a completely custom `asynq.Config` to configure dead-letter queues, strict priority queues, and custom retry delays.
