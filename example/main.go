package main

import (
	"context"
	"net/http"

	"github.com/app-obs/go/observability"
)

func main() {
	// 1. Configure the factory once during startup using functional options.
	// The library provides sensible defaults (like APMType="none"), so you only
	// need to set what you want to override.
	obsFactory := observability.NewFactory(
		observability.WithServiceName("my-service"),
		observability.WithServiceApp("my-application"),
		// For this example, we explicitly keep the default "none" APM type.
		// To send traces, you might use:
		// observability.WithApmType("otlp"),
		// observability.WithApmURL("http://localhost:4318/v1/traces"),
	)

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
		// and provides the observability "toolbox". The `obs` object is discarded
		// with `_` because it's not used directly in this handler.
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
	// 5. Get the observability object from the context.
	// Its internal context currently holds the parent span for the HTTP request.
	obs := observability.ObsFromCtx(ctx)

	// 6. This log is attached to the parent span ("/hello").
	obs.Log.Info("Handling hello request", "user-agent", r.UserAgent())

	// 7. Create a new, nested span for a specific business logic unit.
	// This updates the internal context of the 'obs' object to point to the new span.
	ctx, span := obs.StartSpan(ctx, "say-hello", observability.SpanAttributes{"name": "world"})
	defer span.End()

	w.Write([]byte("Hello, world!"))

	// 8. Because 'obs' is stateful, this log is now automatically attached
	// to the new child span ("say-hello").
	obs.Log.Info("Wrote response to client")
}
