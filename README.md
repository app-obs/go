# Go Observability Library

An opinionated, unified observability library for Go services. This library provides a single, consistent API for structured logging and distributed tracing, abstracting the concrete implementations of OpenTelemetry (OTLP) and Datadog.

Its primary goal is to make robust instrumentation easy, consistent, and highly performant across all microservices in a project.

For a detailed guide to the public API, see the [API Reference](./doc/API.md).

## Features

- **Unified Tracing API**: Write your instrumentation code once and seamlessly switch between `OTLP` and `Datadog` backends via configuration.
- **High-Performance Logging**: Built on Go's standard `log/slog`, the logger is enriched with trace context and includes advanced performance features like optional asynchronous logging.
- **Configurable Sampling**: Control tracing overhead in production with head-based sampling to trace a percentage of requests (e.g., 10%) instead of all of them.
- **Granular Log Levels**: Independently control the log level for `stdout` and the level for logs attached to trace spans, allowing for quiet production logging with targeted trace verbosity.
- **Optimized HTTP Instrumentation**: A single-line, zero-allocation call (`obsFactory.StartSpanFromRequest(r)`) instruments an incoming HTTP request, handling context propagation, span naming, and standard attributes.
- **Zero-Allocation Primitives**: High-performance methods like `StartSpanWith` and `LogWithAttrs` are available for performance-critical code paths, avoiding memory allocations.
- **Production-Ready**: Includes advanced features like hot-reloading of log levels via Kubernetes ConfigMaps and optional collection of runtime performance metrics.

## Getting Started

The following is a complete example of how to instrument a simple HTTP service.

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	
	"github.com/app-obs/go/observability"
)

func main() {
	// 1. Create a factory once at startup.
	// Configuration is loaded from OBS_* environment variables by default.
	obsFactory := observability.NewFactory(
		observability.WithServiceName("my-service"),
		observability.WithLogLevel(slog.LevelInfo), // Set a higher log level for production
	)
	
	// 2. Initialize all observability components (logger, tracer, etc.).
	// This returns a single shutdown function for graceful termination.
	shutdown, err := obsFactory.Setup(context.Background())
	if err != nil {
		// Use a background logger for startup errors.
		bgObs := obsFactory.NewBackgroundObservability(context.Background())
		bgObs.ErrorHandler.Fatal("Failed to setup observability", "error", err)
	}
	defer shutdown.Shutdown(context.Background())

	// 3. Instrument your HTTP handlers.
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// This one line handles context propagation, creates the root span,
		// and provides the observability "toolbox".
		r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
		defer span.End()

		// Your handler logic uses the returned context.
		handleHello(ctx, w, r)
	})

	// Use a background logger for startup/shutdown events.
	bgObs := obsFactory.NewBackgroundObservability(context.Background())
	bgObs.Log.Info("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleHello(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 4. Get the observability object from the context.
	obs := observability.ObsFromCtx(ctx)

	// 5. Use the logger; trace and span IDs are injected automatically.
	obs.Log.Info("Handling hello request", "user-agent", r.UserAgent())

	// 6. Create nested spans for business logic.
	ctx, span := obs.StartSpan(ctx, "say-hello", observability.SpanAttributes{"name": "world"})
	defer span.End()

	w.Write([]byte("Hello, world!"))
}
```

## Production Configuration & Performance

The library is designed for high performance in production environments. Configuration can be controlled via functional options or environment variables.

### Key Environment Variables

**Note for Kubernetes Users:** A common pattern is to define these environment variables in a Kubernetes `ConfigMap` and then expose them to your application's deployment using `envFrom`. **A complete, practical example of this can be found in the [`./example/k8s`](./example/k8s) directory.**

- `OBS_SERVICE_NAME` (string): **Effect:** Sets the `service.name` attribute on all traces and metrics.
- `OBS_APM_TYPE` (string): **Effect:** Selects the tracing backend. Valid values: `"otlp"`, `"datadog"`, `"none"`.
- `OBS_APM_URL` (string): **Effect:** Specifies the endpoint where traces will be sent.
- `OBS_SAMPLE_RATE` (float): **Effect:** Controls the percentage of requests that are traced. `1.0` traces everything, `0.1` traces 10%. **Setting this to a lower value (e.g., 0.05) is the most effective way to reduce tracing overhead.**
- `OBS_LOG_LEVEL` (string): **Effect:** Sets the minimum level for logs to be written to stdout. In a production environment, setting this to `"info"` or `"warn"` will significantly reduce log volume and improve performance. Valid values: `"debug"`, `"info"`, `"warn"`, `"error"`.
- `OBS_TRACE_LOG_LEVEL` (string): **Effect:** Sets the minimum level for logs to be attached to trace spans as events. This allows you to keep stdout quiet while still capturing important events in your traces.
- `OBS_ASYNC_LOGS` (bool): **Effect:** If set to `"true"`, enables non-blocking, buffered logging. This provides a major performance boost by decoupling application logic from I/O, but risks losing a small number of logs if the application crashes.
- `OBS_LOG_SOURCE` (bool): **Effect:** If set to `"false"`, disables the automatic addition of source code file and line numbers to logs, providing a performance boost.

### Hot Reloading with Kubernetes

The library supports hot-reloading of configuration, which is ideal for managing log levels in a live Kubernetes environment without restarting pods.

1.  **Store Config in a ConfigMap:**
    ```yaml
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: my-service-config
    data:
      config.yaml: |
        log-level: "info"
        trace-log-level: "warn"
    ```

2.  **Mount the ConfigMap:** Mount the ConfigMap as a volume in your Deployment.

3.  **Enable in Code:** Use the `WithHotReload` option in your code:
    ```go
    obsFactory := observability.NewFactory(
        observability.WithHotReload("/etc/config/config.yaml"),
    )
    ```

When you `kubectl apply` a change to the ConfigMap, the library will automatically detect the change and update the log levels in the running application.

---

A complete, runnable example can be found in the [`./example`](./example) directory. For more detailed instructions on how to run it with different backends, see the [./example/README.md](./example/README.md).
