//go:build none

package observability

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func init() {
	startSpan = func(t *Trace, ctx context.Context, spanName string) (context.Context, Span) {
		return ctx, &noOpSpan{}
	}

	injectHTTP = func(t *Trace, req *http.Request) {
		// Do nothing
	}

	initializeTracer = func(serviceName string) {
		// Do nothing
	}
}

// noOpSpan is a no-op implementation of the Span interface.
type noOpSpan struct{}

func (s *noOpSpan) End()                                  {}
func (s *noOpSpan) AddEvent(string, ...trace.EventOption) {}
func (s *noOpSpan) RecordError(error, ...trace.EventOption) {}
func (s *noOpSpan) SetStatus(codes.Code, string)          {}
func (s *noOpSpan) SetAttributes(...attribute.KeyValue)   {}