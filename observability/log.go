package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
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
)

// InitLogger initializes the global logger and sets it as the default.
func InitLogger(apmType APMType) *slog.Logger {
	initOnce.Do(func() {
		jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		})
		logger := slog.New(NewAPMHandler(jsonHandler, apmType))
		slog.SetDefault(logger)
		baseLogger = logger
	})
	return baseLogger
}

// Log wraps the slog logger.
type Log struct {
	obs    *Observability
	logger *slog.Logger
}

// NewLog creates a new Log instance.
func NewLog(obs *Observability, baseLogger *slog.Logger) *Log {
	return &Log{
		obs:    obs,
		logger: baseLogger,
	}
}

func (l *Log) getCtx() context.Context {
	return l.obs.Context()
}

func (l *Log) log(level slog.Level, msg string, args ...any) {
	ctx := l.getCtx()
	if !l.logger.Enabled(ctx, level) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip [Callers, log, Info]
	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = l.logger.Handler().Handle(ctx, r)
}

func (l *Log) Debug(msg string, args ...any) {
	l.log(slog.LevelDebug, msg, args...)
}

func (l *Log) Info(msg string, args ...any) {
	l.log(slog.LevelInfo, msg, args...)
}

func (l *Log) Warn(msg string, args ...any) {
	l.log(slog.LevelWarn, msg, args...)
}

func (l *Log) Error(msg string, args ...any) {
	l.log(slog.LevelError, msg, args...)
}

func (l *Log) With(args ...any) *Log {
	return &Log{
		obs:    l.obs,
		logger: l.logger.With(args...),
	}
}

// --- APMHandler for slog integration ---

type APMHandler struct {
	slog.Handler
	attrs   []slog.Attr
	apmType APMType
}

func NewAPMHandler(baseHandler slog.Handler, apmType APMType) *APMHandler {
	return &APMHandler{
		Handler: baseHandler,
		apmType: apmType,
	}
}

func (h *APMHandler) Handle(ctx context.Context, r slog.Record) error {
	// Add trace and span IDs to the record's attributes
	traceID, spanID := h.getTraceSpanID(ctx)
	if traceID != "" {
		r.AddAttrs(slog.String("trace.id", traceID))
	}
	if spanID != "" {
		r.AddAttrs(slog.String("span.id", spanID))
	}

	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	switch h.apmType {
	case OTLP:
		h.handleOTLP(ctx, r, attrs)
	case DataDog:
		h.handleDataDog(ctx, r, attrs)
	}

	return h.Handler.Handle(ctx, r)
}

func (h *APMHandler) getTraceSpanID(ctx context.Context) (traceID, spanID string) {
	if h.apmType == OTLP {
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().HasTraceID() {
			traceID = span.SpanContext().TraceID().String()
		}
		if span.SpanContext().HasSpanID() {
			spanID = span.SpanContext().SpanID().String()
		}
	} else if h.apmType == DataDog {
		if ddSpan, ok := tracer.SpanFromContext(ctx); ok {
			traceID = fmt.Sprintf("%d", ddSpan.Context().TraceID())
			spanID = fmt.Sprintf("%d", ddSpan.Context().SpanID())
		}
	}
	return
}


func (h *APMHandler) handleOTLP(ctx context.Context, r slog.Record, slogAttrs []slog.Attr) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	otelAttrs := make([]attribute.KeyValue, 0, len(slogAttrs))
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

func (h *APMHandler) handleDataDog(ctx context.Context, r slog.Record, attrs []slog.Attr) {
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

func (h *APMHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &APMHandler{
		Handler: h.Handler.WithAttrs(attrs),
		attrs:   newAttrs,
		apmType: h.apmType,
	}
}

func (h *APMHandler) WithGroup(name string) slog.Handler {
	return &APMHandler{
		Handler: h.Handler.WithGroup(name),
		attrs:   h.attrs,
		apmType: h.apmType,
	}
}

func (h *APMHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}

