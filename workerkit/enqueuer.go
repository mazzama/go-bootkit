package workerkit

import (
	"context"
	"time"
)

// Task is a framework-native unit of work, decoupled from the underlying
// job-queue implementation. Adapters map Task to backend-specific types.
type Task struct {
	Type    string
	Payload []byte
}

// TaskInfo carries metadata about an enqueued task.
type TaskInfo struct {
	ID    string
	Type  string
	Queue string
	State string
}

// EnqueueOption configures how a task is enqueued.
type EnqueueOption func(*enqueueOptions)

type enqueueOptions struct {
	queue    string
	maxRetry int
	deadline time.Time
}

// WithQueue sets the target queue name.
func WithQueue(name string) EnqueueOption {
	return func(o *enqueueOptions) { o.queue = name }
}

// WithMaxRetry sets the maximum retry count for the task.
func WithMaxRetry(n int) EnqueueOption {
	return func(o *enqueueOptions) { o.maxRetry = n }
}

// WithDeadline sets an absolute deadline after which the task is discarded.
func WithDeadline(t time.Time) EnqueueOption {
	return func(o *enqueueOptions) { o.deadline = t }
}

// Enqueuer is the interface for enqueueing background tasks.
type Enqueuer interface {
	Enqueue(task Task, opts ...EnqueueOption) (*TaskInfo, error)
	EnqueueContext(ctx context.Context, task Task, opts ...EnqueueOption) (*TaskInfo, error)
}
