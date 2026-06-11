package ctxlog

import (
	"context"
	"log/slog"
)

type contextKey struct{}

var orphanedLogger = slog.Default().With("ctxlog_warning", "no logger in context; using default logger")

// With returns a new context carrying logger.
func With(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// From returns the logger stored in ctx. If no logger has been threaded into
// the context the caller gets a orphaned logger.
func From(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return logger
	}
	return orphanedLogger
}
