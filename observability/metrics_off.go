//go:build !metrics

package observability

import (
	"context"
)

func setupMetrics(ctx context.Context) (Shutdowner, error) {
	return &noOpShutdowner{}, nil
}
