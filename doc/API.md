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

Initializes all configured observability components (logging, tracing, metrics) and returns a single `Shutdowner` instance. This should be called once in your `main` function.

```go
func (f *Factory) Setup(ctx context.Context) (Shutdowner, error)
```

**Example:**
```go
obsFactory := observability.NewFactory(...)
shutdown, err := obsFactory.Setup(context.Background())
if err != nil {
    // handle error
}
defer shutdown.Shutdown(context.Background())
```

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
    ErrorHandler *ErrorHandler
}
```

---

## Manual Span Management

### `Observability.StartSpan`

Creates a new child span. This is a convenience method that accepts a `map[string]interface{}` for attributes, but has higher overhead than `StartSpanWith`.

```go
func (o *Observability) StartSpan(ctx context.Context, name string, attrs SpanAttributes) (context.Context, Span)
```

### `Observability.StartSpanWith`

A high-performance method for creating a new child span. It accepts a variadic slice of `attribute.KeyValue`, which avoids the overhead of map processing and type switching. This is the preferred method for performance-critical code.

```go
func (o *Observability) StartSpanWith(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, Span)
```

**Example:**
```go
ctx, span := obs.StartSpanWith(ctx, "ProcessItem",
    observability.String("item.id", item.ID),
    observability.Int("item.size", item.Size),
)
defer span.End()
```

### `SpanAttributes`

A convenience type alias for `map[string]interface{}` used by `StartSpan`.

```go
type SpanAttributes map[string]interface{}
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