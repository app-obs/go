package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

// factoryConfig holds the static configuration for the observability system.
// It is kept private to encourage the use of functional options.
type factoryConfig struct {
	ServiceName      string
	ServiceApp       string
	ServiceEnv       string
	ApmType          string
	ApmURL           string
	LogSource        bool
	SampleRate       float64
	LogLevel         slog.Level
	TraceLogLevel    slog.Level
	AsynchronousLogs bool
	RuntimeMetrics   bool
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

// WithSampleRate sets the trace sampling rate. A value of 1.0 traces everything, 0.5 traces 50%, etc.
func WithSampleRate(rate float64) Option {
	return func(c *factoryConfig) {
		c.SampleRate = rate
	}
}

// WithLogLevel sets the minimum level for logs written to stdout.
func WithLogLevel(level slog.Level) Option {
	return func(c *factoryConfig) {
		c.LogLevel = level
	}
}

// WithTraceLogLevel sets the minimum level for logs attached to trace spans.
func WithTraceLogLevel(level slog.Level) Option {
	return func(c *factoryConfig) {
		c.TraceLogLevel = level
	}
}

// WithAsynchronousLogging enables non-blocking logging to an in-memory buffer.
func WithAsynchronousLogging(enabled bool) Option {
	return func(c *factoryConfig) {
		c.AsynchronousLogs = enabled
	}
}

// WithRuntimeMetrics enables the collection of automatic runtime metrics.
func WithRuntimeMetrics(enabled bool) Option {
	return func(c *factoryConfig) {
		c.RuntimeMetrics = enabled
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
		ServiceName:      "unknown-service",
		ServiceApp:       "unknown-app",
		ServiceEnv:       "development",
		ApmType:          "none",
		ApmURL:           "", // No default URL
		LogSource:        true,
		SampleRate:       1.0,
		LogLevel:         slog.LevelDebug,
		TraceLogLevel:    slog.LevelInfo,
		AsynchronousLogs: false, // Default to reliable, blocking logs.
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
	if val := os.Getenv("OBS_LOG_SOURCE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			config.LogSource = b
		}
	}
	if val := os.Getenv("OBS_SAMPLE_RATE"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			config.SampleRate = f
		}
	}
	if val := os.Getenv("OBS_LOG_LEVEL"); val != "" {
		config.LogLevel = parseLogLevel(val)
	}
	if val := os.Getenv("OBS_TRACE_LOG_LEVEL"); val != "" {
		config.TraceLogLevel = parseLogLevel(val)
	}
	if val := os.Getenv("OBS_ASYNC_LOGS"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			config.AsynchronousLogs = b
		}
	}
	if val := os.Getenv("OBS_RUNTIME_METRICS"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			config.RuntimeMetrics = b
		}
	}

	return &Factory{config: config}
}

// Setup initializes all observability components (logging, tracing, metrics)
// and returns a composite shutdowner for graceful termination.
func (f *Factory) Setup(ctx context.Context) (Shutdowner, error) {
	var shutdowners []Shutdowner

	logShutdowner := f.setupLogging()
	shutdowners = append(shutdowners, logShutdowner)

	traceShutdowner, err := f.setupTracing(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to setup tracing: %w", err)
	}
	shutdowners = append(shutdowners, traceShutdowner)

	// Metrics are only supported for the "otlp" APM type.
	if normalizeAPMType(f.config.ApmType) != OTLP {
		if f.config.RuntimeMetrics {
			// Use a background logger to inform the user.
			bgObs := f.NewBackgroundObservability(context.Background())
			bgObs.Log.Info("Disabling runtime metrics; this feature is only supported when OBS_APM_TYPE is 'otlp'", "current_apm_type", f.config.ApmType)
			f.config.RuntimeMetrics = false
		}
	}

	if f.config.RuntimeMetrics {
		metricsShutdowner, err := f.setupMetrics(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to setup metrics: %w", err)
		}
		shutdowners = append(shutdowners, metricsShutdowner)
	}

	return &compositeShutdowner{shutdowners: shutdowners}, nil
}

func (f *Factory) setupLogging() Shutdowner {
	_, shutdowner := initLogger(normalizeAPMType(f.config.ApmType), f.config.LogSource, f.config.LogLevel, f.config.TraceLogLevel, f.config.AsynchronousLogs)
	return shutdowner
}

func (f *Factory) setupTracing(ctx context.Context) (Shutdowner, error) {
	return setupTracing(ctx, f.config.ServiceName, f.config.ServiceApp, f.config.ServiceEnv, f.config.ApmURL, f.config.ApmType, f.config.SampleRate)
}

func (f *Factory) setupMetrics(ctx context.Context) (Shutdowner, error) {
	return setupMetrics(ctx)
}

// NewBackgroundObservability creates an Observability instance with a background context,
// ideal for logging startup, shutdown, or other non-request-bound events.
func (f *Factory) NewBackgroundObservability(ctx context.Context) *Observability {
	return NewObservability(ctx, f.config.ServiceName, f.config.ApmType, f.config.LogSource, f.config.LogLevel, f.config.TraceLogLevel)
}

// StartSpanFromRequest is the primary entry point for instrumenting an incoming HTTP request.
// It returns a new request with the full context, the final context itself, the created span, and the observability container.
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability) {
	// Extract the trace context from the incoming headers.
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

	// Create the observability container.
	obs := NewObservability(ctx, f.config.ServiceName, f.config.ApmType, f.config.LogSource, f.config.LogLevel, f.config.TraceLogLevel)

	// Start the span using the new method. This returns a context with the span.
	ctx, span := obs.StartSpanWith(obs.Context(), r.URL.Path,
		attribute.String("http.method", r.Method),
		attribute.String("http.url", r.URL.String()),
		attribute.String("http.target", r.URL.RequestURI()),
		attribute.String("http.host", r.Host),
		attribute.String("http.scheme", r.URL.Scheme),
	)

	// Put the obs object into the new context that contains the span.
	ctx = ctxWithObs(ctx, obs)

	// Update the request with this final, fully-populated context.
	r = r.WithContext(ctx)

	return r, ctx, span, obs
}

func parseLogLevel(levelStr string) slog.Level {
	switch levelStr {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// compositeShutdowner calls Shutdown on multiple shutdowners.
type compositeShutdowner struct {
	shutdowners []Shutdowner
}

func (cs *compositeShutdowner) Shutdown(ctx context.Context) error {
	var errs []error
	for _, s := range cs.shutdowners {
		if err := s.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}