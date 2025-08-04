package observability

import (
	"context"
	"fmt"

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
			switch val := v.(type) {
			case string:
				otelAttrs = append(otelAttrs, attribute.String(k, val))
			case int:
				otelAttrs = append(otelAttrs, attribute.Int(k, val))
			case int64:
				otelAttrs = append(otelAttrs, attribute.Int64(k, val))
			case bool:
				otelAttrs = append(otelAttrs, attribute.Bool(k, val))
			case float64:
				otelAttrs = append(otelAttrs, attribute.Float64(k, val))
			default:
				// As a safe fallback, convert any other type to a string.
				otelAttrs = append(otelAttrs, attribute.String(k, fmt.Sprintf("%v", v)))
			}
		}
		span.SetAttributes(otelAttrs...)
	}

	return ctx, span
}

// StartSpanWith provides a more performant way to create a span with attributes.
// It accepts a pre-built slice of attribute.KeyValue to avoid the overhead of map processing and type switching.
func (o *Observability) StartSpanWith(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, Span) {
	ctx, span := o.Trace.Start(ctx, name)
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	return ctx, span
}