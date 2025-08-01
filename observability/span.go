package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
)

// SpanAttributes provides a simpler, map-based way to define span attributes, similar to logrus.Fields.
type SpanAttributes map[string]interface{}

// StartSpan begins a new trace span, automatically converting a map of attributes to the required OpenTelemetry format.
// It returns the new context, the created span, and the observability instance for further use (e.g., logging).
func StartSpan(ctx context.Context, name string, attrs SpanAttributes) (context.Context, Span, *Observability) {
	obs := ObsFromCtx(ctx)
	ctx, span := obs.Trace.Start(ctx, name)

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

	return ctx, span, obs
}
