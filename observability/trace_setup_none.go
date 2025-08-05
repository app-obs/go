//go:build none

package observability

import (
	"context"
	"fmt"
)

func setupNone(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	return &noOpShutdowner{}, nil
}

func init() {
	setupFuncs[None] = setupNone
	setupFuncs[Datadog] = func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
		return nil, fmt.Errorf("Datadog APM is not included in this build. Please use the 'none' build tag.")
	}
	setupFuncs[OTLP] = func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
		return nil, fmt.Errorf("OTLP APM is not included in this build. Please use the 'none' build tag.")
	}
}
