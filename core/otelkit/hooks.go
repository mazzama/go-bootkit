package otelkit

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/mazzama/go-bootkit/core"
)

// OTelHooks implements core.Hooks by recording metrics via OpenTelemetry.
type OTelHooks struct {
	meter             metric.Meter
	startDuration     metric.Int64Histogram
	stopDuration      metric.Int64Histogram
	healthDuration    metric.Int64Histogram
	healthErrorsTotal metric.Int64Counter
}

var _ core.Hooks = (*OTelHooks)(nil)

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

// OnComponentStart records the component start duration as a metric.
func (h *OTelHooks) OnComponentStart(name string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("component", name),
		attribute.Bool("error", err != nil),
	}
	h.startDuration.Record(context.Background(), duration.Milliseconds(), metric.WithAttributes(attrs...))
}

// OnComponentStop records the component stop duration as a metric.
func (h *OTelHooks) OnComponentStop(name string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("component", name),
		attribute.Bool("error", err != nil),
	}
	h.stopDuration.Record(context.Background(), duration.Milliseconds(), metric.WithAttributes(attrs...))
}

// OnHealthEvaluated records the health check evaluation duration and
// increments the error counter on failure.
func (h *OTelHooks) OnHealthEvaluated(kind string, duration time.Duration, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("kind", kind),
		attribute.Bool("error", err != nil),
	}

	h.healthDuration.Record(context.Background(), duration.Milliseconds(), metric.WithAttributes(attrs...))
	if err != nil {
		h.healthErrorsTotal.Add(context.Background(), 1, metric.WithAttributes(attrs...))
	}
}
