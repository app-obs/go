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

// configSource represents the origin of a configuration value.
type configSource string

const (
	sourceDefault     configSource = "default"
	sourceOption      configSource = "option"
	sourceEnv         configSource = "env"
	sourceHardcoded   configSource = "hardcoded"
	sourceCalculation configSource = "calculation"
)

// setting represents a single configuration value and its source.
type setting[T any] struct {
	Value  T
	Source configSource
}

// factoryConfig holds the static configuration for the observability system.
type factoryConfig struct {
	ServiceName      setting[string]
	ServiceApp       setting[string]
	ServiceEnv       setting[string]
	ApmType          setting[string]
	MetricsType      setting[string]
	ApmURL           setting[string]
	LogSource        setting[bool]
	SampleRate       setting[float64]
	LogLevel         setting[slog.Level]
	TraceLogLevel    setting[slog.Level]
	AsynchronousLogs setting[bool]
}

// Option is a function that configures a `factoryConfig`.
type Option func(*factoryConfig)

// WithServiceName sets the service name for tracing and metrics.
func WithServiceName(name string) Option {
	return func(c *factoryConfig) {
		c.ServiceName = setting[string]{Value: name, Source: sourceOption}
	}
}

// WithServiceApp sets the application or logical group name.
func WithServiceApp(app string) Option {
	return func(c *factoryConfig) {
		c.ServiceApp = setting[string]{Value: app, Source: sourceOption}
	}
}

// WithServiceEnv sets the deployment environment (e.g., "development", "production").
func WithServiceEnv(env string) Option {
	return func(c *factoryConfig) {
		c.ServiceEnv = setting[string]{Value: env, Source: sourceOption}
	}
}

// WithApmType sets the desired APM backend.
func WithApmType(apmType string) Option {
	return func(c *factoryConfig) {
		c.ApmType = setting[string]{Value: apmType, Source: sourceOption}
	}
}

// WithMetricsType sets the desired metrics backend.
func WithMetricsType(metricsType string) Option {
	return func(c *factoryConfig) {
		c.MetricsType = setting[string]{Value: metricsType, Source: sourceOption}
	}
}

// WithApmURL sets the endpoint URL for the APM collector.
func WithApmURL(url string) Option {
	return func(c *factoryConfig) {
		c.ApmURL = setting[string]{Value: url, Source: sourceOption}
	}
}

// WithLogSource enables or disables the automatic addition of source file and line number to logs.
func WithLogSource(enabled bool) Option {
	return func(c *factoryConfig) {
		c.LogSource = setting[bool]{Value: enabled, Source: sourceOption}
	}
}

// WithSampleRate sets the trace sampling rate.
func WithSampleRate(rate float64) Option {
	return func(c *factoryConfig) {
		c.SampleRate = setting[float64]{Value: rate, Source: sourceOption}
	}
}

// WithLogLevel sets the minimum level for logs written to stdout.
func WithLogLevel(level slog.Level) Option {
	return func(c *factoryConfig) {
		c.LogLevel = setting[slog.Level]{Value: level, Source: sourceOption}
	}
}

// WithTraceLogLevel sets the minimum level for logs attached to trace spans.
func WithTraceLogLevel(level slog.Level) Option {
	return func(c *factoryConfig) {
		c.TraceLogLevel = setting[slog.Level]{Value: level, Source: sourceOption}
	}
}

// WithAsynchronousLogging enables high-performance, non-blocking logging.
//
// When enabled, log records are sent to a buffered in-memory channel and written
// to the underlying output (e.g., stdout) by a separate goroutine. This can
// significantly improve application performance by preventing I/O waits on the
// critical path.
//
// Trade-offs:
//   - Performance: Greatly reduces logging overhead in the application's main goroutine.
//   - Reliability: In case of a sudden application crash or if the buffer fills up
//     (see OBS_ASYNC_LOG_BUFFER_SIZE), some recent log messages may be lost.
//
// Use this option for high-throughput services where performance is critical and
// the potential loss of a small number of recent logs during a crash is an
// acceptable trade-off. It is disabled by default for maximum reliability.
func WithAsynchronousLogging(enabled bool) Option {
	return func(c *factoryConfig) {
		c.AsynchronousLogs = setting[bool]{Value: enabled, Source: sourceOption}
	}
}

