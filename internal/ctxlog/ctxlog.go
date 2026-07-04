package ctxlog

import (
	"context"
	"log/slog"
	"os"
)

type contextKey struct{}

var orphanedLogger = slog.Default().With("ctxlog_warning", "no logger in context; using default logger")

// Init creates a leveled text handler writing to stderr, sets it as the slog
// default, stores the logger in ctx, and returns the updated context and the
// level var. Callers adjust the level at runtime via the returned *slog.LevelVar.
func Init(ctx context.Context) (context.Context, *slog.LevelVar) {
	var lv slog.LevelVar // defaults to INFO
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &lv}))
	slog.SetDefault(logger)
	return With(ctx, logger), &lv
}

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
