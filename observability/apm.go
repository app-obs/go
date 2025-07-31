package observability

import "strings"

// APMType defines the type of Application Performance Monitoring.
type APMType string

const (
	// OTLP represents the OpenTelemetry Protocol.
	OTLP APMType = "otlp"
	// Datadog represents the Datadog APM.
	Datadog APMType = "datadog"
	// None disables APM.
	None APMType = "none"
)

// normalizeAPMType converts a string to a canonical APMType, ignoring case.
func normalizeAPMType(apmType string) APMType {
	switch strings.ToLower(apmType) {
	case "otlp":
		return OTLP
	case "datadog":
		return Datadog
	case "none":
		return None
	default:
		return None // Default to no APM if the type is unknown
	}
}
