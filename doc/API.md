# API Reference

This document provides a detailed reference for the public API of the Go Observability Library.

## Table of Contents

- [Configuration](#configuration)
  - [`NewFactory`](#newfactory)
  - [Configuration Options](#configuration-options)
- [HTTP Request Handling](#http-request-handling)
  - [`Factory.StartSpanFromRequest`](#factorystartspanfromrequest)
- [Core Observability Object](#core-observability-object)
  - [`ObsFromCtx`](#obsfromctx)
  - [`Observability`](#observability)
- [Manual Span Management](#manual-span-management)
  - [`Observability.StartSpan`](#observabilitystartspan)
  - [`SpanAttributes`](#spanattributes)
- [Context Propagation](#context-propagation)
  - [`Trace.InjectHTTP`](#traceinjecthttp)
- [Attribute Helpers](#attribute-helpers)
  - [`String`, `Int`, `Bool`](#string-int-bool)

---

## Configuration

Configuration is handled by the `NewFactory` function, which accepts functional options. It is created once at application startup.

### `NewFactory`

Creates a new observability factory using functional options. The factory is the main entry point for the library. It initializes with sensible defaults (e.g., `ApmType: "none"`) that can be overridden by the provided `Option` functions.

```go
func NewFactory(opts ...Option) *Factory
```

**Example (Minimal):**
```go
// This factory will use defaults: service name will be "unknown-service"
// and tracing will be disabled ("none").
obsFactory := observability.NewFactory()
```

**Example (With Options):**
```go
obsFactory := observability.NewFactory(
    observability.WithServiceName("my-service"),
    observability.WithServiceApp("my-application"),
    observability.WithApmType("otlp"),
    observability.WithApmURL("http://otel-collector:4318/v1/traces"),
)
```

### Configuration Options

The following functions provide `Option`s to configure the factory:

- `WithServiceName(name string) Option`: Sets the service name.
- `WithServiceApp(app string) Option`: Sets the application name.
- `WithServiceEnv(env string) Option`: Sets the deployment environment (default: "development").
- `WithApmType(apmType string) Option`: Sets the APM backend ("otlp", "datadog", or "none").
- `WithApmURL(url string) Option`: Sets the APM collector URL.

### Environment Variable Fallbacks

As a convenience, the library will also read the following environment variables as a fallback if the corresponding functional options are not provided:

- `OBS_SERVICE_NAME`
- `OBS_APPLICATION`
- `OBS_ENVIRONMENT`
- `OBS_APM_TYPE`
- `OBS_APM_URL`

**Note:** Functional options always take precedence over environment variables.


---

## HTTP Request Handling

### `Factory.StartSpanFromRequest`

This is the primary entry point for instrumenting an incoming HTTP request. It performs several actions:
1. Extracts trace context from incoming headers.
2. Creates a new root span for the request.
3. Creates the `Observability` container.
4. Injects the `Observability` object into the request's context.
5. Returns an updated `*http.Request` and the `context.Context` containing the span and observability object.

```go
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability)
```

**Example (in an `http.HandlerFunc`):**
```go
func myHandler(w http.ResponseWriter, r *http.Request) {
    r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
    defer span.End()

    // Use ctx for all subsequent operations
    // ...
}
```

---

## Core Observability Object

### `ObsFromCtx`

Retrieves the `Observability` instance from a `context.Context`. This is the standard way to get access to the logger and tracer within your application logic.

```go
func ObsFromCtx(ctx context.Context) *Observability
```

**Example:**
```go
func processRequest(ctx context.Context) {
    obs := observability.ObsFromCtx(ctx)
    obs.Log.Info("Processing request")
    // ...
}
```

### `Observability`

The `Observability` struct is the main container for all instrumentation tools.

```go
type Observability struct {
    Trace        *Trace
    Log          *Log
    ErrorHandler *ErrorHandler
}
```

- **`Log`**: A structured logger (`slog`) that automatically includes `trace.id` and `span.id`.
- **`Trace`**: The tracer used for creating new spans and propagating context.
- **`ErrorHandler`**: A set of helper methods for consistent error handling.

---

## Manual Span Management

### `Observability.StartSpan`

Creates a new child span within an existing trace. This is used to instrument specific operations within a larger request flow.

```go
func (o *Observability) StartSpan(ctx context.Context, name string, attrs SpanAttributes) (context.Context, Span)
```

**Example:**
```go
func (s *myService) DoWork(ctx context.Context) {
    obs := observability.ObsFromCtx(ctx)
    ctx, span := obs.StartSpan(ctx, "DoWork.Internal", observability.SpanAttributes{
        "internal.id": 123,
    })
    defer span.End()

    // ... do work ...
}
```

### `SpanAttributes`

A convenience type alias for `map[string]interface{}` to make adding attributes to spans more readable.

```go
type SpanAttributes map[string]interface{}
```

---

## Context Propagation

### `Trace.InjectHTTP`

Injects the current trace context into the headers of an outgoing HTTP request. This is essential for propagating traces across service boundaries.

```go
func (t *Trace) InjectHTTP(req *http.Request)
```

**Example (in an HTTP client):**
```go
func callAnotherService(ctx context.Context) {
    obs := observability.ObsFromCtx(ctx)

    req, _ := http.NewRequestWithContext(ctx, "GET", "http://another-service/data", nil)

    // Inject trace headers into the outgoing request
    obs.Trace.InjectHTTP(req)

    http.DefaultClient.Do(req)
}
```

---

## Attribute Helpers

These functions are simple wrappers around the OpenTelemetry `attribute` package to create `KeyValue` pairs. They are provided for convenience to avoid an extra import.

### `String`, `Int`, `Bool`

```go
func String(key, value string) attribute.KeyValue
func Int(key string, value int) attribute.KeyValue
func Bool(key string, value bool) attribute.KeyValue
```

**Example:**
```go
span.SetAttributes(
    observability.String("user.id", "xyz"),
    observability.Bool("is.admin", false),
)
```
