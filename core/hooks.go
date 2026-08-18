package core

import (
	"time"
)

// Hooks provides an observability seam for the framework lifecycle.
// The kind parameter in OnHealthEvaluated is the string label of the health
// check kind (see healthkit.Kind.String).
type Hooks interface {
	OnComponentStart(name string, duration time.Duration, err error)
	OnComponentStop(name string, duration time.Duration, err error)
	OnHealthEvaluated(kind string, duration time.Duration, err error)
}

// NoOpHooks is a zero-allocation default implementation of Hooks.
type NoOpHooks struct{}

var _ Hooks = NoOpHooks{}

// OnComponentStart is a no-op implementation of Hooks.OnComponentStart.
func (NoOpHooks) OnComponentStart(name string, duration time.Duration, err error) {}

// OnComponentStop is a no-op implementation of Hooks.OnComponentStop.
func (NoOpHooks) OnComponentStop(name string, duration time.Duration, err error) {}

// OnHealthEvaluated is a no-op implementation of Hooks.OnHealthEvaluated.
func (NoOpHooks) OnHealthEvaluated(kind string, duration time.Duration, err error) {}
