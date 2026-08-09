package core

import (
	"context"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type HealthCheckProvider interface {
	HealthChecks() []healthkit.Check
}
