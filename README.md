# Go Observability Library

An opinionated, unified observability library for Go services. This library provides a single, consistent API for structured logging, distributed tracing, and metrics, abstracting the concrete implementations of OpenTelemetry (OTLP) and Datadog.

Its primary goal is to make robust instrumentation easy, consistent, and highly performant across all microservices in a project.

For a detailed guide to the public API, see the [API Reference](./doc/API.md).

## Runnable Example

This repository contains the source code for the library itself. For a complete, runnable project demonstrating how to use this library in a real-world microservices environment, please see the [**example-services**](https://github.com/app-obs/example-services) repository.

## Features

- **Unified Tracing API**: Write your instrumentation code once and seamlessly switch between `OTLP` and `Datadog` backends via configuration.
- **High-Performance Logging**: Built on Go's standard `log/slog`, the logger is enriched with trace context and includes advanced performance features like optional asynchronous logging.
- **Automatic Runtime Metrics**: Automatically collect key Go runtime metrics (CPU, memory, GC, goroutines) with a single configuration flag.
- **Custom Metrics**: Create and track custom application-level metrics like counters, gauges, and histograms.
- **Configurable Sampling**: Control tracing overhead in production with head-based sampling to trace a percentage of requests (e.g., 10%) instead of all of them.
- **Granular Log Levels**: Independently control the log level for `stdout` and the level for logs attached to trace spans, allowing for quiet production logging with targeted trace verbosity.
- **Optimized HTTP Instrumentation**: A single-line, zero-allocation call (`obsFactory.StartSpanFromRequest(r)`) instruments an incoming HTTP request, handling context propagation, span naming, and standard attributes.
- **Zero-Allocation Primitives**: High-performance methods like `StartSpanWith` and `LogWithAttrs` are available for performance-critical code paths, avoiding memory allocations.

## Build Tags for Conditional Compilation

This library uses Go build tags to produce optimized, smaller binaries for production. By specifying a build tag, you instruct the Go compiler to include only the code for the backends you need.

-   **Why use build tags?** To significantly reduce the size of your final compiled application by excluding unused tracer and metrics libraries.
-   **What happens if you don't use a tag?** By default (no tags), the library compiles in **all** APM backends (OTLP and Datadog). This "kitchen sink" mode is useful for development, as it allows you to switch backends using the `OBS_APM_TYPE` environment variable without recompiling. For production, it is strongly recommended to use a build tag.

### Available Tags

-   `otlp`: Includes only the OpenTelemetry tracer.
-   `datadog`: Includes only the Datadog tracer.
-   `none`: Excludes all tracing code.
-   `metrics`: Includes the OpenTelemetry metrics SDK and enables automatic Go runtime metrics collection. This tag **must be combined** with the `otlp` tag.

### How to Use

You can specify tags using the `-tags` flag. For multiple tags, use a comma-separated string.

**OTLP Tracing with Metrics (Recommended for OTLP):**
```sh
go build -tags "otlp,metrics" -o my-service .
```

**OTLP Tracing only:**
```sh
go build -tags "otlp" -o my-service .
```

**Datadog Tracing only:**
```sh
go build -tags "datadog" -o my-service .
```

**No Tracing or Metrics (Smallest Binary):**
```sh
go build -tags "none" -o my-service .
```

## Getting Started

The following is a complete example of how to instrument a simple HTTP service using the recommended helper functions.

```go
package main

import (
	"context"
	"net/http"
	
	"github.com/app-obs/go/observability"
)

func main() {
	// 1. Create a factory once at startup.
	// Configuration is loaded from OBS_* environment variables by default.
	obsFactory := observability.NewFactory(
		observability.WithServiceName("my-service"),
	)
	
	// 2. Initialize all observability components and defer the shutdown.
	// SetupOrExit will log a fatal error and exit if initialization fails.
	shutdowner := obsFactory.SetupOrExit("Failed to setup observability")
	defer shutdowner.ShutdownOrLog("Error during observability shutdown")

	// 3. Get a background logger for startup and shutdown events.
	bgObs := obsFactory.NewBackgroundObservability(context.Background())

	// 4. Instrument your HTTP handlers.
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// This one line handles context propagation, creates the root span,
		// and provides the observability "toolbox".
		r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
		defer span.End()

		// Your handler logic uses the returned context.
		handleHello(ctx, w, r)
	})

	bgObs.Log.Info("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		bgObs.ErrorHandler.Fatal("Server failed to start", "error", err)
	}
}

func handleHello(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 5. Create a new span and get the observability container in one call.
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "say-hello", 
		observability.SpanAttributes{"name": "world"},
	)
	defer span.End()

	// 6. Use the logger; trace and span IDs are injected automatically into the log.
	obs.Log.Info("Handling hello request", "user-agent", r.UserAgent())

	w.Write([]byte("Hello, world!"))
}
```

## Metrics

The library supports both automatic runtime metrics and custom application metrics.

### Automatic Runtime Metrics

To enable the collection of runtime metrics, simply use the `WithRuntimeMetrics` option when creating the factory:

```go
obsFactory := observability.NewFactory(
    observability.WithRuntimeMetrics(true),
)
```

Or set the environment variable `OBS_RUNTIME_METRICS=true`.

This will automatically collect and export the following metrics:
- `runtime.cpu.usage`
- `runtime.mem.heap_alloc`
- `runtime.mem.heap_sys`
- `runtime.mem.heap_idle`
- `runtime.mem.heap_inuse`
- `runtime.goroutines`
- `runtime.gc.pause_total`
- `runtime.gc.count`

### Custom Metrics

You can create custom metrics from the `Observability` object. The following example shows how to create a counter to track the number of processed items.

```go
func processItem(ctx context.Context, itemID string) {
    obs := observability.ObsFromCtx(ctx)

    // 1. Create a counter. It's safe to call this multiple times; the
    // library will return the same instance for the same name.
    itemsProcessed, err := obs.Metrics.Counter("items_processed_total")
    if err != nil {
        obs.Log.Error("Failed to create counter", "error", err)
        // Handle error...
    }

    // 2. Increment the counter. You can add attributes to slice your
    // metrics by different dimensions.
    itemsProcessed.Add(ctx, 1, attribute.String("item_type", "widget"))

    // ... your processing logic ...
}
```

## Production Configuration & Performance

The library is designed for high performance in production environments. Configuration can be controlled via functional options or environment variables.

### Key Environment Variables

- `OBS_SERVICE_NAME` (string): **Effect:** Sets the `service.name` attribute on all traces and metrics.
- `OBS_APM_TYPE` (string): **Effect:** Selects the tracing backend. Valid values: `"otlp"`, `"datadog"`, `"none"`.
- `OBS_APM_URL` (string): **Effect:** Specifies the single endpoint where both traces and metrics will be sent (e.g., the address of your OpenTelemetry Collector).
- `OBS_SAMPLE_RATE` (float): **Effect:** Controls the percentage of requests that are traced. `1.0` traces everything, `0.1` traces 10%. **Setting this to a lower value (e.g., 0.05) is the most effective way to reduce tracing overhead.**
- `OBS_LOG_LEVEL` (string): **Effect:** Sets the minimum level for logs to be written to stdout. In a production environment, setting this to `"info"` or `"warn"` will significantly reduce log volume and improve performance. Valid values: `"debug"`, `"info"`, `"warn"`, `"error"`.
- `OBS_TRACE_LOG_LEVEL` (string): **Effect:** Sets the minimum level for logs to be attached to trace spans as events. This allows you to keep stdout quiet while still capturing important events in your traces.
- `OBS_LOG_SOURCE` (bool): **Effect:** If set to `"false"`, disables the automatic addition of source code file and line numbers to logs, providing a performance boost.
- `OBS_RUNTIME_METRICS` (bool): **Effect:** If set to `"true"`, enables automatic runtime metrics collection. **Note:** This feature is only supported when `OBS_APM_TYPE` is set to `"otlp"`. It will be automatically disabled for other types.

### Asynchronous Logging

For high-throughput services where logging performance is critical, you can enable non-blocking, asynchronous logging by setting `OBS_ASYNC_LOGS="true"`.

**How it Works:**
When enabled, log records are sent to a buffered in-memory channel and written to the final output (e.g., `stdout`) by a background goroutine. This prevents application code from blocking on I/O operations.

**Trade-offs:**
- **Performance:** Greatly reduces logging overhead on the application's critical path.
- **Reliability:** If the application crashes suddenly or the log volume exceeds the buffer capacity, a small number of the most recent log messages may be lost.

This feature is disabled by default to prioritize reliability. Enable it only when the performance benefits outweigh the risk of minor log loss.

---

A complete, runnable example can be found in the [`./example`](./example) directory. For more detailed instructions on how to run it with different backends, see the [./example/README.md](./example/README.md).
