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
	default:
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

func (c *AsynqClient) Name() string {
	return c.name
}

func (c *AsynqClient) HealthChecks() []healthkit.Check {
	return asynqHealthChecks(c.name, c.Ready())
}

func toAsynqOpts(opts []EnqueueOption) []asynq.Option {
	o := &EnqueueOptions{}
	for _, fn := range opts {
		fn(o)
	}
	var aOpts []asynq.Option
	if o.Queue != "" {
		aOpts = append(aOpts, asynq.Queue(o.Queue))
	}
	if o.MaxRetry != nil {
		aOpts = append(aOpts, asynq.MaxRetry(*o.MaxRetry))
	}
	if !o.Deadline.IsZero() {
		aOpts = append(aOpts, asynq.Deadline(o.Deadline))
	}
	if o.ProcessIn > 0 {
		aOpts = append(aOpts, asynq.ProcessIn(o.ProcessIn))
	}
	if !o.ProcessAt.IsZero() {
		aOpts = append(aOpts, asynq.ProcessAt(o.ProcessAt))
	}
	if o.Unique > 0 {
		aOpts = append(aOpts, asynq.Unique(o.Unique))
	}
	if o.Timeout > 0 {
		aOpts = append(aOpts, asynq.Timeout(o.Timeout))
	}
	if o.Retention > 0 {
		aOpts = append(aOpts, asynq.Retention(o.Retention))
	}
	if o.Group != "" {
		aOpts = append(aOpts, asynq.Group(o.Group))
	}
	return aOpts
}

func fromAsynqInfo(info *asynq.TaskInfo) *TaskInfo {
	return &TaskInfo{
		ID:            info.ID,
		Type:          info.Type,
		Queue:         info.Queue,
		State:         info.State.String(),
		NextProcessAt: info.NextProcessAt,
		Retention:     info.Retention,
		CompletedAt:   info.CompletedAt,
		Result:        info.Result,
		LastErr:       info.LastErr,
		LastFailedAt:  info.LastFailedAt,
	}
}

var _ core.Readyable = (*AsynqClient)(nil)

var _ core.Component = (*AsynqClient)(nil)
var _ Enqueuer = (*AsynqClient)(nil)