// Factory is responsible for creating Observability instances.
type Factory struct {
	config factoryConfig
}

// NewFactory creates a new observability factory using functional options.
func NewFactory(opts ...Option) *Factory {
	config := factoryConfig{
		ServiceName:      setting[string]{Value: "unknown-service", Source: sourceDefault},
		ServiceApp:       setting[string]{Value: "unknown-app", Source: sourceDefault},
		ServiceEnv:       setting[string]{Value: "development", Source: sourceDefault},
		ApmType:          setting[string]{Value: "none", Source: sourceDefault},
		MetricsType:      setting[string]{Value: "none", Source: sourceDefault},
		ApmURL:           setting[string]{Value: "", Source: sourceDefault},
		LogSource:        setting[bool]{Value: true, Source: sourceDefault},
		SampleRate:       setting[float64]{Value: 1.0, Source: sourceDefault},
		LogLevel:         setting[slog.Level]{Value: slog.LevelDebug, Source: sourceDefault},
		TraceLogLevel:    setting[slog.Level]{Value: slog.LevelInfo, Source: sourceDefault},
		AsynchronousLogs: setting[bool]{Value: false, Source: sourceDefault},
	}

	for _, opt := range opts {
		opt(&config)
	}

	// Read from environment variables, giving precedence to explicitly set options.
	if val := os.Getenv("OBS_SERVICE_NAME"); val != "" && config.ServiceName.Source == sourceDefault {
		config.ServiceName = setting[string]{Value: val, Source: sourceEnv}
	}
	if val := os.Getenv("OBS_APPLICATION"); val != "" && config.ServiceApp.Source == sourceDefault {
		config.ServiceApp = setting[string]{Value: val, Source: sourceEnv}
	}
	if val := os.Getenv("OBS_ENVIRONMENT"); val != "" && config.ServiceEnv.Source == sourceDefault {
		config.ServiceEnv = setting[string]{Value: val, Source: sourceEnv}
	}
	if val := os.Getenv("OBS_APM_TYPE"); val != "" && config.ApmType.Source == sourceDefault {
		config.ApmType = setting[string]{Value: val, Source: sourceEnv}
	}
	if val := os.Getenv("OBS_METRICS_TYPE"); val != "" && config.MetricsType.Source == sourceDefault {
		config.MetricsType = setting[string]{Value: val, Source: sourceEnv}
	}
	if val := os.Getenv("OBS_APM_URL"); val != "" && config.ApmURL.Source == sourceDefault {
		config.ApmURL = setting[string]{Value: val, Source: sourceEnv}
	}
	if val := os.Getenv("OBS_LOG_SOURCE"); val != "" && config.LogSource.Source == sourceDefault {
		if b, err := strconv.ParseBool(val); err == nil {
			config.LogSource = setting[bool]{Value: b, Source: sourceEnv}
		}
	}
	if val := os.Getenv("OBS_SAMPLE_RATE"); val != "" && config.SampleRate.Source == sourceDefault {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			config.SampleRate = setting[float64]{Value: f, Source: sourceEnv}
		}
	}
	if val := os.Getenv("OBS_LOG_LEVEL"); val != "" && config.LogLevel.Source == sourceDefault {
		config.LogLevel = setting[slog.Level]{Value: parseLogLevel(val), Source: sourceEnv}
	}
	if val := os.Getenv("OBS_TRACE_LOG_LEVEL"); val != "" && config.TraceLogLevel.Source == sourceDefault {
		config.TraceLogLevel = setting[slog.Level]{Value: parseLogLevel(val), Source: sourceEnv}
	}
	if val := os.Getenv("OBS_ASYNC_LOGS"); val != "" && config.AsynchronousLogs.Source == sourceDefault {
		if b, err := strconv.ParseBool(val); err == nil {
			config.AsynchronousLogs = setting[bool]{Value: b, Source: sourceEnv}
		}
	}

	return &Factory{config: config}
}

