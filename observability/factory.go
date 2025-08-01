package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// FactoryConfig holds the static configuration for the observability system.
type FactoryConfig struct {
	ServiceName string
	ServiceApp  string
	ServiceEnv  string
	ApmType     string
	ApmURL      string
}

// Factory is responsible for creating Observability instances.
type Factory struct {
	config FactoryConfig
}

// NewFactory creates a new observability factory.
func NewFactory(config FactoryConfig) *Factory {
	return &Factory{config: config}
}

// NewBackgroundObservability creates an Observability instance with a background context,
// ideal for logging startup, shutdown, or other non-request-bound events.
func (f *Factory) NewBackgroundObservability(ctx context.Context) *Observability {
	return NewObservability(ctx, f.config.ServiceName, f.config.ApmType)
}

// SetupTracing initializes the global tracer provider based on the factory's configuration.
func (f *Factory) SetupTracing(ctx context.Context) (Shutdowner, error) {
	return SetupTracing(ctx, f.config.ServiceName, f.config.ServiceApp, f.config.ServiceEnv, f.config.ApmURL, f.config.ApmType)
}

// StartSpanFromRequest is the primary entry point for instrumenting an incoming HTTP request.
// It returns a new request with the full context, the final context itself, the created span, and the observability container.
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability) {
	// Extract the trace context from the incoming headers.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	// Create the observability container.
	obs := NewObservability(ctx, f.config.ServiceName, f.config.ApmType)

	// Automatically create default attributes from the request.
	defaultAttrs := SpanAttributes{
		"http.method": r.Method,
		"http.url":    r.URL.String(),
		"http.target": r.URL.RequestURI(),
		"http.host":   r.Host,
		"http.scheme": r.URL.Scheme,
	}

	// Merge any custom attributes provided by the caller.
	if len(customAttrs) > 0 {
		for k, v := range customAttrs[0] {
			defaultAttrs[k] = v
		}
	}

	// Start the span using the new method. This returns a context with the span.
	ctx, span := obs.StartSpan(obs.Context(), r.URL.Path, defaultAttrs)

	// Put the obs object into the new context that contains the span.
	ctx = CtxWithObs(ctx, obs)

	// Update the request with this final, fully-populated context.
	r = r.WithContext(ctx)

	return r, ctx, span, obs
}
