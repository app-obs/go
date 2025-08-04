//go:build metrics

package observability

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

func setupMetrics(ctx context.Context) (Shutdowner, error) {
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

// meter is responsible for collecting and exporting runtime metrics.
type meter struct {
	provider metric.MeterProvider
	meter    metric.Meter
	process  *process.Process
	done     chan struct{}
}

// newMeter creates a new meter for collecting runtime metrics.
func newMeter(provider metric.MeterProvider, p *process.Process) *meter {
	return &meter{
		provider: provider,
		meter:    provider.Meter("go-observability"),
		process:  p,
		done:     make(chan struct{}),
	}
}

// start begins the periodic collection of runtime metrics.
func (m *meter) start() error {
	// --- CPU Metrics ---
	cpuUsage, err := m.meter.Float64ObservableGauge("runtime.cpu.usage", metric.WithDescription("CPU usage percentage"), metric.WithUnit("1"))
	if err != nil {
		return err
	}

	// --- Memory Metrics ---
	heapAlloc, err := m.meter.Int64ObservableGauge("runtime.mem.heap_alloc", metric.WithDescription("Bytes of allocated heap objects"), metric.WithUnit("By"))
	if err != nil {
		return err
	}
	heapSys, err := m.meter.Int64ObservableGauge("runtime.mem.heap_sys", metric.WithDescription("Bytes of heap memory obtained from the OS"), metric.WithUnit("By"))
	if err != nil {
		return err
	}
	heapIdle, err := m.meter.Int64ObservableGauge("runtime.mem.heap_idle", metric.WithDescription("Bytes in idle (unused) heap spans"), metric.WithUnit("By"))
	if err != nil {
		return err
	}
	heapInuse, err := m.meter.Int64ObservableGauge("runtime.mem.heap_inuse", metric.WithDescription("Bytes in in-use heap spans"), metric.WithUnit("By"))
	if err != nil {
		return err
	}

	// --- Goroutine Metrics ---
	goroutines, err := m.meter.Int64ObservableGauge("runtime.goroutines", metric.WithDescription("Number of goroutines"))
	if err != nil {
		return err
	}

	// --- GC Metrics ---
	gcPauseTotal, err := m.meter.Float64ObservableCounter("runtime.gc.pause_total", metric.WithDescription("Total GC pause duration"), metric.WithUnit("s"))
	if err != nil {
		return err
	}
	gcCount, err := m.meter.Int64ObservableCounter("runtime.gc.count", metric.WithDescription("Total number of GC cycles"))
	if err != nil {
		return err
	}

	// Register the callback that will be periodically invoked.
	_, err = m.meter.RegisterCallback(
		func(_ context.Context, o metric.Observer) error {
			// CPU
			if percent, err := m.process.CPUPercent(); err == nil {
				o.ObserveFloat64(cpuUsage, percent/100) // Convert from percent to 0-1 range
			}

			// Memory
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)
			o.ObserveInt64(heapAlloc, int64(memStats.HeapAlloc))
			o.ObserveInt64(heapSys, int64(memStats.HeapSys))
			o.ObserveInt64(heapIdle, int64(memStats.HeapIdle))
			o.ObserveInt64(heapInuse, int64(memStats.HeapInuse))

			// Goroutines
			o.ObserveInt64(goroutines, int64(runtime.NumGoroutine()))

			// GC
			var gcStats debug.GCStats
			debug.ReadGCStats(&gcStats)
			o.ObserveFloat64(gcPauseTotal, gcStats.PauseTotal.Seconds())
			o.ObserveInt64(gcCount, gcStats.NumGC)

			return nil
		},
		cpuUsage, heapAlloc, heapSys, heapIdle, heapInuse, goroutines, gcPauseTotal, gcCount,
	)

	return err
}

// Shutdown stops the metric collection.
func (m *meter) Shutdown(ctx context.Context) error {
	// The meter provider's shutdown will handle the callback removal.
	return nil
}

// ShutdownOrLog implements the Shutdowner interface for the meter.
func (m *meter) ShutdownOrLog(msg string) {
	// The meter shutdown is a no-op, so no action is needed.
}