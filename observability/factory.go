package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
)

// factoryConfig holds the static configuration for the observability system.
type factoryConfig struct {
	ServiceName      string
	ServiceApp       string
	ServiceEnv       string
	ApmType          string
	MetricsType      string
	ApmURL           string
	LogSource        bool
	SampleRate       float64
	LogLevel         slog.Level
	TraceLogLevel    slog.Level
	AsynchronousLogs bool
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
func WithApmType(apmType string) Option {
	return func(c *factoryConfig) {
		c.ApmType = apmType
	}
}

// WithMetricsType sets the desired metrics backend.
func WithMetricsType(metricsType string) Option {
	return func(c *factoryConfig) {
		c.MetricsType = metricsType
	}
}

// WithApmURL sets the endpoint URL for the APM collector.
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

// WithSampleRate sets the trace sampling rate.
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

// Factory is responsible for creating Observability instances.
type Factory struct {
	config factoryConfig
}

// NewFactory creates a new observability factory using functional options.
func NewFactory(opts ...Option) *Factory {
	config := factoryConfig{
		ServiceName:      "unknown-service",
		ServiceApp:       "unknown-app",
		ServiceEnv:       "development",
		ApmType:          "none",
		MetricsType:      "none",
		ApmURL:           "",
		LogSource:        true,
		SampleRate:       1.0,
		LogLevel:         slog.LevelDebug,
		TraceLogLevel:    slog.LevelInfo,
		AsynchronousLogs: false,
	}

	for _, opt := range opts {
		opt(&config)
	}

	// Read from environment variables, giving precedence to explicitly set options.
	if val := os.Getenv("OBS_SERVICE_NAME"); val != "" && config.ServiceName == "unknown-service" {
		config.ServiceName = val
	}
	if val := os.Getenv("OBS_APPLICATION"); val != "" && config.ServiceApp == "unknown-app" {
		config.ServiceApp = val
	}
	if val := os.Getenv("OBS_ENVIRONMENT"); val != "" && config.ServiceEnv == "development" {
		config.ServiceEnv = val
	}
	if val := os.Getenv("OBS_APM_TYPE"); val != "" && config.ApmType == "none" {
		config.ApmType = val
	}
	if val := os.Getenv("OBS_METRICS_TYPE"); val != "" && config.MetricsType == "none" {
		config.MetricsType = val
	}
	if val := os.Getenv("OBS_APM_URL"); val != "" && config.ApmURL == "" {
		config.ApmURL = val
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

	return &Factory{config: config}
}

// Setup initializes all observability components.
func (f *Factory) Setup(ctx context.Context) (Shutdowner, error) {
	var shutdowners []Shutdowner

	logShutdowner := f.setupLogging()
	shutdowners = append(shutdowners, logShutdowner)

	traceShutdowner, err := f.setupTracing(ctx)
	if err != nil {
		(&compositeShutdowner{shutdowners: shutdowners}).Shutdown(ctx)
		return nil, fmt.Errorf("failed to setup tracing: %w", err)
	}
	shutdowners = append(shutdowners, traceShutdowner)

	if normalizeMetricsType(f.config.MetricsType) == OTLPMetrics {
		metricsShutdowner, err := f.setupMetrics(ctx)
		if err != nil {
			(&compositeShutdowner{shutdowners: shutdowners}).Shutdown(ctx)
			return nil, fmt.Errorf("failed to setup metrics: %w", err)
		}
		shutdowners = append(shutdowners, metricsShutdowner)
	}

	return &compositeShutdowner{shutdowners: shutdowners}, nil
}

// SetupOrExit is a convenience wrapper around Setup.
func (f *Factory) SetupOrExit(fatalMsg string) Shutdowner {
	shutdowner, err := f.Setup(context.Background())
	if err != nil {
		LogFatal(fatalMsg, "error", err)
	}
	return shutdowner
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

// NewBackgroundObservability creates an Observability instance with a background context.
func (f *Factory) NewBackgroundObservability(ctx context.Context) *Observability {
	return NewObservability(ctx, f.config.ServiceName, f.config.ApmType, f.config.LogSource, f.config.LogLevel, f.config.TraceLogLevel, f.config.MetricsType == "otlp")
}

// StartSpanFromRequest instruments an incoming HTTP request.
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	obs := NewObservability(ctx, f.config.ServiceName, f.config.ApmType, f.config.LogSource, f.config.LogLevel, f.config.TraceLogLevel, f.config.MetricsType == "otlp")

	ctx, obs, span := obs.StartSpanWith(r.URL.Path,
		attribute.String("http.method", r.Method),
		attribute.String("http.url", r.URL.String()),
		attribute.String("http.target", r.URL.RequestURI()),
		attribute.String("http.host", r.Host),
		attribute.String("http.scheme", r.URL.Scheme),
	)

	if len(customAttrs) > 0 {
		for _, attrs := range customAttrs {
			for k, v := range attrs {
				span.SetAttributes(ToAttribute(k, v))
			}
		}
	}

	ctx = ctxWithObs(ctx, obs)
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

func (cs *compositeShutdowner) ShutdownOrLog(msg string) {
	shutdownWithDefaultTimeout(cs, msg)
}

func shutdownWithDefaultTimeout(s Shutdowner, msg string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := s.Shutdown(ctx); err != nil {
		LogShutdownError(msg, err)
	}
}