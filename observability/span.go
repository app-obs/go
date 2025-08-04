package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// SpanAttributes provides a simpler, map-based way to define span attributes, similar to logrus.Fields.
type SpanAttributes map[string]interface{}

// StartSpanFromCtx is a convenience function that gets the observability
// container from the context and starts a new span.
// It returns the new context, a new observability container associated with that
// context, and the created span.
func StartSpanFromCtx(ctx context.Context, name string, attrs SpanAttributes) (context.Context, *Observability, Span) {
	obs := ObsFromCtx(ctx)
	return obs.StartSpan(name, attrs)
}

// StartSpanFromCtxWith is a more performant version of StartSpanFromCtx that
// accepts a pre-built slice of attribute.KeyValue to avoid map processing overhead.
func StartSpanFromCtxWith(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, *Observability, Span) {
	obs := ObsFromCtx(ctx)
	return obs.StartSpanWith(name, attrs...)
}

// StartSpan begins a new trace span. Crucially, it returns a new Observability
// object associated with the new span's context. The original Observability
// object is un-changed. This ensures immutability and makes the library safe
// for concurrent use.
func (o *Observability) StartSpan(name string, attrs SpanAttributes) (context.Context, *Observability, Span) {
	ctx, span := o.Trace.Start(o.ctx, name)

	if len(attrs) > 0 {
		otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for k, v := range attrs {
			otelAttrs = append(otelAttrs, ToAttribute(k, v))
		}
		span.SetAttributes(otelAttrs...)
	}

	// Return a clone of the observability object with the new context.
	return ctx, o.clone(ctx), span
}

// StartSpanWith is the high-performance version of StartSpan. It also returns
// a new, cloned Observability object, preserving immutability.
func (o *Observability) StartSpanWith(name string, attrs ...attribute.KeyValue) (context.Context, *Observability, Span) {
	ctx, span := o.Trace.Start(o.ctx, name)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	// Return a clone of the observability object with the new context.
	return ctx, o.clone(ctx), span
}