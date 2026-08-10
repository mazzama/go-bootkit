# workerkit

The `workerkit` module integrates `hibiken/asynq` (a Redis-backed background job processor) with the Bootkit framework.

## Glossary

- **Enqueuer**: Interface for enqueueing background tasks (`Enqueue`, `EnqueueContext`). Accepts framework `Task` values — callers never import `asynq`. `AsynqClient` implements it for production; `InMemoryClient` in `workerkit/memqueue` implements it for tests.
- **Task**: Framework-native unit of work (`Type string`, `Payload []byte`). Decoupled from `asynq.Task`. Adapters map to backend-specific types internally.
- **TaskInfo**: Metadata about an enqueued task (`ID`, `Type`, `Queue`, `State`).
- **EnqueueOption**: Functional options for task enqueueing (`WithQueue`, `WithMaxRetry`, `WithDeadline`, `WithProcessIn`, `WithProcessAt`, `WithUnique`, `WithTimeout`, `WithRetention`, `WithGroup`). Mapped to asynq options inside `AsynqClient`.
- **AsynqClient**: Production adapter. Implements `Enqueuer` and `core.Component`. Readiness-gates task enqueueing so callers don't enqueue before the Redis connection is live.
- **AsynqServer**: Production adapter for processing tasks. Implements `core.Component`. Exposes `Mux()` for task handler registration.
- **InMemoryClient**: Test adapter in `workerkit/memqueue`. Stores enqueued tasks in a slice. Never returns errors (unless context is cancelled). Use `Tasks()` to inspect and `Reset()` between tests.

## Architecture

By separating Client and Server into two components:
1. **HTTP Nodes** can embed just the `AsynqClient` component. Saves resources — no worker goroutines.
2. **Worker Nodes** can embed just the `AsynqServer` component.
3. **Monoliths** can embed both.

### Enqueuer seam

The `Enqueuer` interface decouples callers from `asynq`. Services accept `Enqueuer` in their constructors and use `Task` values. In production, pass `AsynqClient`. In tests, pass `memqueue.New()`.

This mirrors the `cachekit.Cache` / `memcache.MemoryCache` pattern — same seam shape, same test ergonomics.

## Usage

### 1. Initialize the Components

```go
import (
	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/workerkit"
)

redisOpt := asynq.RedisClientOpt{Addr: "localhost:6379"}

// Client for enqueueing tasks — implements Enqueuer.
client := workerkit.NewAsynqClient("asynq-client", redisOpt)

// Server for processing tasks.
server := workerkit.NewAsynqServer(
	"asynq-server",
	redisOpt,
	asynq.Config{Concurrency: 20},
)

// Register task handlers.
server.Mux().HandleFunc("email:deliver", HandleEmailDelivery)

runner := core.NewApplicationRunner(
	core.WithServices(client, server),
)
```

### 2. Enqueue Tasks (framework-native)

Use `Enqueuer` and `Task` — no asynq import needed:

```go
type MyService struct {
	enqueuer workerkit.Enqueuer
}

func (s *MyService) Send(ctx context.Context) error {
	task := workerkit.Task{
		Type:    "email:deliver",
		Payload: []byte(`{"to":"user@example.com"}`),
	}
	info, err := s.enqueuer.EnqueueContext(ctx, task, workerkit.WithQueue("critical"))
	// ...
}
```

### 3. Test Services with InMemoryClient

```go
import "github.com/mazzama/go-bootkit/workerkit/memqueue"

func TestSend(t *testing.T) {
	client := memqueue.New()
	svc := &MyService{enqueuer: client}

	err := svc.Send(context.Background())
	// No error expected.

	tasks := client.Tasks()
	// Assert task type, payload, etc.
}
```


### 5. Process Tasks

Handler functions match asynq's signature:

```go
func HandleEmailDelivery(ctx context.Context, t *asynq.Task) error {
	// Deserialize payload and do work.
	return nil // returning an error triggers retries.
}
```

## Configuration

`NewAsynqServer` accepts `asynq.Config` directly for dead-letter queues, strict priority queues, and custom retry delays.
