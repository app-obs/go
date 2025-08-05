//go:build otlp

package observability

import (
	"context"
	"fmt"

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

func init() {
	setupFuncs[OTLP] = setupOTLP
	setupFuncs[Datadog] = func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
		return nil, fmt.Errorf("Datadog APM is not included in this build. Please use the 'otlp' build tag.")
	}
	setupFuncs[None] = func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
		return &noOpShutdowner{}, nil
	}
}
