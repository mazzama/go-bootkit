package workerkit

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type AsynqClient struct {
	core.Lifecycle
	name   string
	client *asynq.Client
}

func NewAsynqClient(name string, redisOpt asynq.RedisConnOpt) *AsynqClient {
	client := asynq.NewClient(redisOpt)

	c := &AsynqClient{
		name:   name,
		client: client,
	}

	c.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		return func(shutdownCtx context.Context) error {
			return c.client.Close()
		}, nil
	})

	return c
}

// Enqueue schedules a framework Task for asynchronous execution.
func (c *AsynqClient) Enqueue(task Task, opts ...EnqueueOption) (*TaskInfo, error) {
	select {
	case <-c.Ready():
	case <-context.Background().Done():
		return nil, fmt.Errorf("client not ready")
	}
	aTask := asynq.NewTask(task.Type, task.Payload)
	info, err := c.client.Enqueue(aTask, toAsynqOpts(opts)...)
	if err != nil {
		return nil, err
	}
	return fromAsynqInfo(info), nil
}

// EnqueueContext schedules a framework Task with context propagation.
func (c *AsynqClient) EnqueueContext(ctx context.Context, task Task, opts ...EnqueueOption) (*TaskInfo, error) {
	select {
	case <-c.Ready():
	case <-ctx.Done():
		return nil, fmt.Errorf("client not ready: %w", ctx.Err())
	}
	aTask := asynq.NewTask(task.Type, task.Payload)
	info, err := c.client.EnqueueContext(ctx, aTask, toAsynqOpts(opts)...)
	if err != nil {
		return nil, err
	}
	return fromAsynqInfo(info), nil
}

// EnqueueWithAsynq enqueues a raw *asynq.Task with asynq.Option values.
// This is the low-level escape hatch; prefer Enqueue through the Enqueuer interface.
func (c *AsynqClient) EnqueueWithAsynq(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	select {
	case <-c.Ready():
	case <-context.Background().Done():
		return nil, fmt.Errorf("client not ready")
	}
	return c.client.Enqueue(task, opts...)
}

// EnqueueContextWithAsynq enqueues a raw *asynq.Task with context propagation.
// This is the low-level escape hatch; prefer EnqueueContext through the Enqueuer interface.
func (c *AsynqClient) EnqueueContextWithAsynq(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	select {
	case <-c.Ready():
	case <-ctx.Done():
		return nil, fmt.Errorf("client not ready: %w", ctx.Err())
	}
	return c.client.EnqueueContext(ctx, task, opts...)
}

func (c *AsynqClient) Name() string {
	return c.name
}

func (c *AsynqClient) HealthChecks() []healthkit.Check {
	return asynqHealthChecks(c.name, c.Ready())
}

func toAsynqOpts(opts []EnqueueOption) []asynq.Option {
	o := &enqueueOptions{}
	for _, fn := range opts {
		fn(o)
	}
	var aOpts []asynq.Option
	if o.queue != "" {
		aOpts = append(aOpts, asynq.Queue(o.queue))
	}
	if o.maxRetry > 0 {
		aOpts = append(aOpts, asynq.MaxRetry(o.maxRetry))
	}
	if !o.deadline.IsZero() {
		aOpts = append(aOpts, asynq.Deadline(o.deadline))
	}
	return aOpts
}

func fromAsynqInfo(info *asynq.TaskInfo) *TaskInfo {
	return &TaskInfo{
		ID:    info.ID,
		Type:  info.Type,
		Queue: info.Queue,
		State: fmt.Sprintf("%d", info.State),
	}
}

var _ core.Component = (*AsynqClient)(nil)
var _ Enqueuer = (*AsynqClient)(nil)
