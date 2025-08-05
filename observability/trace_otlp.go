//go:build otlp

package observability

import (
	"context"
	"net/http"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var (
	// unifiedSpanPool reduces allocations by reusing unifiedSpan objects.
	unifiedSpanPool = sync.Pool{
		New: func() interface{} {
			return new(unifiedSpan)
		},
	}
	otelTracer trace.Tracer
)

// unifiedSpan is a concrete implementation of the Span interface for OTLP.
type unifiedSpan struct {
	span trace.Span
	obs  *Observability
}

// End ends the span.
func (s *unifiedSpan) End() {
	s.span.End()
	s.span = nil
	s.obs = nil
	unifiedSpanPool.Put(s)
}

// AddEvent adds an event to the span.
func (s *unifiedSpan) AddEvent(name string, options ...trace.EventOption) {
	s.span.AddEvent(name, options...)
}

// RecordError records an error on the span.
func (s *unifiedSpan) RecordError(err error, options ...trace.EventOption) {
	s.span.RecordError(err, options...)
}

// SetStatus sets the status of the span.
func (s *unifiedSpan) SetStatus(code codes.Code, description string) {
	s.span.SetStatus(code, description)
}

// SetAttributes sets attributes on the span.
func (s *unifiedSpan) SetAttributes(attrs ...attribute.KeyValue) {
	s.span.SetAttributes(attrs...)
}

func init() {
	startSpan = func(t *Trace, ctx context.Context, spanName string) (context.Context, Span) {
		if t.apmType != OTLP {
			// When built with the otlp tag, only otlp is supported.
			return ctx, &noOpSpan{}
		}

		span := unifiedSpanPool.Get().(*unifiedSpan)
		span.obs = t.obs

		newCtx, otelSpan := otelTracer.Start(ctx, spanName)
		span.span = otelSpan

		return newCtx, span
	}

	injectHTTP = func(t *Trace, req *http.Request) {
		if t.apmType != OTLP {
			return
		}
		ctx := t.obs.Context()
		otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
	}

	initializeTracer = func(serviceName string) {
		otelTracer = otel.Tracer(serviceName)
	}
}

// noOpSpan is a no-op implementation of the Span interface.
type noOpSpan struct{}

func (s *noOpSpan) End()                                  {}
func (s *noOpSpan) AddEvent(string, ...trace.EventOption) {}
func (s *noOpSpan) RecordError(error, ...trace.EventOption) {}
func (s *noOpSpan) SetStatus(codes.Code, string)          {}
func (s *noOpSpan) SetAttributes(...attribute.KeyValue)   {}
