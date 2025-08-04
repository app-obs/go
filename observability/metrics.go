package observability

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

// Metrics provides an API for creating and managing metric instruments.
type Metrics struct {
	obs   *Observability
	meter metric.Meter
}

// newMetrics creates a new Metrics instance.
func newMetrics(obs *Observability) *Metrics {
	return &Metrics{
		obs:   obs,
		meter: otel.GetMeterProvider().Meter(obs.serviceName),
	}
}

// Counter creates a new float64 counter.
func (m *Metrics) Counter(name string, opts ...metric.Float64CounterOption) (metric.Float64Counter, error) {
	return m.meter.Float64Counter(name, opts...)
}
