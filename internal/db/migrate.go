package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies any unapplied versioned SQL migrations from internal/db/migrations/.
// Migrations are identified by filename and applied in lexicographic order.
// Already-applied migrations are skipped; each new migration runs in its own transaction.
func Migrate(database *sql.DB) error {
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT     NOT NULL PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		version := entry.Name()

		var count int
		if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
			return fmt.Errorf("check %s: %w", version, err)
		}
		if count > 0 {
			continue
		}

		sql, err := migrationsFS.ReadFile("migrations/" + version)
		if err != nil {
			return fmt.Errorf("read %s: %w", version, err)
		}

		tx, err := database.Begin()
		if err != nil {
			return fmt.Errorf("begin %s: %w", version, err)
		}
		if _, err := tx.Exec(string(sql)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record %s: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", version, err)
		}
	}

	return nil
}
