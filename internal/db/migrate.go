package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies all pending migrations. Safe to call on every startup.
func Migrate(ctx context.Context, database *sql.DB) error {
	provider, err := createMigrationsProvider(ctx, database)
	if err != nil {
		return err
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

// Rollback rolls back the most recently applied migration.
func Rollback(ctx context.Context, database *sql.DB) error {
	provider, err := createMigrationsProvider(ctx, database)
	if err != nil {
		return err
	}
	if _, err := provider.Down(ctx); err != nil {
		return fmt.Errorf("rollback: %w", err)
	}
	return nil
}

func createMigrationsProvider(ctx context.Context, database *sql.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("migrations fs: %w", err)
	}
	return goose.NewProvider(goose.DialectSQLite3, database, fsys,
		goose.WithLogger(gooseSlogLogger{logger: ctxlog.From(ctx)}),
	)
}

// gooseSlogLogger bridges goose's Logger interface to slog.
type gooseSlogLogger struct {
	logger *slog.Logger
}

func (gl gooseSlogLogger) Printf(format string, args ...any) {
	gl.logger.Info(strings.TrimRight(fmt.Sprintf(format, args...), "\n"))
}

func (gl gooseSlogLogger) Fatalf(format string, args ...any) {
	gl.logger.Error(strings.TrimRight(fmt.Sprintf(format, args...), "\n"))
}
