package core

import "context"

type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Readyable interface {
	Ready() <-chan struct{}
}
