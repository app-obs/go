package observability

import "context"

// obsKey is a private type to prevent collisions with other packages.
type obsKey struct{}

// ctxWithObs returns a new context with the Observability instance stored.
func ctxWithObs(ctx context.Context, obs *Observability) context.Context {
	return context.WithValue(ctx, obsKey{}, obs)
}

// ObsFromCtx retrieves the Observability instance from the context.
// If no instance is found, it returns a default, non-operational instance.
func ObsFromCtx(ctx context.Context) *Observability {
	if obs, ok := ctx.Value(obsKey{}).(*Observability); ok {
		return obs
	}
	// Return a default instance to prevent panics.
	return NewObservability(context.Background(), "unknown", "none")
}
