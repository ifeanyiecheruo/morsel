package db_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestMigrateCreatesVersionTable(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	var count int
	if err := database.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&count); err != nil {
		t.Fatalf("query goose_db_version: %v", err)
	}
	if count == 0 {
		t.Error("goose_db_version is empty after migration")
	}
}

func TestMigrateCreatesInitialTables(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{"repos", "apps", "operations"} {
		var name string
		if err := database.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name); err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestRollbackRemovesTables(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Roll back all migrations (one per call).
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("first Rollback: %v", err)
	}
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("second Rollback: %v", err)
	}
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("third Rollback: %v", err)
	}
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("fourth Rollback: %v", err)
	}
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("fifth Rollback: %v", err)
	}
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("sixth Rollback: %v", err)
	}

	for _, table := range []string{"repos", "apps", "operations", "refresh_tokens"} {
		var name string
		err := database.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err == nil {
			t.Errorf("table %q still exists after full rollback", table)
		}
	}
}

func TestMigrateAfterRollbackReapplies(t *testing.T) {
	database := openTestDB(t)
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := db.Rollback(context.Background(), database); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("re-Migrate: %v", err)
	}

	var name string
	if err := database.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='repos'`,
	).Scan(&name); err != nil {
		t.Errorf("repos table missing after re-migrate: %v", err)
	}
}