// logSettings logs the final configuration values and their sources.
func (f *Factory) logSettings() {
	slog.Info("Observability settings initialized",
		slog.Group("settings",
			slog.String("service_name", fmt.Sprintf("%s (source: %s)", f.config.ServiceName.Value, f.config.ServiceName.Source)),
			slog.String("service_app", fmt.Sprintf("%s (source: %s)", f.config.ServiceApp.Value, f.config.ServiceApp.Source)),
			slog.String("service_env", fmt.Sprintf("%s (source: %s)", f.config.ServiceEnv.Value, f.config.ServiceEnv.Source)),
			slog.String("apm_type", fmt.Sprintf("%s (source: %s)", f.config.ApmType.Value, f.config.ApmType.Source)),
			slog.String("metrics_type", fmt.Sprintf("%s (source: %s)", f.config.MetricsType.Value, f.config.MetricsType.Source)),
			slog.String("apm_url", fmt.Sprintf("%s (source: %s)", f.config.ApmURL.Value, f.config.ApmURL.Source)),
			slog.String("log_source", fmt.Sprintf("%t (source: %s)", f.config.LogSource.Value, f.config.LogSource.Source)),
			slog.String("sample_rate", fmt.Sprintf("%f (source: %s)", f.config.SampleRate.Value, f.config.SampleRate.Source)),
			slog.String("log_level", fmt.Sprintf("%s (source: %s)", f.config.LogLevel.Value, f.config.LogLevel.Source)),
			slog.String("trace_log_level", fmt.Sprintf("%s (source: %s)", f.config.TraceLogLevel.Value, f.config.TraceLogLevel.Source)),
			slog.String("async_logs", fmt.Sprintf("%t (source: %s)", f.config.AsynchronousLogs.Value, f.config.AsynchronousLogs.Source)),
		),
	)
}

// Setup initializes all observability components.
func (f *Factory) Setup(ctx context.Context) (Shutdowner, error) {
	var shutdowners []Shutdowner

	logShutdowner := f.setupLogging()
	shutdowners = append(shutdowners, logShutdowner)

	// Log settings after logger is initialized
	f.logSettings()

	traceShutdowner, err := f.setupTracing(ctx)
	if err != nil {
		(&compositeShutdowner{shutdowners: shutdowners}).Shutdown(ctx)
		return nil, fmt.Errorf("failed to setup tracing: %w", err)
	}
	shutdowners = append(shutdowners, traceShutdowner)

	if normalizeMetricsType(f.config.MetricsType.Value) == OTLPMetrics {
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
	_, shutdowner := initLogger(normalizeAPMType(f.config.ApmType.Value), f.config.LogSource.Value, f.config.LogLevel.Value, f.config.TraceLogLevel.Value, f.config.AsynchronousLogs.Value)
	return shutdowner
}

func (f *Factory) setupTracing(ctx context.Context) (Shutdowner, error) {
	return setupTracing(ctx, f.config.ServiceName.Value, f.config.ServiceApp.Value, f.config.ServiceEnv.Value, f.config.ApmURL.Value, f.config.ApmType.Value, f.config.SampleRate.Value)
}

func (f *Factory) setupMetrics(ctx context.Context) (Shutdowner, error) {
	return setupMetrics(ctx)
}

// NewBackgroundObservability creates an Observability instance with a background context.
func (f *Factory) NewBackgroundObservability(ctx context.Context) *Observability {
	return NewObservability(ctx, f.config.ServiceName.Value, f.config.ApmType.Value, f.config.LogSource.Value, f.config.LogLevel.Value, f.config.TraceLogLevel.Value, f.config.MetricsType.Value == "otlp")
}

// StartSpanFromRequest instruments an incoming HTTP request.
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability) {
	ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	obs := NewObservability(ctx, f.config.ServiceName.Value, f.config.ApmType.Value, f.config.LogSource.Value, f.config.LogLevel.Value, f.config.TraceLogLevel.Value, f.config.MetricsType.Value == "otlp")

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
