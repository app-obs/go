package observability

import (
	"log/slog"
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
	h.obs.Log.Logc(slog.LevelError, 3, msg)
	http.Error(w, msg, statusCode)
}

// Record logs an error. The underlying logging handler will automatically
// record the error on the current trace span and set its status to Error.
// This is for recoverable errors that are returned up the call stack.
func (h *ErrorHandler) Record(err error, msg string) {
	h.obs.Log.Error(msg, "error", err)
}

// Fatal logs a fatal error and exits the application.
// This is for unrecoverable errors during startup.
func (h *ErrorHandler) Fatal(msg string, args ...any) {
	h.obs.Log.Logc(slog.LevelError, 3, msg, args...)
	os.Exit(1)
}
