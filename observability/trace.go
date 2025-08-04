package observability

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	// unifiedSpanPool reduces allocations by reusing unifiedSpan objects.
	unifiedSpanPool = sync.Pool{
		New: func() interface{} {
			return new(unifiedSpan)
		},
	}
)

// Span is a unified interface for a trace span, wrapping both OTel and Datadog spans.
type Span interface {
	End()
	AddEvent(string, ...trace.EventOption)
	RecordError(error, ...trace.EventOption)
	SetStatus(codes.Code, string)
	SetAttributes(...attribute.KeyValue)

	// OtelSpan provides an "escape hatch" to the underlying OpenTelemetry span.
	// It returns nil if the APM type is not OTLP.
	OtelSpan() trace.Span

	// DatadogSpan provides an "escape hatch" to the underlying Datadog span.
	// It returns nil if the APM type is not Datadog.
	DatadogSpan() tracer.Span
}

// noOpSpan is a no-op implementation of the Span interface.
type noOpSpan struct{}

func (s *noOpSpan) End()                                     {}
func (s *noOpSpan) AddEvent(string, ...trace.EventOption)    {}
func (s *noOpSpan) RecordError(error, ...trace.EventOption)  {}
func (s *noOpSpan) SetStatus(codes.Code, string)             {}
func (s *noOpSpan) SetAttributes(...attribute.KeyValue)      {}
func (s *noOpSpan) OtelSpan() trace.Span                     { return nil }
func (s *noOpSpan) DatadogSpan() tracer.Span                 { var sp tracer.Span; return sp }

// traceImpl is an interface for a tracer.
type traceImpl interface {
	Start(ctx context.Context, spanName string) (context.Context, Span)
}

// unifiedSpan is a concrete implementation of the Span interface.
type unifiedSpan struct {
	span      interface{} // Can be trace.Span or *tracer.Span
	obs       *Observability
	parentCtx context.Context
}

// End ends the span based on the APM type.
func (s *unifiedSpan) End() {
	s.obs.SetContext(s.parentCtx)
	switch span := s.span.(type) {
	case trace.Span:
		span.End()
	case tracer.Span:
		span.Finish()
	}
	// Reset the struct and put it back in the pool.
	s.span = nil
	s.obs = nil
	s.parentCtx = nil
	unifiedSpanPool.Put(s)
}

// AddEvent adds an event to the span.
func (s *unifiedSpan) AddEvent(name string, options ...trace.EventOption) {
	switch span := s.span.(type) {
	case trace.Span:
		span.AddEvent(name, options...)
	case tracer.Span:
		span.SetTag("event", name)
	}
}

// RecordError records an error on the span.
func (s *unifiedSpan) RecordError(err error, options ...trace.EventOption) {
	switch span := s.span.(type) {
	case trace.Span:
		span.RecordError(err, options...)
	case tracer.Span:
		span.SetTag("error", err)
	}
}

// SetStatus sets the status of the span.
func (s *unifiedSpan) SetStatus(code codes.Code, description string) {
	switch span := s.span.(type) {
	case trace.Span:
		span.SetStatus(code, description)
	case tracer.Span:
		span.SetTag("status", description)
	}
}

// SetAttributes sets attributes on the span.
func (s *unifiedSpan) SetAttributes(attrs ...attribute.KeyValue) {
	switch span := s.span.(type) {
	case trace.Span:
		span.SetAttributes(attrs...)
	case tracer.Span:
		for _, attr := range attrs {
			span.SetTag(string(attr.Key), attr.Value.AsInterface())
		}
	}
}

// OtelSpan returns the underlying OpenTelemetry span, or nil.
func (s *unifiedSpan) OtelSpan() trace.Span {
	if span, ok := s.span.(trace.Span); ok {
		return span
	}
	return nil
}

// DatadogSpan returns the underlying Datadog span, or nil.
func (s *unifiedSpan) DatadogSpan() tracer.Span {
	if span, ok := s.span.(tracer.Span); ok {
		return span
	}
	var ddSpan tracer.Span
	return ddSpan
}

// unifiedTracer is a unified tracer that can create either OTel or Datadog spans.
type unifiedTracer struct {
	obs    *Observability
	tracer trace.Tracer // OTel tracer
}

