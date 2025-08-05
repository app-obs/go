//go:build datadog

package observability

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

// setupDatadog configures and initializes the Datadog Tracer.
func setupDatadog(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
	tracer.Start(
		tracer.WithService(serviceName),
		tracer.WithEnv(serviceEnv),
		tracer.WithServiceVersion(serviceApp),
		tracer.WithAgentAddr(apmURL),
		tracer.WithAnalyticsRate(sampleRate),
	)

	obs := NewObservability(ctx, serviceName, string(Datadog), true, slog.LevelDebug, slog.LevelInfo, false)
	obs.Log.Info("Datadog Tracer initialized successfully",
		"APMURL", apmURL,
		"APMType", Datadog,
		"SampleRate", sampleRate,
	)

	return &datadogShutdowner{}, nil
}

// datadogShutdowner implements the Shutdowner interface for Datadog.
type datadogShutdowner struct{}

// Shutdown stops the Datadog tracer.
func (d *datadogShutdowner) Shutdown(ctx context.Context) error {
	tracer.Stop()
	return nil
}

// ShutdownOrLog implements the Shutdowner interface for the datadogShutdowner.
func (d *datadogShutdowner) ShutdownOrLog(msg string) {
	d.Shutdown(context.Background())
}

func init() {
	setupFuncs[Datadog] = setupDatadog
	setupFuncs[OTLP] = func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
		return nil, fmt.Errorf("OTLP APM is not included in this build. Please use the 'datadog' build tag.")
	}
	setupFuncs[None] = func(ctx context.Context, serviceName, serviceApp, serviceEnv, apmURL string, sampleRate float64) (Shutdowner, error) {
		return &noOpShutdowner{}, nil
	}
}
