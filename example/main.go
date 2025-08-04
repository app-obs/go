package main

import (
	"context"
	"net/http"

	"github.com/app-obs/go/observability"
)

func main() {
	// 1. Configure the factory once during startup.
	// The library provides sensible defaults and environment variable fallbacks.
	obsFactory := observability.NewFactory(
		observability.WithServiceName("my-service"),
		// To send traces to a local collector, you might use:
		// observability.WithApmType("otlp"),
		// observability.WithApmURL("http://localhost:4318/v1/traces"),
	)

	// 2. Initialize all observability components and defer the shutdown.
	// SetupOrExit will log a fatal error and exit if initialization fails.
	shutdowner := obsFactory.SetupOrExit("Failed to setup observability")
	defer shutdowner.ShutdownOrLog("Error during observability shutdown")

	// 3. Get a background logger for startup and shutdown events.
	bgObs := obsFactory.NewBackgroundObservability(context.Background())

	// 4. Instrument your HTTP handlers.
	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// This one line handles context propagation and creates the root span.
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
	// 5. Create a new span. This returns a new context, a new observability
	// object, and the span. The new 'obs' is tied to the new span's context.
	ctx, obs, span := observability.StartSpanFromCtx(ctx, "say-hello",
		observability.SpanAttributes{"name": "world"},
	)
	defer span.End()

	// 6. This log is now automatically attached to the new child span ("say-hello").
	obs.Log.Info("Handling hello request", "user-agent", r.UserAgent())

	w.Write([]byte("Hello, world!"))

	obs.Log.Info("Wrote response to client")
}
