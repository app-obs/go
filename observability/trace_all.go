//go:build !datadog && !otlp && !none

package observability

import (
	"context"
	"net/http"
	"sync"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
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

// unifiedSpan is a concrete implementation of the Span interface.
type unifiedSpan struct {
	span      interface{} // Can be trace.Span or tracer.Span
	obs       *Observability
	parentCtx context.Context
}

// End ends the span based on the APM type.
func (s *unifiedSpan) End() {
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

func init() {
	startSpan = func(t *Trace, ctx context.Context, spanName string) (context.Context, Span) {
		if t.apmType == None {
			return ctx, &noOpSpan{}
		}

		parentCtx := t.obs.Context()
		span := unifiedSpanPool.Get().(*unifiedSpan)
		span.obs = t.obs
		span.parentCtx = parentCtx

		var newCtx context.Context
		if t.apmType == Datadog {
			ddSpan, newDdCtx := tracer.StartSpanFromContext(ctx, spanName)
			span.span = ddSpan
			newCtx = newDdCtx
		} else {
			var otelSpan trace.Span
			newCtx, otelSpan = otelTracer.Start(ctx, spanName)
			span.span = otelSpan
		}

		return newCtx, span
	}

	injectHTTP = func(t *Trace, req *http.Request) {
		ctx := t.obs.Context() // Always use the context from the parent observability object.
		switch t.apmType {
		case OTLP:
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))
		case Datadog:
			if span, ok := tracer.SpanFromContext(ctx); ok {
				tracer.Inject(span.Context(), tracer.HTTPHeadersCarrier(req.Header))
			}
		case None:
			// Do nothing
		}
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
