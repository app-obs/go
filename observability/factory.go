package observability

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// factoryConfig holds the static configuration for the observability system.
// It is kept private to encourage the use of functional options.
type factoryConfig struct {
	ServiceName string
	ServiceApp  string
	ServiceEnv  string
	ApmType      string
	ApmURL       string
	LogLevel     slog.Level
	SpanLogLevel slog.Level
	Sampler      sdktrace.Sampler
	SampleRatio  float64
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

// WithLogLevel sets the minimum log level to be written to stdout.
func WithLogLevel(level slog.Level) Option {
	return func(c *factoryConfig) {
		c.LogLevel = level
	}
}

// WithSpanLogLevel sets the minimum log level to be attached to a trace span as an event.
func WithSpanLogLevel(level slog.Level) Option {
	return func(c *factoryConfig) {
		c.SpanLogLevel = level
	}
}

// WithSampler allows providing a custom OpenTelemetry Sampler.
// This option is more advanced and overrides WithSampleRatio.
func WithSampler(sampler sdktrace.Sampler) Option {
	return func(c *factoryConfig) {
		c.Sampler = sampler
	}
}

// WithSampleRatio sets a ratio-based sampler for traces.
// For example, a ratio of 0.1 will sample 10% of traces.
func WithSampleRatio(ratio float64) Option {
	return func(c *factoryConfig) {
		c.SampleRatio = ratio
	}
}

// Factory is responsible for creating Observability instances.
type Factory struct {
	config factoryConfig
	logger *slog.Logger
}

// NewFactory creates a new observability factory using functional options.
// It initializes with sensible defaults that can be overridden by the provided options.
func NewFactory(opts ...Option) *Factory {
	// Establish default configuration
	config := factoryConfig{
		ServiceName:  "unknown-service",
		ServiceApp:   "unknown-app",
		ServiceEnv:   "development",
		ApmType:      "none",
		ApmURL:       "", // No default URL
		LogLevel:     slog.LevelInfo,
		SpanLogLevel: slog.LevelInfo,
		SampleRatio:  1.0, // Default to sampling all traces
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
	// Handle log level environment variables
	if val := os.Getenv("OBS_LOG_LEVEL"); val != "" {
		config.LogLevel = parseLogLevel(val, slog.LevelInfo)
	}
	if val := os.Getenv("OBS_SPAN_LOG_LEVEL"); val != "" {
		config.SpanLogLevel = parseLogLevel(val, slog.LevelInfo)
	}
	if val := os.Getenv("OBS_SAMPLE_RATIO"); val != "" {
		if ratio, err := strconv.ParseFloat(val, 64); err == nil {
			config.SampleRatio = ratio
		}
	}

	// Set up the sampler
	if config.Sampler == nil {
		config.Sampler = sdktrace.ParentBased(sdktrace.TraceIDRatioBased(config.SampleRatio))
	}

	logger := newLogger(normalizeAPMType(config.ApmType), config.LogLevel, config.SpanLogLevel)

	return &Factory{
		config: config,
		logger: logger,
	}
}

// parseLogLevel converts a string to a slog.Level, with a default.
func parseLogLevel(levelStr string, defaultLevel slog.Level) slog.Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return defaultLevel
	}
}

// NewBackgroundObservability creates an Observability instance.
// It is intended for use in background processes or startup/shutdown hooks.
func (f *Factory) NewBackgroundObservability() *Observability {
	return NewObservability(f.config.ServiceName, f.config.ApmType, f.logger)
}

// SetupTracing initializes the global tracer provider based on the factory's configuration.
func (f *Factory) SetupTracing(ctx context.Context) (Shutdowner, error) {
	return setupTracing(ctx, f.config, f.logger)
}

// StartSpanFromRequest is the primary entry point for instrumenting an incoming HTTP request.
// It returns a new request with the full context, the final context itself, the created span, and the observability container.
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability) {
	// Extract the trace context from the incoming headers.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	// Create the stateless observability container.
	obs := NewObservability(f.config.ServiceName, f.config.ApmType, f.logger)

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

	// Start the span. This returns a new context containing the span.
	ctx, span := obs.StartSpan(ctx, r.URL.Path, defaultAttrs)

	// Put the obs object into the new context so it can be retrieved by app code.
	ctx = ctxWithObs(ctx, obs)

	// Update the request with this final, fully-populated context.
	r = r.WithContext(ctx)

	return r, ctx, span, obs
}
