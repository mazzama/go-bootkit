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
		// Nothing to start for the client, just return a closure to close it
		return func(shutdownCtx context.Context) error {
			return c.client.Close()
		}, nil
	})

	return c
}

func (c *AsynqClient) Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	select {
	case <-c.Ready():
	case <-context.Background().Done():
		return nil, fmt.Errorf("client not ready")
	}
	return c.client.Enqueue(task, opts...)
}

func (c *AsynqClient) EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error) {
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

var _ core.Component = (*AsynqClient)(nil)
