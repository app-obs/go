# Go Observability Library

An opinionated, unified observability library for Go services. This library provides a single, consistent API for structured logging and distributed tracing, abstracting the concrete implementations of OpenTelemetry (OTLP) and Datadog.

Its primary goal is to make robust instrumentation easy and consistent across all microservices in a project.

## Features

- **Unified Tracing API**: Write your instrumentation code once and seamlessly switch between `OTLP` and `Datadog` backends via configuration. Also supports a `none` type to disable tracing completely.
- **Automatic Log Correlation**: Built on Go's standard `log/slog` package, any log created using a request-scoped logger (`obs.Log`) is automatically enriched with `trace.id` and `span.id`, providing a seamless link between logs and traces in your backend. Note that only logs at the `INFO` level or higher are attached to the trace as span events; `DEBUG` logs are excluded to reduce noise.
- **High-Level HTTP Instrumentation**: A single-line call (`obsFactory.StartSpanFromRequest(r)`) is all that's needed to instrument an incoming HTTP request, automatically handling context propagation, span naming, and standard HTTP attributes.
- **Standardized Error Handling**: Provides an `ErrorHandler` "toolbox" (`obs.ErrorHandler`) with methods like `HTTP`, `Record`, and `Fatal` to ensure errors are handled consistently across your application.
- **Performance-Conscious**: Uses `sync.Pool` for logging attributes to significantly reduce memory allocations and GC pressure in high-throughput services.
- **Familiar API**: Includes a compatibility layer for developers accustomed to the standard `log` package (`obs.Log.Printf`, `obs.Log.Fatal`, etc.).

## Getting Started

A complete, runnable example can be found in the [`./example`](./example) directory.

## Running the Example

To run the example server, navigate to the example directory and use `go run`:

```sh
# From the root of the 'go' module directory
cd example
go run .
```

The server will start on port `8080`. You can test it by sending a request from another terminal:

```sh
curl http://localhost:8080/hello
```

You will see structured, JSON-formatted logs in your terminal that include `trace.id` and `span.id` fields, demonstrating the core functionality of the library.

For more detailed instructions on how to run the example with different backends (like a generic OTLP collector or the Datadog agent), see the [./example/README.md](./example/README.md).

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
