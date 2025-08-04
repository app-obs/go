package observability

import (
	"context"
	"fmt"
)

// SetupFunc defines the signature for functions that set up an APM provider.
type SetupFunc func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error)

// setupFuncs is a registry of APM setup functions, populated by build-tagged files.
var setupFuncs = make(map[APMType]SetupFunc)

// setupTracing initializes and configures the global TracerProvider based on APM type.
func setupTracing(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL, apmType string, sampleRate float64) (Shutdowner, error) {
	normalizedApmType := normalizeAPMType(apmType)

	setupFunc, ok := setupFuncs[normalizedApmType]
	if !ok {
		return nil, fmt.Errorf("unsupported APM type: %s", apmType)
	}

	return setupFunc(ctx, serviceName, serviceApp, serviceEnv, apmURL, sampleRate)
}
