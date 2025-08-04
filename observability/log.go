package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	baseLogger *slog.Logger
	initOnce   sync.Once

	// slogAttrPool reduces allocations by reusing slices for slog attributes.
	slogAttrPool = sync.Pool{
		New: func() interface{} {
			// Pre-allocate a slice with a reasonable capacity.
			s := make([]slog.Attr, 0, 16)
			return &s
		},
	}

	// otelAttrPool reduces allocations by reusing slices for OpenTelemetry attributes.
	otelAttrPool = sync.Pool{
		New: func() interface{} {
			// Pre-allocate a slice with a reasonable capacity.
			s := make([]attribute.KeyValue, 0, 16)
			return &s
		},
	}
)

// initLogger initializes the global logger and sets it as the default.
// It returns the logger and a shutdowner for graceful termination.
func initLogger(apmType APMType, logSource bool, logLevel, traceLogLevel slog.Level, async bool) (*slog.Logger, Shutdowner) {
	var shutdowner Shutdowner = &noOpShutdowner{}
	initOnce.Do(func() {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: logSource,
			Level:     logLevel,
		})

		var handler slog.Handler = newApmHandler(jsonHandler, apmType, traceLogLevel, logSource)

		if async {
			asyncHandler := newAsyncHandler(handler)
			handler = asyncHandler
			shutdowner = asyncHandler
		}

		logger := slog.New(handler)
		slog.SetDefault(logger)
		baseLogger = logger
	})
	return baseLogger, shutdowner
}

// Log wraps the slog logger.
type Log struct {
	obs    *Observability
	logger *slog.Logger
}

// newLog creates a new Log instance.
func newLog(obs *Observability, baseLogger *slog.Logger) *Log {
	return &Log{
		obs:    obs,
		logger: baseLogger,
	}
}

func (l *Log) getCtx() context.Context {
	return l.obs.Context()
}

// Logc is the centralized logging function. It accepts a call depth
// to ensure the log source is reported correctly, even from wrappers.
func (l *Log) Logc(level slog.Level, depth int, msg string, args ...any) {
	ctx := l.getCtx()
	if !l.logger.Enabled(ctx, level) {
		return
	}
	// The slog.Handler is responsible for adding the source location.
	// We pass a zero PC here to avoid the overhead of runtime.Callers.
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.Add(args...)
	_ = l.logger.Handler().Handle(ctx, r)
}

func (l *Log) Debug(msg string, args ...any) {
	l.Logc(slog.LevelDebug, 3, msg, args...)
}

func (l *Log) Info(msg string, args ...any) {
	l.Logc(slog.LevelInfo, 3, msg, args...)
}

func (l *Log) Warn(msg string, args ...any) {
	l.Logc(slog.LevelWarn, 3, msg, args...)
}

func (l *Log) Error(msg string, args ...any) {
	l.Logc(slog.LevelError, 3, msg, args...)
}

// LogWithAttrs provides a more performant logging method for high-frequency calls.
// It accepts a pre-built slice of slog.Attr to avoid the overhead of parsing variadic arguments.
// The call depth is fixed to 3, which assumes this method is not wrapped.
func (l *Log) LogWithAttrs(level slog.Level, msg string, attrs ...slog.Attr) {
	ctx := l.getCtx()
	if !l.logger.Enabled(ctx, level) {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.AddAttrs(attrs...)
	_ = l.logger.Handler().Handle(ctx, r)
}

func (l *Log) With(args ...any) *Log {
	return &Log{
		obs:    l.obs,
		logger: l.logger.With(args...),
	}
}

// --- Standard Log Compatibility Methods ---

// Printf formats and logs a message at the DEBUG level.
func (l *Log) Printf(format string, v ...any) {
	l.Logc(slog.LevelDebug, 3, fmt.Sprintf(format, v...))
}

// Println formats and logs a message at the DEBUG level.
func (l *Log) Println(v ...any) {
	l.Logc(slog.LevelDebug, 3, fmt.Sprint(v...))
}

// Fatalf formats a message, logs it as a fatal error, and exits the application.
func (l *Log) Fatalf(format string, v ...any) {
	l.obs.ErrorHandler.Fatal(fmt.Sprintf(format, v...))
}

// Fatal logs a message as a fatal error and exits the application.
func (l *Log) Fatal(v ...any) {
	l.obs.ErrorHandler.Fatal(fmt.Sprint(v...))
}

// Panicf formats a message, logs it as an error, and panics.
func (l *Log) Panicf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	l.Logc(slog.LevelError, 3, msg)
	panic(msg)
}

