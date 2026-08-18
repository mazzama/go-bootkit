package core

import (
	"context"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

// Component is a lifecycle-managed unit of the application: it reports its
// name and can be started and stopped.
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Readyable signals that a component has an observable readiness channel.
// The channel closes when the component is fully connected and ready to serve.
// Lifecycle implements Readyable; custom adapters may implement it directly.
type Readyable interface {
	Ready() <-chan struct{}
}

// HealthCheckProvider exposes the health checks a component contributes to
// the health aggregator.
type HealthCheckProvider interface {
	HealthChecks() []healthkit.Check
}
