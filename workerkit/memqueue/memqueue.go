// Package memqueue provides an in-memory implementation of workerkit.Enqueuer
// for tests. Tasks are stored in a slice and can be inspected after the fact.
// Options passed to Enqueue/EnqueueContext are ignored — the purpose of this
// adapter is to capture what was enqueued, not to validate option routing.
package memqueue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mazzama/go-bootkit/workerkit"
)

var _ workerkit.Enqueuer = (*InMemoryClient)(nil)

// EnqueuedTask is a task as it was enqueued, with metadata captured at enqueue time.
type EnqueuedTask struct {
	Task       workerkit.Task
	EnqueuedAt time.Time
}

// InMemoryClient is an in-memory Enqueuer for use in tests. It stores
// every enqueued task in order and never returns an error (unless the
// context is already cancelled).
type InMemoryClient struct {
	mu    sync.Mutex
	tasks []EnqueuedTask
}

// New creates a ready-to-use InMemoryClient.
func New() *InMemoryClient {
	return &InMemoryClient{}
}

// Tasks returns a copy of all enqueued tasks in insertion order.
func (c *InMemoryClient) Tasks() []EnqueuedTask {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]EnqueuedTask, len(c.tasks))
	copy(out, c.tasks)
	return out
}

// Reset clears all stored tasks. Useful in test teardown.
func (c *InMemoryClient) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tasks = nil
}

// Enqueue stores the task and returns a synthetic TaskInfo.
func (c *InMemoryClient) Enqueue(task workerkit.Task, _ ...workerkit.EnqueueOption) (*workerkit.TaskInfo, error) {
	return c.enqueue(task), nil
}

// EnqueueContext stores the task and returns a synthetic TaskInfo.
// If ctx is already cancelled, it returns ctx.Err() — mirroring the
// real AsynqClient behavior.
func (c *InMemoryClient) EnqueueContext(ctx context.Context, task workerkit.Task, _ ...workerkit.EnqueueOption) (*workerkit.TaskInfo, error) {
	if ctx.Err() != nil {
		return nil, fmt.Errorf("enqueue context cancelled: %w", ctx.Err())
	}
	return c.enqueue(task), nil
}

func (c *InMemoryClient) enqueue(task workerkit.Task) *workerkit.TaskInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tasks = append(c.tasks, EnqueuedTask{
		Task:       task,
		EnqueuedAt: time.Now(),
	})

	return &workerkit.TaskInfo{
		ID:    fmt.Sprintf("mem-%d", len(c.tasks)),
		Type:  task.Type,
		State: "enqueued",
	}
}
