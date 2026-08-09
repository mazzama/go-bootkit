package memory

import (
	"context"
	"fmt"
	"sync"

	"github.com/mazzama/go-bootkit/workerpool"
)

type Queue struct {
	mu       sync.RWMutex
	jobs     map[string]workerpool.Job
	statuses map[string]workerpool.JobStatus
	ch       chan workerpool.Job
}

func NewQueue(capacity int) *Queue {
	return &Queue{
		jobs:     make(map[string]workerpool.Job),
		statuses: make(map[string]workerpool.JobStatus),
		ch:       make(chan workerpool.Job, capacity),
	}
}

func (q *Queue) Enqueue(ctx context.Context, job workerpool.Job) error {
	q.mu.Lock()
	if _, exists := q.jobs[job.ID]; exists {
		q.mu.Unlock()
		return fmt.Errorf("job %s already exists", job.ID)
	}
	q.jobs[job.ID] = job
	q.statuses[job.ID] = workerpool.StatusPending
	q.mu.Unlock()

	select {
	case q.ch <- job:
		return nil
	case <-ctx.Done():
		// Revert state if context is cancelled before enqueuing
		q.mu.Lock()
		delete(q.jobs, job.ID)
		delete(q.statuses, job.ID)
		q.mu.Unlock()
		return ctx.Err()
	}
}

func (q *Queue) Dequeue(ctx context.Context) (workerpool.Job, error) {
	select {
	case job := <-q.ch:
		q.mu.Lock()
		q.statuses[job.ID] = workerpool.StatusRunning
		q.mu.Unlock()
		return job, nil
	case <-ctx.Done():
		return workerpool.Job{}, ctx.Err()
	}
}

func (q *Queue) Ack(ctx context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.jobs[jobID]; !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	q.statuses[jobID] = workerpool.StatusCompleted
	return nil
}

func (q *Queue) Nack(ctx context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.jobs[jobID]; !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	q.statuses[jobID] = workerpool.StatusFailed
	return nil
}

func (q *Queue) Status(ctx context.Context, jobID string) (workerpool.JobStatus, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	status, exists := q.statuses[jobID]
	if !exists {
		return workerpool.StatusUnknown, nil
	}

	return status, nil
}
