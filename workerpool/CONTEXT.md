# workerpool

The `workerpool` module provides an asynchronous background job processing system designed for resilient bulk processing. It solves the problem of long-running or batch tasks (like generating thousands of QR codes, sending emails, or batching external API calls) that would otherwise block HTTP requests and cause timeouts.

## Architecture

The module is built around three core concepts:
1. **WorkerPool:** A `core.Component` that manages a pool of worker goroutines. It integrates cleanly with `core.Lifecycle` to guarantee graceful startup, isolation of panics during job processing, and graceful shutdown (draining in-flight jobs without losing work).
2. **Queue:** The adapter seam for enqueueing and dequeueing jobs. Backpressure is defined by the queue implementation (e.g., blocking `Submit` when full). The module currently ships with an in-memory queue (`workerpool/memory`).
3. **Processor:** An application-defined interface that actually executes the business logic for each job.

## Usage

### 1. Implement a Processor

Your domain logic should implement the `workerpool.Processor` interface.

```go
type QRProcessor struct {
	logger *slog.Logger
}

func (p *QRProcessor) Process(ctx context.Context, payload []byte) error {
	p.logger.Info("Generating QR code", "payload", string(payload))
	// Execute generation logic here...
	return nil
}
```

### 2. Wire the WorkerPool

In your main wiring, initialize a `Queue` adapter, construct the `WorkerPool`, and pass it to your `core.ApplicationRunner`.

```go
import (
	"github.com/mazzama/go-bootkit/workerpool"
	"github.com/mazzama/go-bootkit/workerpool/memory"
)

// 1. Create a queue adapter (e.g., in-memory with capacity of 10,000 jobs)
queue := memory.NewQueue(10000)

// 2. Instantiate your domain processor
processor := &QRProcessor{logger: logger}

// 3. Create the worker pool with 10 concurrent workers
pool := workerpool.NewWorkerPool(
	"qr-worker-pool",
	queue,
	processor,
	workerpool.WithWorkers(10),
	workerpool.WithLogger(logger),
)

// 4. Pass the pool to your application runner alongside DB and server
runner := core.NewApplicationRunner(
	core.WithServices(db, cache, pool, server),
)
```

### 3. Submit Jobs

From your HTTP handlers or domain services, call `pool.Submit` to asynchronously dispatch jobs.

```go
func (h *Handler) CreateBulkQR(w http.ResponseWriter, r *http.Request) {
    // Read request...
    
    // Submit job (blocks if the queue is at capacity, providing natural backpressure)
    jobID, err := h.pool.Submit(r.Context(), payloadBytes)
    if err != nil {
        http.Error(w, "Service Unavailable: Queue full", http.StatusServiceUnavailable)
        return
    }
    
    // Return the Job ID immediately
    fmt.Fprintf(w, "Job %s submitted successfully", jobID)
}
```

## Observability and Resilience

The `WorkerPool` automatically:
- **Catches Panics:** If a job processor panics, the pool recovers, marks the job as `Failed`, logs the incident, and moves on to the next job without crashing the worker goroutine.
- **Reports Health:** Automatically provides `Liveness` and `Readiness` checks via `healthkit.StandardChecks`.
- **Integrates with OTel:** When using `core.Hooks`, start and stop durations are seamlessly tracked as metrics.
