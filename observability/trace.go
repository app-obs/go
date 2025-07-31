package observability

import (
	"context"
	"fmt"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// Span is a unified interface for a trace span, wrapping both OTel and DataDog spans.
type Span interface {
	End()
	AddEvent(string, ...trace.EventOption)
	RecordError(error, ...trace.EventOption)
	SetStatus(codes.Code, string)
	SetAttributes(...attribute.KeyValue)
}

// Tracer is an interface for a tracer.
type Tracer interface {
	Start(ctx context.Context, spanName string) (context.Context, Span)
}

// unifiedSpan is a concrete implementation of the Span interface.
type unifiedSpan struct {
	trace.Span    // OTel span
	ddSpan     *tracer.Span
	obs        *Observability
	parentCtx  context.Context
	apmType    APMType
}

// End ends the span based on the APM type.
func (s *unifiedSpan) End() {
	s.obs.SetContext(s.parentCtx)
	if s.apmType == DataDog {
		s.ddSpan.Finish()
	} else {
		s.Span.End()
	}
}

// AddEvent adds an event to the span.
func (s *unifiedSpan) AddEvent(name string, options ...trace.EventOption) {
	if s.apmType == DataDog {
		s.ddSpan.SetTag("event", name)
	} else {
		s.Span.AddEvent(name, options...)
	}
}

// RecordError records an error on the span.
func (s *unifiedSpan) RecordError(err error, options ...trace.EventOption) {
	if s.apmType == DataDog {
		s.ddSpan.SetTag("error", err)
	} else {
		s.Span.RecordError(err, options...)
	}
}

// SetStatus sets the status of the span.
func (s *unifiedSpan) SetStatus(code codes.Code, description string) {
	if s.apmType == DataDog {
		s.ddSpan.SetTag("status", description)
	} else {
		s.Span.SetStatus(code, description)
	}
}

// SetAttributes sets attributes on the span.
func (s *unifiedSpan) SetAttributes(attrs ...attribute.KeyValue) {
	if s.apmType == DataDog {
		for _, attr := range attrs {
			s.ddSpan.SetTag(string(attr.Key), attr.Value.AsString())
		}
	} else {
		s.Span.SetAttributes(attrs...)
	}
}

// unifiedTracer is a unified tracer that can create either OTel or DataDog spans.
type unifiedTracer struct {
	obs    *Observability
	tracer trace.Tracer // OTel tracer
}

// Start creates a new span based on the APM type.
func (t *unifiedTracer) Start(ctx context.Context, spanName string) (context.Context, Span) {
	parentCtx := t.obs.Context()
	apmType := t.obs.Trace.apmType

	span := &unifiedSpan{
		obs:       t.obs,
		parentCtx: parentCtx,
		apmType:   apmType,
	}

	var newCtx context.Context
	if apmType == DataDog {
		ddSpan, newDdCtx := tracer.StartSpanFromContext(ctx, spanName)
		span.ddSpan = ddSpan
		newCtx = newDdCtx
	} else {
		var otelSpan trace.Span
		newCtx, otelSpan = t.tracer.Start(ctx, spanName)
		span.Span = otelSpan
	}

	t.obs.SetContext(newCtx)
	return newCtx, span
}

// Trace holds the active tracer and APM type.
type Trace struct {
	*unifiedTracer
	apmType APMType
}

// NewTrace creates a new Trace instance.
func NewTrace(obs *Observability, serviceName string, apmType APMType) *Trace {
	return &Trace{
		unifiedTracer: &unifiedTracer{
			obs:    obs,
			tracer: otel.Tracer(serviceName),
		},
		apmType: apmType,
	}
}

// SetupTracing initializes and configures the global TracerProvider based on APM type.
func SetupTracing(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, apmType string) (Shutdowner, error) {
	switch APMType(apmType) {
	case OTLP:
		return setupOTLP(ctx, serviceName, serviceApp, serviceEnv, apmURL)
	case DataDog:
		return setupDataDog(ctx, serviceName, serviceApp, serviceEnv, apmURL)
	default:
		return nil, fmt.Errorf("unsupported APM type: %s", apmType)
	}
}

// setupDataDog configures and initializes the DataDog Tracer.
func setupDataDog(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string) (Shutdowner, error) {
	tracer.Start(
		tracer.WithService(serviceName),
		tracer.WithEnv(serviceEnv),
		tracer.WithServiceVersion(serviceApp),
		tracer.WithAgentAddr(apmURL),
	)

	obs := NewObservability(ctx, serviceName, string(DataDog))
	obs.Log.Info("DataDog Tracer initialized successfully",
		"APMURL", apmURL,
		"APMType", DataDog,
	)

	return &dataDogShutdowner{}, nil
}

// dataDogShutdowner implements the Shutdowner interface for DataDog.
type dataDogShutdowner struct{}

// Shutdown stops the DataDog tracer.
func (d *dataDogShutdowner) Shutdown(ctx context.Context) error {
	tracer.Stop()
	return nil
}

// setupOTLP configures and initializes the OpenTelemetry TracerProvider.
func setupOTLP(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string) (Shutdowner, error) {
	exporter, err := newOTLPExporter(ctx, apmURL)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("application", serviceApp),
			attribute.String("environment", serviceEnv),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	obs := NewObservability(ctx, serviceName, string(OTLP))
	obs.Log.Info("OpenTelemetry TracerProvider initialized successfully",
		"APMURL", apmURL,
		"APMType", OTLP,
	)

	return tp, nil
}

// newOTLPExporter creates a new OTLP exporter.
func newOTLPExporter(ctx context.Context, apmURL string) (sdktrace.SpanExporter, error) {
	client := otlptracehttp.NewClient(
		otlptracehttp.WithEndpointURL(apmURL),
	)
	exporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}
	return exporter, nil
}
