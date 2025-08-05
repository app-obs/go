//go:build datadog

package observability

import (
	"context"
	"net/http"
	"sync"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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

// unifiedSpan is a concrete implementation of the Span interface for Datadog.
type unifiedSpan struct {
	span interface{}
	obs  *Observability
}

// End ends the span.
func (s *unifiedSpan) End() {
	if span, ok := s.span.(tracer.Span); ok {
		span.Finish()
	}
	s.span = nil
	s.obs = nil
	unifiedSpanPool.Put(s)
}

// AddEvent adds an event to the span.
func (s *unifiedSpan) AddEvent(name string, options ...trace.EventOption) {
	if span, ok := s.span.(tracer.Span); ok {
		span.SetTag("event", name)
	}
}

// RecordError records an error on the span.
func (s *unifiedSpan) RecordError(err error, options ...trace.EventOption) {
	if span, ok := s.span.(tracer.Span); ok {
		span.SetTag("error", err)
	}
}

// SetStatus sets the status of the span.
func (s *unifiedSpan) SetStatus(code codes.Code, description string) {
	if span, ok := s.span.(tracer.Span); ok {
		span.SetTag("status", description)
	}
}

// SetAttributes sets attributes on the span.
func (s *unifiedSpan) SetAttributes(attrs ...attribute.KeyValue) {
	if span, ok := s.span.(tracer.Span); ok {
		for _, attr := range attrs {
			span.SetTag(string(attr.Key), attr.Value.AsInterface())
		}
	}
}

func init() {
	startSpan = func(t *Trace, ctx context.Context, spanName string) (context.Context, Span) {
		if t.apmType != Datadog {
			// When built with the datadog tag, only datadog is supported.
			return ctx, &noOpSpan{}
		}

		span := unifiedSpanPool.Get().(*unifiedSpan)
		span.obs = t.obs

		ddSpan, newDdCtx := tracer.StartSpanFromContext(ctx, spanName)
		span.span = ddSpan

		return newDdCtx, span
	}

	injectHTTP = func(t *Trace, req *http.Request) {
		if t.apmType != Datadog {
			return
		}
		ctx := t.obs.Context()
		if span, ok := tracer.SpanFromContext(ctx); ok {
			tracer.Inject(span.Context(), tracer.HTTPHeadersCarrier(req.Header))
		}
	}

	initializeTracer = func(serviceName string) {
		// Datadog tracer is initialized via tracer.Start(), not here.
	}
}

// noOpSpan is a no-op implementation of the Span interface.
type noOpSpan struct{}

func (s *noOpSpan) End()                                  {}
func (s *noOpSpan) AddEvent(string, ...trace.EventOption) {}
func (s *noOpSpan) RecordError(error, ...trace.EventOption) {}
func (s *noOpSpan) SetStatus(codes.Code, string)          {}
func (s *noOpSpan) SetAttributes(...attribute.KeyValue)   {}
