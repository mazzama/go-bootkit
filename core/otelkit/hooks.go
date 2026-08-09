package otelkit

import (
	"context"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// OTelHooks implements core.Hooks by recording metrics via OpenTelemetry.
type OTelHooks struct {
	meter             metric.Meter
	startDuration     metric.Int64Histogram
	stopDuration      metric.Int64Histogram
	healthDuration    metric.Int64Histogram
	healthErrorsTotal metric.Int64Counter
}

// NewHooks creates a new OTelHooks instance using the provided meter.
func NewHooks(meter metric.Meter) (*OTelHooks, error) {
	startDuration, err := meter.Int64Histogram("bootkit.component.start.duration",
		metric.WithDescription("Duration of component startup"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	stopDuration, err := meter.Int64Histogram("bootkit.component.stop.duration",
		metric.WithDescription("Duration of component shutdown"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	healthDuration, err := meter.Int64Histogram("bootkit.health.duration",
		metric.WithDescription("Duration of health check evaluation"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	healthErrorsTotal, err := meter.Int64Counter("bootkit.health.errors.total",
		metric.WithDescription("Total number of failed health check evaluations"),
	)
	if err != nil {
		return nil, err
	}

	return &OTelHooks{
		meter:             meter,
		startDuration:     startDuration,
		stopDuration:      stopDuration,
		healthDuration:    healthDuration,
		healthErrorsTotal: healthErrorsTotal,
	}, nil
}

func (h *OTelHooks) OnComponentStart(name string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("component", name),
		attribute.Bool("error", err != nil),
	}
	h.startDuration.Record(context.Background(), duration.Milliseconds(), metric.WithAttributes(attrs...))
}

func (h *OTelHooks) OnComponentStop(name string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("component", name),
		attribute.Bool("error", err != nil),
	}
	h.stopDuration.Record(context.Background(), duration.Milliseconds(), metric.WithAttributes(attrs...))
}

func (h *OTelHooks) OnHealthEvaluated(kind healthkit.Kind, duration time.Duration, err error) {
	kindStr := "unknown"
	switch kind {
	case healthkit.Liveness:
		kindStr = "liveness"
	case healthkit.Readiness:
		kindStr = "readiness"
	case healthkit.Startup:
		kindStr = "startup"
	}

	attrs := []attribute.KeyValue{
		attribute.String("kind", kindStr),
		attribute.Bool("error", err != nil),
	}

	h.healthDuration.Record(context.Background(), duration.Milliseconds(), metric.WithAttributes(attrs...))
	if err != nil {
		h.healthErrorsTotal.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}

var _ core.Hooks = (*OTelHooks)(nil)
