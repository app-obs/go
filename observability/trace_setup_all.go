//go:build !datadog && !otlp && !none

package observability

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// setupDatadog configures and initializes the Datadog Tracer.
func setupDatadog(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	tracer.Start(
		tracer.WithService(serviceName),
		tracer.WithEnv(serviceEnv),
		tracer.WithServiceVersion(serviceApp),
		tracer.WithAgentAddr(apmURL),
		tracer.WithAnalyticsRate(sampleRate),
	)

	obs := NewObservability(ctx, serviceName, string(Datadog), true, slog.LevelDebug, slog.LevelInfo, false)
	obs.Log.Info("Datadog Tracer initialized successfully",
		"APMURL", apmURL,
		"APMType", Datadog,
		"SampleRate", sampleRate,
	)

	return &datadogShutdowner{}, nil
}

// datadogShutdowner implements the Shutdowner interface for Datadog.
type datadogShutdowner struct{}

// Shutdown stops the Datadog tracer.
func (d *datadogShutdowner) Shutdown(ctx context.Context) error {
	tracer.Stop()
	return nil
}

// ShutdownOrLog implements the Shutdowner interface for the datadogShutdowner.
func (d *datadogShutdowner) ShutdownOrLog(msg string) {
	d.Shutdown(context.Background())
}

// setupOTLP configures and initializes the OpenTelemetry TracerProvider and MeterProvider.
func setupOTLP(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		attribute.String("application", serviceApp),
		attribute.String("environment", serviceEnv),
	)

	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(apmURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRate)),
	)

	metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(apmURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &compositeShutdowner{
		shutdowners: []Shutdowner{
			&otlpShutdowner{provider: tp, name: "TracerProvider"},
			&otlpShutdowner{provider: mp, name: "MeterProvider"},
		},
	}, nil
}

// otlpShutdowner is a wrapper for OpenTelemetry providers to implement the full Shutdowner interface.
type otlpShutdowner struct {
	provider interface {
		Shutdown(context.Context) error
	}
	name string
}

// Shutdown calls the underlying provider's Shutdown method.
func (s *otlpShutdowner) Shutdown(ctx context.Context) error {
	if err := s.provider.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown %s: %w", s.name, err)
	}
	return nil
}

// ShutdownOrLog implements the Shutdowner interface.
func (s *otlpShutdowner) ShutdownOrLog(msg string) {
	shutdownWithDefaultTimeout(s, msg)
}

func setupNone(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	return &noOpShutdowner{}, nil
}

func init() {
	setupFuncs[Datadog] = setupDatadog
	setupFuncs[OTLP] = setupOTLP
	setupFuncs[None] = setupNone
}
