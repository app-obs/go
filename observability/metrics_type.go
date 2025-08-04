package observability

import "strings"

// MetricsType defines the type of Metrics implementation.
type MetricsType string

const (
	// OTLPMetrics represents the OpenTelemetry Protocol for metrics.
	OTLPMetrics MetricsType = "otlp"
	// NoneMetrics disables metrics.
	NoneMetrics MetricsType = "none"
)

// normalizeMetricsType converts a string to a canonical MetricsType, ignoring case.
func normalizeMetricsType(metricsType string) MetricsType {
	switch strings.ToLower(metricsType) {
	case "otlp":
		return OTLPMetrics
	case "none":
		return NoneMetrics
	default:
		return NoneMetrics // Default to no metrics if the type is unknown
	}
}
