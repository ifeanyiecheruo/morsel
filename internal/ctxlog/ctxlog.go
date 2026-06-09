package ctxlog

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// With returns a new context carrying logger.
func With(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// From returns the logger stored in ctx, falling back to slog.Default().
func From(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
