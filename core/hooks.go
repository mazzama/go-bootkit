package core

import (
	"time"

	"github.com/mazzama/go-bootkit/core/healthkit"
)

// Hooks provides an observability seam for the framework lifecycle.
type Hooks interface {
	OnComponentStart(name string, duration time.Duration, err error)
	OnComponentStop(name string, duration time.Duration, err error)
	OnHealthEvaluated(kind healthkit.Kind, duration time.Duration, err error)
}

// NoOpHooks is a zero-allocation default implementation of Hooks.
type NoOpHooks struct{}

func (NoOpHooks) OnComponentStart(name string, duration time.Duration, err error)          {}
func (NoOpHooks) OnComponentStop(name string, duration time.Duration, err error)           {}
func (NoOpHooks) OnHealthEvaluated(kind healthkit.Kind, duration time.Duration, err error) {}

var _ Hooks = NoOpHooks{}
