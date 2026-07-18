package core

import (
	"context"
	"log/slog"
)

type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type Readyable interface {
	Ready() <-chan struct{}
}

type Loggable interface {
	SetLogger(logger *slog.Logger)
}
