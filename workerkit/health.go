package workerkit

import (
	"context"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

func asynqHealthChecks(name string, ready <-chan struct{}) []healthkit.Check {
	return healthkit.StandardChecks(name, func(ctx context.Context) error {
		return core.WaitReady(ctx, ready)
	})
}
