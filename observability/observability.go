// Package observability provides a unified, opinionated framework for instrumenting
// Go services. It offers a consistent API for structured logging and distributed
// tracing, abstracting over concrete implementations like OpenTelemetry and Datadog.
//
// The primary entry point for consumers is the `Factory`, which is used to
// configure and create `Observability` instances. From there, users can access
// logging, tracing, and error handling capabilities.
package observability

import (
	"context"
	"log/slog"
)

// Shutdowner defines a contract for components that can be gracefully shut down.
type Shutdowner interface {
	// Shutdown attempts to gracefully shut down the component, respecting the
	// provided context for deadlines or cancellation.
	Shutdown(ctx context.Context) error

	// ShutdownOrLog is a convenience method that calls Shutdown with a default
	// timeout. If an error occurs, it logs the provided message and the error
	// to a fallback logger. This is ideal for simple defer calls in main.
	ShutdownOrLog(msg string)
}

// Observability holds the tracing and logging components.
type Observability struct {
	Trace        *Trace
	Log          *Log
	Metrics      *Metrics
	ErrorHandler *ErrorHandler
	ctx          context.Context
	serviceName  string
	apmType      APMType
}

// NewObservability creates a new Observability instance.
func NewObservability(ctx context.Context, serviceName string, apmType string, logSource bool, logLevel, traceLogLevel slog.Level, metrics bool) *Observability {
	typedAPMType := normalizeAPMType(apmType)
	obs := &Observability{
		ctx:         ctx,
		serviceName: serviceName,
		apmType:     typedAPMType,
	}
	// The factory is now responsible for initializing the logger.
	// We assume baseLogger is already initialized and available.
	obs.Trace = newTrace(obs, serviceName, typedAPMType)
	obs.Log = newLog(obs, baseLogger)
	obs.Metrics = newMetrics(obs)
	obs.ErrorHandler = newErrorHandler(obs) // Initialize the error handler

	if metrics {
		shutdowner, err := setupMetrics(ctx)
		if err != nil {
			obs.Log.Error("failed to setup metrics", "error", err)
		} else {
			// We might need to manage this shutdowner, but for now, we don't have a composite shutdowner here.
			// This will be handled in the factory.
			_ = shutdowner
		}
	}

	return obs
}

// Context returns the current context from the Observability instance.
func (o *Observability) Context() context.Context {
	return o.ctx
}

// clone creates a new Observability instance with a new context, ensuring
// that the original instance remains immutable.
func (o *Observability) clone(ctx context.Context) *Observability {
	// Create a shallow copy of the struct.
	newObs := *o
	// Set the new context.
	newObs.ctx = ctx

	// Re-initialize the components that depend on the observability object itself
	// to ensure they point to the new, cloned object, not the original.
	newObs.Trace = newTrace(&newObs, newObs.serviceName, newObs.apmType)
	newObs.Log = newLog(&newObs, baseLogger)
	newObs.Metrics = newMetrics(&newObs)
	newObs.ErrorHandler = newErrorHandler(&newObs)
	return &newObs
}

// noOpShutdowner implements the Shutdowner interface for components that need no shutdown logic.
type noOpShutdowner struct{}

// Shutdown is a no-op.
func (n *noOpShutdowner) Shutdown(ctx context.Context) error {
	return nil
}

// ShutdownOrLog is a no-op.
func (n *noOpShutdowner) ShutdownOrLog(msg string) {
	// Do nothing.
}
