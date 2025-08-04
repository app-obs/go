# API Reference

This document provides a detailed reference for the public API of the Go Observability Library.

## Table of Contents

- [Initialization](#initialization)
  - [`NewFactory`](#newfactory)
  - [`Factory.Setup`](#factorysetup)
- [Configuration Options](#configuration-options)
  - [Service Identity](#service-identity)
  - [APM & Tracing](#apm--tracing)
  - [Logging](#logging)
  - [Metrics](#metrics)
  - [Environment Variable Fallbacks](#environment-variable-fallbacks)
- [HTTP Request Handling](#http-request-handling)
  - [`Factory.StartSpanFromRequest`](#factorystartspanfromrequest)
- [Core Observability Object](#core-observability-object)
  - [`ObsFromCtx`](#obsfromctx)
  - [`Observability`](#observability)
- [Manual Span Management](#manual-span-management)
  - [`Observability.StartSpan`](#observabilitystartspan)
  - [`Observability.StartSpanWith`](#observabilitystartspanwith)
  - [`SpanAttributes`](#spanattributes)
- [High-Performance Logging](#high-performance-logging)
  - [`Log.LogWithAttrs`](#loglogwithattrs)
- [Custom Metrics](#custom-metrics)
  - [`Metrics.Counter`](#metricscounter)
- [Context Propagation](#context-propagation)
  - [`Trace.InjectHTTP`](#traceinjecthttp)
- [Advanced Usage ("Escape Hatches")](#advanced-usage-escape-hatches)
  - [`Trace.OtelTracer`](#traceoteltracer)
  - [`Span.OtelSpan`](#spanotelspan)
  - [`Span.DatadogSpan`](#spandatadogspan)
- [Attribute Helpers](#attribute-helpers)
  - [`String`, `Int`, `Bool`](#string-int-bool)

---

## Initialization

### `NewFactory`

Creates a new observability factory using functional options. The factory is the main entry point for the library. It is created once at application startup.

```go
func NewFactory(opts ...Option) *Factory
```

### `Factory.Setup`

Initializes all configured observability components (logging, tracing, metrics) and returns a single `Shutdowner` instance. This should be called once in your `main` function. You should use this function if you need to handle initialization errors with custom logic.

```go
func (f *Factory) Setup(ctx context.Context) (Shutdowner, error)
```

### `Factory.SetupOrExit` (Recommended)

A convenience wrapper around `Setup`. It initializes all components and returns a `Shutdowner`. If any error occurs during setup, it logs a fatal message and exits the application. This is the recommended method for most applications as it simplifies `main` function logic.

```go
func (f *Factory) SetupOrExit(fatalMsg string) Shutdowner
```

**Example:**
```go
// In main.go
obsFactory := observability.NewFactory(...)

// 1. Initialize all observability components, exiting on failure.
shutdowner := obsFactory.SetupOrExit("Failed to setup observability")

// 2. Defer the shutdown call.
defer shutdowner.ShutdownOrLog("Error during observability shutdown")

// ... rest of your application
```

The returned `Shutdowner` object has two methods:

- `Shutdown(ctx context.Context) error`: Attempts to gracefully shut down all components, respecting a context for deadlines. Returns an error if any component fails to shut down.
- `ShutdownOrLog(msg string)`: The recommended convenience method. It calls `Shutdown` with a default internal timeout (10s) and automatically logs any error that occurs. This is perfect for a `defer` statement.

---

## Configuration Options

### Service Identity

- `WithServiceName(name string) Option`: Sets the service name (e.g., "user-service").
- `WithServiceApp(app string) Option`: Sets the application or logical group name (e.g., "ecommerce").
- `WithServiceEnv(env string) Option`: Sets the deployment environment (default: "development").

### APM & Tracing

- `WithApmType(apmType string) Option`: Sets the APM backend ("otlp", "datadog", or "none").
- `WithApmURL(url string) Option`: Sets the APM collector URL.
- `WithSampleRate(rate float64) Option`: Sets the trace sampling rate. `1.0` traces every request, `0.1` traces 10%. Default is `1.0`. This is the most effective way to control tracing overhead in production.

### Logging

- `WithLogLevel(level slog.Level) Option`: Sets the minimum level for logs written to stdout. Default is `slog.LevelDebug`.
- `WithTraceLogLevel(level slog.Level) Option`: Sets the minimum level for logs to be attached to trace spans as events. Default is `slog.LevelInfo`.
- `WithLogSource(enabled bool) Option`: Toggles adding the source file and line number to logs. Enabled by default. Disabling this in production provides a performance boost.
- `WithAsynchronousLogging(enabled bool) Option`: Enables non-blocking, buffered logging. This provides a significant performance gain but risks losing logs during a crash. Disabled by default.

### Metrics

- `WithRuntimeMetrics(enabled bool) Option`: Enables the automatic collection of Go runtime metrics (CPU, memory, GC, goroutines). Disabled by default.

### Environment Variable Fallbacks

As a convenience, the library will also read the following environment variables as a fallback if the corresponding functional options are not provided. Functional options always take precedence.

**Note for Kubernetes Users:** A common pattern is to define these environment variables in a Kubernetes `ConfigMap` and then expose them to your application's deployment using `envFrom`.

- `OBS_SERVICE_NAME` (string): Sets the service name used in traces and metrics.
- `OBS_APPLICATION` (string): Sets the application name, used for grouping services.
- `OBS_ENVIRONMENT` (string): Sets the deployment environment (e.g., "production").
- `OBS_APM_TYPE` (string): Sets the APM backend. Valid values: `"otlp"`, `"datadog"`, `"none"`.
- `OBS_APM_URL` (string): The endpoint URL for the APM collector.
- `OBS_SAMPLE_RATE` (float): The trace sampling rate. `1.0` traces everything, `0.1` traces 10%.
- `OBS_LOG_LEVEL` (string): The minimum level for logs written to stdout. Valid values: `"debug"`, `"info"`, `"warn"`, `"error"`.
- `OBS_TRACE_LOG_LEVEL` (string): The minimum level for logs attached to trace spans. Valid values: `"debug"`, `"info"`, `"warn"`, `"error"`.
- `OBS_LOG_SOURCE` (bool): Set to `"false"` to disable adding source code location to logs for a performance boost.
- `OBS_ASYNC_LOGS` (bool): Set to `"true"` to enable high-performance, non-blocking logging.
- `OBS_RUNTIME_METRICS` (bool): Set to `"true"` to enable automatic runtime metrics collection.

---

## HTTP Request Handling

### `Factory.StartSpanFromRequest`

This is the primary entry point for instrumenting an incoming HTTP request. It is highly optimized and performs several actions:
1. Extracts trace context from incoming headers.
2. Creates a new root span for the request.
3. Creates and injects the `Observability` object into the request's context.
4. Returns an updated `*http.Request`, the `context.Context`, the `Span`, and the `Observability` object.

```go
func (f *Factory) StartSpanFromRequest(r *http.Request, customAttrs ...SpanAttributes) (*http.Request, context.Context, Span, *Observability)
```

---

## Core Observability Object

### `ObsFromCtx`

Retrieves the `Observability` instance from a `context.Context`.

```go
func ObsFromCtx(ctx context.Context) *Observability
```

### `Observability`

The `Observability` struct is the main container for all instrumentation tools.

```go
type Observability struct {
    Trace        *Trace
    Log          *Log
    Metrics      *Metrics
    ErrorHandler *ErrorHandler
}
```

---

## Manual Span Management

### `StartSpanFromCtx` (Recommended)

A convenience function that gets the observability container from the context and starts a new span. It returns the new context, the observability container, and the created span. This is the recommended way to create spans.

```go
func StartSpanFromCtx(ctx context.Context, name string, attrs SpanAttributes) (context.Context, *Observability, Span)
```

### `StartSpanFromCtxWith` (Recommended, High-Performance)

A more performant version of `StartSpanFromCtx` that accepts a pre-built slice of `attribute.KeyValue` to avoid map processing overhead.

```go
func StartSpanFromCtxWith(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, *Observability, Span)
```

**Example:**
```go
ctx, obs, span := observability.StartSpanFromCtxWith(ctx, "ProcessItem",
    observability.String("item.id", item.ID),
    observability.Int("item.size", item.Size),
)
defer span.End()

// You can now use the obs object for logging, etc.
obs.Log.Info("Processing item")
```

### `SpanAttributes`

A convenience type alias for `map[string]interface{}` used by `StartSpanFromCtx`.

```go
type SpanAttributes map[string]interface{}
```

### `Observability.StartSpan` (Advanced)

Creates a new child span. This method is available on the `Observability` object but it is generally recommended to use the `StartSpanFromCtx` helper functions instead. It uses the context already stored within the `Observability` object.

```go
func (o *Observability) StartSpan(name string, attrs SpanAttributes) (context.Context, Span)
```

### `Observability.StartSpanWith` (Advanced, High-Performance)

A high-performance method for creating a new child span from the `Observability` object.

```go
func (o *Observability) StartSpanWith(name string, attrs ...attribute.KeyValue) (context.Context, Span)
```

---

## High-Performance Logging

### `Log.LogWithAttrs`

A high-performance logging method that bypasses the parsing of variadic key-value pairs. It is ideal for structured, high-frequency logging in performance-sensitive code.

```go
func (l *Log) LogWithAttrs(level slog.Level, msg string, attrs ...slog.Attr)
```

**Example:**
```go
obs.Log.LogWithAttrs(slog.LevelDebug, "Item processed",
    slog.String("item.id", item.ID),
    slog.Int("item.size", item.Size),
)
```

---

## Custom Metrics

### `Metrics.Counter`

Creates or retrieves a `float64` counter metric. Counters are monotonic, meaning their value can only increase. They are useful for tracking things like the number of requests, items processed, or errors.

```go
func (m *Metrics) Counter(name string, opts ...metric.Float64CounterOption) (metric.Float64Counter, error)
```

**Example:**
```go
// In initialization code:
itemsProcessed, err := obs.Metrics.Counter("items_processed_total")
if err != nil {
    // handle error
}

// In application code:
itemsProcessed.Add(ctx, 1.0, attribute.String("item_type", "widget"))
```

---

## Context Propagation

### `Trace.InjectHTTP`

Injects the current trace context into the headers of an outgoing HTTP request.

```go
func (t *Trace) InjectHTTP(req *http.Request)
```

---

## Advanced Usage ("Escape Hatches")

These methods provide direct access to the underlying APM-specific objects when you need functionality not exposed by the unified API.

### `Trace.OtelTracer`

Returns the underlying OpenTelemetry `trace.Tracer`.

```go
func (t *Trace) OtelTracer() trace.Tracer
```

### `Span.OtelSpan`

Returns the underlying OpenTelemetry `trace.Span`. Returns `nil` if the backend is not OTLP.

```go
func (s *Span) OtelSpan() trace.Span
```

### `Span.DatadogSpan`

Returns the underlying Datadog `tracer.Span`. Returns `nil` if the backend is not Datadog.

```go
func (s *Span) DatadogSpan() tracer.Span
```

---

## Attribute Helpers

These functions are simple wrappers to create `attribute.KeyValue` pairs for use with `StartSpanWith`.

### `String`, `Int`, `Bool`

```go
func String(key, value string) attribute.KeyValue
func Int(key string, value int) attribute.KeyValue
func Bool(key string, value bool) attribute.KeyValue
```
