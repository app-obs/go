# Go Observability Library

An opinionated, unified observability library for Go services. This library provides a single, consistent API for structured logging and distributed tracing, abstracting the concrete implementations of OpenTelemetry (OTLP) and Datadog.

Its primary goal is to make robust instrumentation easy and consistent across all microservices in a project.

## Features

- **Unified Tracing API**: Write your instrumentation code once and seamlessly switch between `OTLP` and `Datadog` backends via configuration. Also supports a `none` type to disable tracing completely.
- **Trace-Aware Structured Logging**: Built on Go's standard `log/slog` package, all log entries are automatically enriched with `trace.id` and `span.id`, providing a seamless link between logs and traces.
- **High-Level HTTP Instrumentation**: A single-line call (`obsFactory.StartSpanFromRequest(r)`) is all that's needed to instrument an incoming HTTP request, automatically handling context propagation, span naming, and standard HTTP attributes.
- **Standardized Error Handling**: Provides an `ErrorHandler` "toolbox" (`obs.ErrorHandler`) with methods like `HTTP`, `Record`, and `Fatal` to ensure errors are handled consistently across your application.
- **Performance-Conscious**: Uses `sync.Pool` for logging attributes to significantly reduce memory allocations and GC pressure in high-throughput services.
- **Familiar API**: Includes a compatibility layer for developers accustomed to the standard `log` package (`obs.Log.Printf`, `obs.Log.Fatal`, etc.).

## Getting Started

Here is a complete example of how to instrument a simple HTTP service.

### In `main.go`

```go
package main

import (
	"context"
	"net/http"
	
	"github.com/app-obs/go/observability"
)

func main() {
	// 1. Configure the factory once during startup.
	factoryConfig := observability.FactoryConfig{
		ServiceName: "my-service",
		ServiceApp:  "my-application",
		ServiceEnv:  "development",
		ApmType:     "otlp", // or "datadog", "none"
		ApmURL:      "http://tempo:4318/v1/traces",
	}
	obsFactory := observability.NewFactory(factoryConfig)
	
	// 2. Get a background logger for startup/shutdown events.
	bgObs := obsFactory.NewBackgroundObservability(context.Background())

	// 3. Initialize the global tracer provider.
	tp, err := obsFactory.SetupTracing(context.Background())
	if err != nil {
		bgObs.ErrorHandler.Fatal("Failed to initialize TracerProvider", "error", err)
	}
	// Ensure traces are flushed on shutdown.
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			bgObs.Log.Error("Error shutting down TracerProvider", "error", err)
		}
	}()

	// 4. Instrument your HTTP handlers.
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// This one line handles context propagation, creates the root span,
		// and provides the observability "toolbox".
		r, ctx, span, obs := obsFactory.StartSpanFromRequest(r)
		defer span.End()

		// Your handler logic uses the returned context.
		handleHello(ctx, w, r)
	})

	bgObs.Log.Info("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleHello(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 5. Get the observability object from the context.
	obs := observability.ObsFromCtx(ctx)

	// 6. Use the logger; trace and span IDs are injected automatically.
	obs.Log.Info("Handling hello request", "user-agent", r.UserAgent())

	// 7. Create nested spans for business logic.
	ctx, span := obs.StartSpan(ctx, "say-hello", observability.SpanAttributes{"name": "world"})
	defer span.End()

	w.Write([]byte("Hello, world!"))
}
```

## Migration Guides

### Migrating from standard `log`

The `obs.Log` object provides a compatibility layer.

**Before:**
```go
import "log"

log.Printf("User %s logged in", user.ID)

if err != nil {
    log.Fatalf("Critical error: %v", err)
}
```

**After:**
```go
// Get obs from context
obs := observability.ObsFromCtx(ctx)

// Printf logs at DEBUG level and is now associated with a trace.
obs.Log.Printf("User %s logged in", user.ID)

if err != nil {
    // Fatalf uses the standardized ErrorHandler.
    obs.Log.Fatalf("Critical error: %v", err)
}
```

### Migrating from `slog`

The API is nearly identical. The main change is moving from a global logger to the request-scoped logger provided by the `obs` object.

**Before:**
```go
import "log/slog"

slog.Info("User logged in", "userID", user.ID)
```

**After:**
```go
// Get obs from context
obs := observability.ObsFromCtx(ctx)

// API is the same, but now includes trace and span IDs.
obs.Log.Info("User logged in", "userID", user.ID)
```

### Migrating from `logrus`

The concepts are very similar. `SpanAttributes` is analogous to `logrus.Fields`, and `obs.Log.With()` is analogous to `logrus.WithFields()`.

**Before:**
```go
import "github.com/sirupsen/logrus"

log := logrus.WithFields(logrus.Fields{
  "userID": user.ID,
})
log.Info("User logged in")
```

**After:**
```go
// Get obs from context
obs := observability.ObsFromCtx(ctx)

log := obs.Log.With("userID", user.ID)
log.Info("User logged in")
```

### Migrating from OpenTelemetry (`otel`)

This library wraps the standard `otel` calls to provide a simpler, more opinionated API.

**Before:**
```go
import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
)

tracer := otel.Tracer("my-tracer")
ctx, span := tracer.Start(ctx, "my-span")
span.SetAttributes(attribute.String("key", "value"))
defer span.End()
```

**After:**
```go
// Get obs from context
obs := observability.ObsFromCtx(ctx)

// The StartSpan helper handles tracer acquisition and attribute conversion.
ctx, span := obs.StartSpan(ctx, "my-span", observability.SpanAttributes{
    "key": "value",
})
defer span.End()
```

### Migrating from Datadog (`ddtrace`)

The library completely abstracts the Datadog tracer. The migration path is the same for both `v1` and `v2` of the `dd-trace-go` library.

**Before:**
```go
import "github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

span, ctx := tracer.StartSpanFromContext(ctx, "my-span")
span.SetTag("key", "value")
defer span.Finish()
```

**After:**
```go
// Get obs from context
obs := observability.ObsFromCtx(ctx)

// The code is now identical to the OpenTelemetry version and backend-agnostic.
ctx, span := obs.StartSpan(ctx, "my-span", observability.SpanAttributes{
    "key": "value",
})
defer span.End()
```
