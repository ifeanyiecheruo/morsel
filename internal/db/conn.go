package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	_ "modernc.org/sqlite"
)

// Open opens a SQLite database at path with WAL mode enabled and the connection
// pool capped to one connection. SQLite serialises writes regardless, but a pool
// cap prevents the driver from opening multiple file handles and racing on locks.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)

	closeOnErr := func() {
		if err := database.Close(); err != nil {
			ctxlog.From(ctx).Error("database close error", "err", err)
		}
	}

	if err := database.Ping(); err != nil {
		closeOnErr()
		return nil, fmt.Errorf("ping: %w", err)
	}

	pragmas := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA foreign_keys=ON`,
		`PRAGMA busy_timeout=5000`,
	}
	for _, pragma := range pragmas {
		if _, err := database.Exec(pragma); err != nil {
			closeOnErr()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}

	return database, nil
}
