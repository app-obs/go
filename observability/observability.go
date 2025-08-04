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

// v0.250801.1

// Shutdowner defines a contract for components that can be gracefully shut down.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Observability holds the tracing and logging components. It is a stateless
// toolbox; all of its methods require a context to be passed explicitly.
type Observability struct {
	Trace        *Trace
	Log          *Log
	ErrorHandler *ErrorHandler
	serviceName  string
	apmType      APMType
}

// NewObservability creates a new Observability instance.
func NewObservability(serviceName string, apmType string, logger *slog.Logger) *Observability {
	typedAPMType := normalizeAPMType(apmType)
	obs := &Observability{
		serviceName: serviceName,
		apmType:     typedAPMType,
	}
	obs.Trace = newTrace(obs, serviceName, typedAPMType)
	obs.Log = newLog(obs, logger)
	obs.ErrorHandler = newErrorHandler(obs) // Initialize the error handler
	return obs
}
