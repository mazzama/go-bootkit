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
	ID            string
	Type          string
	Queue         string
	State         string
	NextProcessAt time.Time
	Retention     time.Duration
	CompletedAt   time.Time
	Result        []byte
	LastErr       string
	LastFailedAt  time.Time
}

// EnqueueOptions holds the evaluated configuration for a task.
type EnqueueOptions struct {
	Queue     string
	MaxRetry  *int
	Deadline  time.Time
	ProcessIn time.Duration
	ProcessAt time.Time
	Unique    time.Duration
	Timeout   time.Duration
	Retention time.Duration
	Group     string
}

// EnqueueOption configures how a task is enqueued.
type EnqueueOption func(*EnqueueOptions)

// WithQueue sets the target queue name.
func WithQueue(name string) EnqueueOption {
	return func(o *EnqueueOptions) { o.Queue = name }
}

// WithMaxRetry sets the maximum retry count for the task.
func WithMaxRetry(n int) EnqueueOption {
	return func(o *EnqueueOptions) { o.MaxRetry = &n }
}

// WithDeadline sets an absolute deadline after which the task is discarded.
func WithDeadline(t time.Time) EnqueueOption {
	return func(o *EnqueueOptions) { o.Deadline = t }
}

// WithProcessIn schedules the task to be processed after the specified duration.
// If WithProcessAt was previously set, it is cleared.
func WithProcessIn(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.ProcessIn = d
		o.ProcessAt = time.Time{}
	}
}

// WithProcessAt schedules the task to be processed at the specified time.
// If WithProcessIn was previously set, it is cleared.
func WithProcessAt(t time.Time) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.ProcessAt = t
		o.ProcessIn = 0
	}
}

// WithUnique ensures only one task with the same Type, Payload, and Queue exists within the given TTL.
func WithUnique(ttl time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) { o.Unique = ttl }
}

// WithTimeout sets the maximum duration for a single execution of the task.
func WithTimeout(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) { o.Timeout = d }
}

// WithRetention sets how long the task is kept in the queue after successful completion.
func WithRetention(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) { o.Retention = d }
}

// WithGroup groups tasks together for aggregation.
func WithGroup(name string) EnqueueOption {
	return func(o *EnqueueOptions) { o.Group = name }
}

// Enqueuer is the interface for enqueueing background tasks.
type Enqueuer interface {
	Enqueue(task Task, opts ...EnqueueOption) (*TaskInfo, error)
	EnqueueContext(ctx context.Context, task Task, opts ...EnqueueOption) (*TaskInfo, error)
}
