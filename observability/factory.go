package observability

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// factoryConfig holds the static configuration for the observability system.
// It is kept private to encourage the use of functional options.
type factoryConfig struct {
	ServiceName string
	ServiceApp  string
	ServiceEnv  string
	ApmType     string
	ApmURL      string
	LogSource   bool
}

// Option is a function that configures a `factoryConfig`.
type Option func(*factoryConfig)

// WithServiceName sets the service name for tracing and metrics.
func WithServiceName(name string) Option {
	return func(c *factoryConfig) {
		c.ServiceName = name
	}
}

// WithServiceApp sets the application or logical group name.
func WithServiceApp(app string) Option {
	return func(c *factoryConfig) {
		c.ServiceApp = app
	}
}

// WithServiceEnv sets the deployment environment (e.g., "development", "production").
func WithServiceEnv(env string) Option {
	return func(c *factoryConfig) {
		c.ServiceEnv = env
	}
}

// WithApmType sets the desired APM backend.
// Valid options are "otlp", "datadog", or "none".
func WithApmType(apmType string) Option {
	return func(c *factoryConfig) {
		c.ApmType = apmType
	}
}

// WithApmURL sets the endpoint URL for the APM collector (e.g., "http://tempo:4318/v1/traces").
func WithApmURL(url string) Option {
	return func(c *factoryConfig) {
		c.ApmURL = url
	}
}

// WithLogSource enables or disables the automatic addition of source file and line number to logs.
func WithLogSource(enabled bool) Option {
	return func(c *factoryConfig) {
		c.LogSource = enabled
	}
}

// Factory is responsible for creating Observability instances.
type Factory struct {
	config factoryConfig
}

// NewFactory creates a new observability factory using functional options.
// It initializes with sensible defaults that can be overridden by the provided options.
func NewFactory(opts ...Option) *Factory {
	// Establish default configuration
	config := factoryConfig{
		ServiceName: "unknown-service",
		ServiceApp:  "unknown-app",
		ServiceEnv:  "development",
		ApmType:     "none",
		ApmURL:      "", // No default URL
		LogSource:   true, // Default to enabled for development convenience.
	}

	// Apply user-provided options
	for _, opt := range opts {
		opt(&config)
	}

	// As a convenience, also read from standard environment variables if they exist,
	// but only if the user hasn't already set the value via an option.
	if config.ServiceName == "unknown-service" {
		if val := os.Getenv("OBS_SERVICE_NAME"); val != "" {
			config.ServiceName = val
		}
	}
	if config.ServiceApp == "unknown-app" {
		if val := os.Getenv("OBS_APPLICATION"); val != "" {
			config.ServiceApp = val
		}
	}
	if config.ServiceEnv == "development" {
		if val := os.Getenv("OBS_ENVIRONMENT"); val != "" {
			config.ServiceEnv = val
		}
	}
	if config.ApmType == "none" {
		if val := os.Getenv("OBS_APM_TYPE"); val != "" {
			config.ApmType = val
		}
	}
	if config.ApmURL == "" {
		if val := os.Getenv("OBS_APM_URL"); val != "" {
			config.ApmURL = val
		}
	}
	// Allow overriding LogSource via environment variable.
	if val := os.Getenv("OBS_LOG_SOURCE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			config.LogSource = b
		}
	}

	return &Factory{config: config}
}

// NewBackgroundObservability creates an Observability instance with a background context,
// ideal for logging startup, shutdown, or other non-request-bound events.
func (f *Factory) NewBackgroundObservability(ctx context.Context) *Observability {
	return NewObservability(ctx, f.config.ServiceName, f.config.ApmType, f.config.LogSource)
}

// SetupTracing initializes the global tracer provider based on the factory's configuration.
func (f *Factory) SetupTracing(ctx context.Context) (Shutdowner, error) {
	return setupTracing(ctx, f.config.ServiceName, f.config.ServiceApp, f.config.ServiceEnv, f.config.ApmURL, f.config.ApmType)
}

// StartSpanFromRequest is the primary entry point for instrumenting an incoming HTTP request.
// It returns a new request with the full context, the final context itself, the created span, and the observability container.
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability) {
	// Extract the trace context from the incoming headers.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	// Create the observability container.
	obs := NewObservability(ctx, f.config.ServiceName, f.config.ApmType, f.config.LogSource)

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
	ctx = ctxWithObs(ctx, obs)

	// Update the request with this final, fully-populated context.
	r = r.WithContext(ctx)

	return r, ctx, span, obs
}