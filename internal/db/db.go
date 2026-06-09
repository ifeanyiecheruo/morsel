package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens a SQLite database at path with WAL mode enabled and the connection
// pool capped to one connection. SQLite serialises writes regardless, but a pool
// cap prevents the driver from opening multiple file handles and racing on locks.
func Open(path string) (*sql.DB, error) {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)

	if err := database.Ping(); err != nil {
		database.Close()
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
			database.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}

	return database, nil
}
