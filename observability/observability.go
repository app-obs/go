package observability

import (
	"context"
)

// v0.250801.1

// Shutdowner defines a contract for components that can be gracefully shut down.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Observability holds the tracing and logging components.
type Observability struct {
	Trace       *Trace
	Log         *Log
	ctx         context.Context
	serviceName string
	apmType     APMType
}

// NewObservability creates a new Observability instance.
func NewObservability(ctx context.Context, serviceName string, apmType string) *Observability {
	typedAPMType := normalizeAPMType(apmType)
	obs := &Observability{
		ctx:         ctx,
		serviceName: serviceName,
		apmType:     typedAPMType,
	}
	baseLogger := InitLogger(typedAPMType)
	obs.Trace = NewTrace(obs, serviceName, typedAPMType) // Pass obs and apmType to Trace
	obs.Log = NewLog(obs, baseLogger)                    // Pass obs to Log
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