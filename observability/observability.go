package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Shutdowner defines a contract for components that can be gracefully shut down.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Observability holds the tracing and logging components.
type Observability struct {
	Trace *Trace
	Log   *Log
	ctx   context.Context
}

// NewObservabilityFromRequest creates a new Observability instance by extracting the
// trace context from an incoming HTTP request.
func NewObservabilityFromRequest(r *http.Request, serviceName string, apmType string) *Observability {
	var ctx context.Context
	typedAPMType := APMType(apmType)
	// For OTLP, we need to manually extract the context from the headers.
	// For DataDog, the tracer does this automatically when starting a span.
	if typedAPMType == OTLP {
		ctx = otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	} else {
		ctx = r.Context()
	}
	return NewObservability(ctx, serviceName, apmType)
}

// NewObservability creates a new Observability instance.
func NewObservability(ctx context.Context, serviceName string, apmType string) *Observability {
	typedAPMType := APMType(apmType)
	obs := &Observability{
		ctx: ctx,
	}
	baseLogger := InitLogger(typedAPMType)
	obs.Trace = NewTrace(obs, serviceName, typedAPMType) // Pass obs and apmType to Trace
	obs.Log = NewLog(obs, baseLogger)                   // Pass obs to Log
	return obs
}

// Context returns the current context from the Observability instance.
func (o *Observability) Context() context.Context {
	return o.ctx
}

// SetContext updates the context in the Observability instance.
func (o *Observability) SetContext(ctx context.Context) {
	o.ctx = ctx
}