// Panic logs a message as an error and panics.
func (l *Log) Panic(v ...any) {
	msg := fmt.Sprint(v...)
	l.Logc(slog.LevelError, 3, msg)
	panic(msg)
}

// --- apmHandler for slog integration ---

type apmHandler struct {
	slog.Handler
	attrs         []slog.Attr
	apmType       APMType
	traceLogLevel slog.Level
	addSource     bool
}

func newApmHandler(baseHandler slog.Handler, apmType APMType, traceLogLevel slog.Level, addSource bool) *apmHandler {
	return &apmHandler{
		Handler:       baseHandler,
		apmType:       apmType,
		traceLogLevel: traceLogLevel,
		addSource:     addSource,
	}
}

func (h *apmHandler) Handle(ctx context.Context, r slog.Record) error {
	// Add source location if enabled.
	if h.addSource {
		var pcs [1]uintptr
		runtime.Callers(4, pcs[:]) // skip [Callers, Handle, logc, Info/Debug/etc.]
		r.PC = pcs[0]
	}

	// Add trace and span IDs to the record's attributes
	traceID, spanID := h.getTraceSpanID(ctx)
	if traceID != "" {
		r.AddAttrs(slog.String("trace.id", traceID))
	}
	if spanID != "" {
		r.AddAttrs(slog.String("span.id", spanID))
	}

	// Only attach to spans if the level is high enough.
	if r.Level >= h.traceLogLevel {
		// Use a pooled slice for attributes to reduce allocations.
		slogAttrsPtr := slogAttrPool.Get().(*[]slog.Attr)
		defer func() {
			// Reset the slice length and return it to the pool.
			*slogAttrsPtr = (*slogAttrsPtr)[:0]
			slogAttrPool.Put(slogAttrsPtr)
		}()
		slogAttrs := *slogAttrsPtr

		slogAttrs = append(slogAttrs, h.attrs...)
		r.Attrs(func(a slog.Attr) bool {
			slogAttrs = append(slogAttrs, a)
			return true
		})

		switch h.apmType {
		case OTLP:
			h.handleOTLP(ctx, r, slogAttrs)
		case Datadog:
			h.handleDatadog(ctx, r, slogAttrs)
		case None:
			// Do nothing
		}
	}

	return h.Handler.Handle(ctx, r)
}

func (h *apmHandler) getTraceSpanID(ctx context.Context) (traceID, spanID string) {
	if h.apmType == None {
		return "", ""
	}
	if h.apmType == OTLP {
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().HasTraceID() {
			traceID = span.SpanContext().TraceID().String()
		}
		if span.SpanContext().HasSpanID() {
			spanID = span.SpanContext().SpanID().String()
		}
	} else if h.apmType == Datadog {
		if ddSpan, ok := tracer.SpanFromContext(ctx); ok {
			traceID = ddSpan.Context().TraceID()
			spanID = strconv.FormatUint(ddSpan.Context().SpanID(), 10)
		}
	}
	return
}

