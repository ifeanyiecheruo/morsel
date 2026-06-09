package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/db"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestMigrateCreatesSchemaTable(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count == 0 {
		t.Error("schema_migrations is empty after migration")
	}
}

func TestMigrateCreatesInitialTables(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{"repos", "apps", "operations"} {
		var name string
		err := database.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrateRecordsVersions(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	rows, err := database.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query versions: %v", err)
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			t.Fatalf("scan: %v", err)
		}
		versions = append(versions, version)
	}
	if len(versions) == 0 {
		t.Error("no versions recorded in schema_migrations")
	}
	if versions[0] != "001_initial.sql" {
		t.Errorf("versions[0] = %q, want 001_initial.sql", versions[0])
	}
}
