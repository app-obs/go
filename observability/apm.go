package observability

// APMType defines the type of Application Performance Monitoring.
type APMType string

const (
	// OTLP represents the OpenTelemetry Protocol.
	OTLP APMType = "OTLP"
	// DataDog represents the DataDog APM.
	DataDog APMType = "DataDog"
	// Add other APM types here in the future.
)