func (h *apmHandler) handleOTLP(ctx context.Context, r slog.Record, slogAttrs []slog.Attr) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	// Use a pooled slice for OTel attributes.
	otelAttrsPtr := otelAttrPool.Get().(*[]attribute.KeyValue)
	defer func() {
		*otelAttrsPtr = (*otelAttrsPtr)[:0]
		otelAttrPool.Put(otelAttrsPtr)
	}()
	otelAttrs := *otelAttrsPtr

	for _, a := range slogAttrs {
		otelAttrs = append(otelAttrs, toOtelAttribute(a))
	}

	if r.Level >= slog.LevelError {
		err := extractError(r)
		span.RecordError(err, trace.WithAttributes(otelAttrs...))
		span.SetStatus(codes.Error, r.Message)
	} else {
		span.AddEvent(r.Message, trace.WithAttributes(otelAttrs...))
	}
}

func (h *apmHandler) handleDatadog(ctx context.Context, r slog.Record, attrs []slog.Attr) {
	if ddSpan, ok := tracer.SpanFromContext(ctx); ok {
		for _, a := range attrs {
			ddSpan.SetTag(a.Key, a.Value.String())
		}

		if r.Level >= slog.LevelError {
			err := extractError(r)
			ddSpan.SetTag("error", err)
		} else {
			ddSpan.SetTag("event", r.Message)
		}
	}
}

// extractError finds and returns an error from a slog record, or creates a new one.
func extractError(r slog.Record) error {
	var loggedErr error
	r.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "error" {
			if errVal, ok := attr.Value.Any().(error); ok {
				loggedErr = errVal
				return false // stop iterating
			}
		}
		return true
	})
	if loggedErr == nil {
		loggedErr = errors.New(r.Message)
	}
	return loggedErr
}

func toOtelAttribute(a slog.Attr) attribute.KeyValue {
	switch a.Value.Kind() {
	case slog.KindString:
		return attribute.String(a.Key, a.Value.String())
	case slog.KindInt64:
		return attribute.Int64(a.Key, a.Value.Int64())
	case slog.KindUint64:
		return attribute.Int64(a.Key, int64(a.Value.Uint64()))
	case slog.KindFloat64:
		return attribute.Float64(a.Key, a.Value.Float64())
	case slog.KindBool:
		return attribute.Bool(a.Key, a.Value.Bool())
	default:
		return attribute.String(a.Key, a.Value.String())
	}
}

func (h *apmHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &apmHandler{
		Handler:       h.Handler.WithAttrs(attrs),
		attrs:         newAttrs,
		apmType:       h.apmType,
		traceLogLevel: h.traceLogLevel,
		addSource:     h.addSource,
	}
}

func (h *apmHandler) WithGroup(name string) slog.Handler {
	return &apmHandler{
		Handler:       h.Handler.WithGroup(name),
		attrs:         h.attrs,
		apmType:       h.apmType,
		traceLogLevel: h.traceLogLevel,
		addSource:     h.addSource,
	}
}

func (h *apmHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

// --- asyncHandler for non-blocking logging ---

const defaultAsyncBufferSize = 10000

type asyncHandler struct {
	underlying slog.Handler
	records    chan slog.Record
	wg         sync.WaitGroup
}

func newAsyncHandler(underlying slog.Handler) *asyncHandler {
	h := &asyncHandler{
		underlying: underlying,
		records:    make(chan slog.Record, defaultAsyncBufferSize),
	}

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		for record := range h.records {
			_ = h.underlying.Handle(context.Background(), record)
		}
	}()

	return h
}

func (h *asyncHandler) Handle(ctx context.Context, r slog.Record) error {
	recordCopy := r.Clone()
	select {
	case h.records <- recordCopy:
		// Log sent successfully.
	default:
		// Channel is full, drop the log.
	}
	return nil
}

func (h *asyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.underlying.Enabled(ctx, level)
}

func (h *asyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return newAsyncHandler(h.underlying.WithAttrs(attrs))
}

func (h *asyncHandler) WithGroup(name string) slog.Handler {
	return newAsyncHandler(h.underlying.WithGroup(name))
}

func (h *asyncHandler) Shutdown(ctx context.Context) error {
	close(h.records)
	h.wg.Wait()
	return nil
}