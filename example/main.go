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

	// 2. Get a background observability instance for startup/shutdown events.
	bgObs := obsFactory.NewBackgroundObservability()
	bgCtx := context.Background()

	// 3. Initialize the global tracer provider.
	tp, err := obsFactory.SetupTracing(bgCtx)
	if err != nil {
		bgObs.ErrorHandler.Fatal(bgCtx, "Failed to initialize TracerProvider", "error", err)
	}
	// Ensure traces are flushed on shutdown.
	defer func() {
		if err := tp.Shutdown(bgCtx); err != nil {
			bgObs.Log.Error(bgCtx, "Error shutting down TracerProvider", "error", err)
		}
	}()

	// 4. Instrument your HTTP handlers.
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// This one line handles context propagation, creates the root span,
		// and embeds the observability "toolbox" in the context.
		r, ctx, span, _ := obsFactory.StartSpanFromRequest(r)
		defer span.End()

		// Your handler logic uses the returned context.
		handleHello(ctx, w, r)
	})

	bgObs.Log.Info(bgCtx, "Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		bgObs.ErrorHandler.Fatal(bgCtx, "Server failed to start", "error", err)
	}
}

func handleHello(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// 5. Get the stateless observability object from the context.
	obs := observability.ObsFromCtx(ctx)

	// 6. Pass the context explicitly to all logging methods.
	// This log is attached to the parent span ("/hello").
	obs.Log.Info(ctx, "Handling hello request", "user-agent", r.UserAgent())

	// 7. Create a new, nested span. This returns a new context containing the span.
	ctx, span := obs.StartSpan(ctx, "say-hello", observability.SpanAttributes{"name": "world"})
	defer span.End()

	w.Write([]byte("Hello, world!"))

	// 8. Pass the *new* context to subsequent log calls to associate them
	// with the new child span.
	obs.Log.Info(ctx, "Wrote response to client")
}
