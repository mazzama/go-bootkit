package workerpool

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type WorkerPool struct {
	core.Lifecycle
	name        string
	queue       Queue
	processor   Processor
	workerCount int
	logger      *slog.Logger
	cancelFn    context.CancelFunc
	wg          sync.WaitGroup
}

type Option func(*WorkerPool)

func WithWorkers(n int) Option {
	return func(w *WorkerPool) {
		if n > 0 {
			w.workerCount = n
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(w *WorkerPool) {
		if logger != nil {
			w.logger = logger
		}
	}
}

func NewWorkerPool(name string, q Queue, p Processor, opts ...Option) *WorkerPool {
	wp := &WorkerPool{
		name:        name,
		queue:       q,
		processor:   p,
		workerCount: 1,
		logger:      slog.Default(), // Provide a reasonable default
	}

	for _, opt := range opts {
		opt(wp)
	}

	wp.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		workerCtx, cancel := context.WithCancel(context.Background())
		wp.cancelFn = cancel

		wp.wg.Add(wp.workerCount)
		for i := 0; i < wp.workerCount; i++ {
			go wp.workerLoop(workerCtx, i)
		}

		return func(shutdownCtx context.Context) error {
			wp.cancelFn()

			// Wait for workers to finish or shutdownCtx to expire
			done := make(chan struct{})
			go func() {
				wp.wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				return nil
			case <-shutdownCtx.Done():
				return fmt.Errorf("workerpool shutdown timed out: %w", shutdownCtx.Err())
			}
		}, nil
	})

	return wp
}

func (w *WorkerPool) workerLoop(ctx context.Context, id int) {
	defer w.wg.Done()

	for {
		job, err := w.queue.Dequeue(ctx)
		if err != nil {
			// context cancelled, time to stop
			return
		}

		w.processJob(ctx, id, job)
	}
}

func (w *WorkerPool) processJob(ctx context.Context, workerID int, job Job) {
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("panic in job processor", "job_id", job.ID, "worker_id", workerID, "panic", r)
			_ = w.queue.Nack(context.Background(), job.ID)
		}
	}()

	err := w.processor.Process(ctx, job.Payload)
	if err != nil {
		w.logger.Error("job failed", "job_id", job.ID, "worker_id", workerID, "error", err)
		_ = w.queue.Nack(context.Background(), job.ID)
		return
	}

	w.logger.Info("job completed", "job_id", job.ID, "worker_id", workerID)
	_ = w.queue.Ack(context.Background(), job.ID)
}

func (w *WorkerPool) Submit(ctx context.Context, payload []byte) (string, error) {
	id := uuid.New().String()
	job := Job{
		ID:        id,
		Payload:   payload,
		CreatedAt: time.Now(),
	}

	if err := w.queue.Enqueue(ctx, job); err != nil {
		return "", err
	}

	return id, nil
}

func (w *WorkerPool) Status(ctx context.Context, jobID string) (JobStatus, error) {
	return w.queue.Status(ctx, jobID)
}

func (w *WorkerPool) Name() string {
	return w.name
}

func (w *WorkerPool) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(w.name, func(ctx context.Context) error {
		select {
		case <-w.Ready():
			if w.cancelFn == nil {
				return fmt.Errorf("workerpool not started")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

var _ core.Component = (*WorkerPool)(nil)
