package observability

import (
	"net/http"
	"os"

	"go.opentelemetry.io/otel/codes"
)

// ErrorHandler provides convenience methods for handling errors in a standardized way.
type ErrorHandler struct {
	obs *Observability
}

// newErrorHandler creates a new error handler associated with an Observability instance.
func newErrorHandler(obs *Observability) *ErrorHandler {
	return &ErrorHandler{obs: obs}
}

// HTTP logs an error and writes a standard HTTP error response.
func (h *ErrorHandler) HTTP(w http.ResponseWriter, msg string, statusCode int) {
	h.obs.Log.Error(msg)
	http.Error(w, msg, statusCode)
}

// Record logs an error and records it to the current trace span, marking the span as failed.
// This is for recoverable errors that are returned up the call stack.
func (h *ErrorHandler) Record(span Span, err error, msg string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, msg)
	h.obs.Log.Error(msg, "error", err)
}

// Fatal logs a fatal error and exits the application.
// This is for unrecoverable errors during startup.
func (h *ErrorHandler) Fatal(msg string, args ...any) {
	h.obs.Log.Error(msg, args...)
	os.Exit(1)
}