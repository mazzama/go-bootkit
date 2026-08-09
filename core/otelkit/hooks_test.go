package otelkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestOTelHooks_OnComponentStart(t *testing.T) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test-meter")

	hooks, err := NewHooks(meter)
	require.NoError(t, err)

	hooks.OnComponentStart("test-svc", 150*time.Millisecond, nil)

	var rm metricdata.ResourceMetrics
	err = reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	require.Len(t, rm.ScopeMetrics, 1)
	metrics := rm.ScopeMetrics[0].Metrics
	
	// Find bootkit.component.start.duration
	var startMetric *metricdata.Metrics
	for _, m := range metrics {
		if m.Name == "bootkit.component.start.duration" {
			mCopy := m
			startMetric = &mCopy
			break
		}
	}
	require.NotNil(t, startMetric)
	
	histogram := startMetric.Data.(metricdata.Histogram[int64])
	require.Len(t, histogram.DataPoints, 1)
	dp := histogram.DataPoints[0]
	
	assert.Equal(t, uint64(1), dp.Count)
	assert.Equal(t, int64(150), dp.Sum)
}

func TestOTelHooks_OnHealthEvaluated(t *testing.T) {
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	meter := meterProvider.Meter("test-meter")

	hooks, err := NewHooks(meter)
	require.NoError(t, err)

	hooks.OnHealthEvaluated(healthkit.Readiness, 42*time.Millisecond, errors.New("boom"))

	var rm metricdata.ResourceMetrics
	err = reader.Collect(context.Background(), &rm)
	require.NoError(t, err)

	require.Len(t, rm.ScopeMetrics, 1)
	metrics := rm.ScopeMetrics[0].Metrics

	var durationMetric, errorMetric *metricdata.Metrics
	for _, m := range metrics {
		if m.Name == "bootkit.health.duration" {
			mCopy := m
			durationMetric = &mCopy
		}
		if m.Name == "bootkit.health.errors.total" {
			mCopy := m
			errorMetric = &mCopy
		}
	}
	
	require.NotNil(t, durationMetric)
	require.NotNil(t, errorMetric)

	// Validate duration histogram
	hist := durationMetric.Data.(metricdata.Histogram[int64])
	require.Len(t, hist.DataPoints, 1)
	assert.Equal(t, uint64(1), hist.DataPoints[0].Count)
	assert.Equal(t, int64(42), hist.DataPoints[0].Sum)

	// Validate error counter
	counter := errorMetric.Data.(metricdata.Sum[int64])
	require.Len(t, counter.DataPoints, 1)
	assert.Equal(t, int64(1), counter.DataPoints[0].Value)
}