// Start creates a new span based on the APM type.
func (t *unifiedTracer) Start(ctx context.Context, spanName string) (context.Context, Span) {
	apmType := t.obs.Trace.apmType

	if apmType == None {
		return ctx, &noOpSpan{}
	}

	parentCtx := t.obs.Context()
	span := unifiedSpanPool.Get().(*unifiedSpan)
	span.obs = t.obs
	span.parentCtx = parentCtx

	var newCtx context.Context
	if apmType == Datadog {
		ddSpan, newDdCtx := tracer.StartSpanFromContext(ctx, spanName)
		span.span = ddSpan
		newCtx = newDdCtx
	} else {
		var otelSpan trace.Span
		newCtx, otelSpan = t.tracer.Start(ctx, spanName)
		span.span = otelSpan
	}

	t.obs.SetContext(newCtx)
	return newCtx, span
}

// Trace holds the active tracer and APM type.
type Trace struct {
	*unifiedTracer
	apmType APMType
}

// OtelTracer provides an "escape hatch" to the underlying OpenTelemetry tracer.
func (t *Trace) OtelTracer() trace.Tracer {
	return t.unifiedTracer.tracer
}

// newTrace creates a new Trace instance.
func newTrace(obs *Observability, serviceName string, apmType APMType) *Trace {
	return &Trace{
		unifiedTracer: &unifiedTracer{
			obs:    obs,
			tracer: otel.Tracer(serviceName),
		},
		apmType: apmType,
	}
}

// InjectHTTP injects the current trace context into the headers of an outgoing HTTP request.
// It automatically handles the correct propagation format for the configured APM type.
func (t *Trace) InjectHTTP(req *http.Request) {
	switch t.apmType {
	case OTLP:
		otel.GetTextMapPropagator().Inject(t.obs.Context(), propagation.HeaderCarrier(req.Header))
	case Datadog:
		if span, ok := tracer.SpanFromContext(t.obs.Context()); ok {
			tracer.Inject(span.Context(), tracer.HTTPHeadersCarrier(req.Header))
		}
	case None:
		// Do nothing
	}
}

// setupTracing initializes and configures the global TracerProvider based on APM type.
func setupTracing(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL, apmType string, sampleRate float64) (Shutdowner, error) {
	switch normalizeAPMType(apmType) {
	case OTLP:
		return setupOTLP(ctx, serviceName, serviceApp, serviceEnv, apmURL, sampleRate)
	case Datadog:
		return setupDatadog(ctx, serviceName, serviceApp, serviceEnv, apmURL, sampleRate)
	case None:
		return &noOpShutdowner{}, nil
	default:
		return nil, fmt.Errorf("unsupported APM type: %s", apmType)
	}
}

// setupDatadog configures and initializes the Datadog Tracer.
func setupDatadog(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	tracer.Start(
		tracer.WithService(serviceName),
		tracer.WithEnv(serviceEnv),
		tracer.WithServiceVersion(serviceApp),
		tracer.WithAgentAddr(apmURL),
		tracer.WithAnalyticsRate(sampleRate),
	)

	obs := NewObservability(ctx, serviceName, string(Datadog), true, slog.LevelDebug, slog.LevelInfo)
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

// setupOTLP configures and initializes the OpenTelemetry TracerProvider and MeterProvider.
func setupOTLP(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String(serviceName),
		attribute.String("application", serviceApp),
		attribute.String("environment", serviceEnv),
	)

	// Set up the trace exporter
	traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(apmURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(sampleRate)),
	)

	// Set up the metric exporter
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

	obs := NewObservability(ctx, serviceName, string(OTLP), true, slog.LevelDebug, slog.LevelInfo)
	obs.Log.Info("OpenTelemetry TracerProvider and MeterProvider initialized successfully",
		"APMURL", apmURL,
		"APMType", OTLP,
		"SampleRate", sampleRate,
	)

	// Return a composite shutdowner for both providers
	return &compositeShutdowner{
		shutdowners: []Shutdowner{tp, mp},
	}, nil
}

func setupMetrics(ctx context.Context) (Shutdowner, error) {
	// The meter provider is now set up in setupOTLP.
	// This function's role is to start the runtime metrics collection.
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("failed to get current process: %w", err)
	}
	meter := newMeter(otel.GetMeterProvider(), p)
	if err := meter.start(); err != nil {
		return nil, fmt.Errorf("failed to start runtime metrics: %w", err)
	}
	return meter, nil
}
