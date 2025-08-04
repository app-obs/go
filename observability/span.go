package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// SpanAttributes provides a simpler, map-based way to define span attributes, similar to logrus.Fields.
type SpanAttributes map[string]interface{}

// StartSpanFromCtx is a convenience function that gets the observability
// container from the context and starts a new span.
// It returns the new context, the observability container, and the created span.
func StartSpanFromCtx(ctx context.Context, name string, attrs SpanAttributes) (context.Context, *Observability, Span) {
	obs := ObsFromCtx(ctx)
	newCtx, span := obs.StartSpan(name, attrs)
	return newCtx, obs, span
}

// StartSpanFromCtxWith is a more performant version of StartSpanFromCtx that
// accepts a pre-built slice of attribute.KeyValue to avoid map processing overhead.
func StartSpanFromCtxWith(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, *Observability, Span) {
	obs := ObsFromCtx(ctx)
	newCtx, span := obs.StartSpanWith(name, attrs...)
	return newCtx, obs, span
}

// StartSpan begins a new trace span using the tracer within the Observability container.
// It uses the context already stored within the Observability object.
// It returns a new context containing the span, and the span itself.
func (o *Observability) StartSpan(name string, attrs SpanAttributes) (context.Context, Span) {
	ctx, span := o.Trace.Start(o.ctx, name)

	if len(attrs) > 0 {
		otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for k, v := range attrs {
			otelAttrs = append(otelAttrs, ToAttribute(k, v))
		}
		span.SetAttributes(otelAttrs...)
	}

	return ctx, span
}

// StartSpanWith provides a more performant way to create a span with attributes.
// It uses the context already stored within the Observability object.
// It accepts a pre-built slice of attribute.KeyValue to avoid the overhead of map processing and type switching.
func (o *Observability) StartSpanWith(name string, attrs ...attribute.KeyValue) (context.Context, Span) {
	ctx, span := o.Trace.Start(o.ctx, name)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}