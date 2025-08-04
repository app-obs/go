package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// SpanAttributes provides a simpler, map-based way to define span attributes, similar to logrus.Fields.
type SpanAttributes map[string]interface{}

// StartSpan begins a new trace span using the tracer within the Observability container.
// It returns a new context containing the span, and the span itself.
func (o *Observability) StartSpan(ctx context.Context, name string, attrs SpanAttributes) (context.Context, Span) {
	ctx, span := o.Trace.Start(ctx, name)

	if len(attrs) > 0 {
		otelAttrs := make([]attribute.KeyValue, 0, len(attrs))
		for k, v := range attrs {
			// Use attribute.Any for more efficient and robust type handling.
			otelAttrs = append(otelAttrs, attribute.Any(k, v))
		}
		span.SetAttributes(otelAttrs...)
	}

	return ctx, span
}