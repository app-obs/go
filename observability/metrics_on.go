//go:build metrics

package observability

import (
	"context"
	"fmt"
	"os"

	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel"
)

func setupMetrics(ctx context.Context) (Shutdowner, error) {
	// The meter provider is now set up in setupOTLP.
	// This function's role is to start the runtime metrics collection.
	p, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		return nil, fmt.Errorf("failed to get current process: %w", err)
	}
	meter := newMeter(otel.GetMeterProvider(), p)
	if err := meter.start(); err != nil {
		return nil, fmt.Errorf("failed to start runtime metrics: %w", err)
	}
	return meter, nil
}
