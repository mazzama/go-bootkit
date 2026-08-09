package workerpool

import (
	"context"
	"time"
)

type JobStatus string

const (
	StatusUnknown   JobStatus = "Unknown"
	StatusPending   JobStatus = "Pending"
	StatusRunning   JobStatus = "Running"
	StatusCompleted JobStatus = "Completed"
	StatusFailed    JobStatus = "Failed"
)

type Job struct {
	ID        string
	Payload   []byte
	CreatedAt time.Time
}

type Queue interface {
	Enqueue(ctx context.Context, job Job) error
	Dequeue(ctx context.Context) (Job, error)
	Ack(ctx context.Context, jobID string) error
	Nack(ctx context.Context, jobID string) error
	Status(ctx context.Context, jobID string) (JobStatus, error)
}

type Processor interface {
	Process(ctx context.Context, payload []byte) error
}
