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
)

// Shutdowner defines a contract for components that can be gracefully shut down.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Observability holds the tracing and logging components.
type Observability struct {
	Trace        *Trace
	Log          *Log
	ErrorHandler *ErrorHandler
	ctx          context.Context
	serviceName  string
	apmType      APMType
}

// NewObservability creates a new Observability instance.
func NewObservability(ctx context.Context, serviceName string, apmType string) *Observability {
	typedAPMType := normalizeAPMType(apmType)
	obs := &Observability{
		ctx:         ctx,
		serviceName: serviceName,
		apmType:     typedAPMType,
	}
	baseLogger := initLogger(typedAPMType)
	obs.Trace = newTrace(obs, serviceName, typedAPMType)
	obs.Log = newLog(obs, baseLogger)
	obs.ErrorHandler = newErrorHandler(obs) // Initialize the error handler
	return obs
}

// Context returns the current context from the Observability instance.
func (o *Observability) Context() context.Context {
	return o.ctx
}

// SetContext updates the context in the Observability instance.
func (o *Observability) SetContext(ctx context.Context) {
	o.ctx = ctx
}
