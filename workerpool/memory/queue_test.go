package memory

import (
	"context"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/workerpool"
	"github.com/stretchr/testify/assert"
)

func TestQueue_EnqueueAndDequeue(t *testing.T) {
	q := NewQueue(5)
	job := workerpool.Job{ID: "job-1", Payload: []byte("test")}

	err := q.Enqueue(context.Background(), job)
	assert.NoError(t, err)

	status, err := q.Status(context.Background(), "job-1")
	assert.NoError(t, err)
	assert.Equal(t, workerpool.StatusPending, status)

	deqJob, err := q.Dequeue(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, "job-1", deqJob.ID)

	status, err = q.Status(context.Background(), "job-1")
	assert.NoError(t, err)
	assert.Equal(t, workerpool.StatusRunning, status)
}

func TestQueue_AckAndNack(t *testing.T) {
	q := NewQueue(5)

	job1 := workerpool.Job{ID: "job-1"}
	job2 := workerpool.Job{ID: "job-2"}

	_ = q.Enqueue(context.Background(), job1)
	_ = q.Enqueue(context.Background(), job2)

	// Dequeue both
	_, _ = q.Dequeue(context.Background())
	_, _ = q.Dequeue(context.Background())

	err := q.Ack(context.Background(), "job-1")
	assert.NoError(t, err)

	err = q.Nack(context.Background(), "job-2")
	assert.NoError(t, err)

	status1, _ := q.Status(context.Background(), "job-1")
	assert.Equal(t, workerpool.StatusCompleted, status1)

	status2, _ := q.Status(context.Background(), "job-2")
	assert.Equal(t, workerpool.StatusFailed, status2)
}

func TestQueue_EnqueueBackpressure(t *testing.T) {
	q := NewQueue(1)

	err := q.Enqueue(context.Background(), workerpool.Job{ID: "job-1"})
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = q.Enqueue(ctx, workerpool.Job{ID: "job-2"})
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Ensure job-2 wasn't tracked
	status, _ := q.Status(context.Background(), "job-2")
	assert.Equal(t, workerpool.StatusUnknown, status)
}
