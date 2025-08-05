
package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span is a unified interface for a trace span.
// The underlying implementation is determined by build tags.
type Span interface {
	End()
	AddEvent(string, ...trace.EventOption)
	RecordError(error, ...trace.EventOption)
	SetStatus(codes.Code, string)
	SetAttributes(...attribute.KeyValue)
}

// Trace holds the active tracer and APM type.
type Trace struct {
	obs     *Observability
	apmType APMType
}

// Start creates a new span. The actual implementation is provided by a
// build-specific file (`trace_otlp.go`, `trace_datadog.go`, etc.).
func (t *Trace) Start(ctx context.Context, spanName string) (context.Context, Span) {
	return startSpan(t, ctx, spanName)
}

// InjectHTTP injects the current trace context into HTTP headers. The actual
// implementation is provided by a build-specific file.
func (t *Trace) InjectHTTP(req *http.Request) {
	injectHTTP(t, req)
}

// newTrace creates a new Trace instance.
func newTrace(obs *Observability, serviceName string, apmType APMType) *Trace {
	// The serviceName is used by the OTel tracer, which is initialized
	// in the build-specific files.
	initializeTracer(serviceName)

	return &Trace{
		obs:     obs,
		apmType: apmType,
	}
}

/*
The following functions and variables must be implemented by a build-specific file
(e.g., trace_otlp.go, trace_datadog.go, trace_all.go, trace_none.go).
This approach ensures that we only compile the code for the selected APM provider.

var (
	// startSpan creates a new span.
	startSpan func(t *Trace, ctx context.Context, spanName string) (context.Context, Span)

	// injectHTTP injects the trace context into HTTP headers.
	injectHTTP func(t *Trace, req *http.Request)

	// initializeTracer sets up the tracer for the given service name.
	initializeTracer func(serviceName string)
)
*/
var (
	startSpan        func(t *Trace, ctx context.Context, spanName string) (context.Context, Span)
	injectHTTP       func(t *Trace, req *http.Request)
	initializeTracer func(serviceName string)
)
