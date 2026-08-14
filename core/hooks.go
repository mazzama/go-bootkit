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

func (NoOpHooks) OnComponentStart(name string, duration time.Duration, err error)  {}
func (NoOpHooks) OnComponentStop(name string, duration time.Duration, err error)   {}
func (NoOpHooks) OnHealthEvaluated(kind string, duration time.Duration, err error) {}

var _ Hooks = NoOpHooks{}
