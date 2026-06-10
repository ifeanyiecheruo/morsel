package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/db"
)

func TestOpenCreatesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := database.Ping(); err != nil {
		t.Errorf("Ping after Open: %v", err)
	}
}

func TestOpenEnablesWALMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var mode string
	if err := database.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}
}

func TestOpenEnablesForeignKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var enabled int
	if err := database.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys = %d, want 1", enabled)
	}
}

func TestOpenFailsOnBadPath(t *testing.T) {
	_, err := db.Open(context.Background(), filepath.Join(t.TempDir(), "no-such-dir", "x", "test.db"))
	if err == nil {
		t.Error("expected error for non-existent directory, got nil")
	}
}
