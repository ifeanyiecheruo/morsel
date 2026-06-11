package secrets

import (
	"context"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/ifeanyiecheruo/morsel/platform"
)

// memStore is an in-memory platform.SecretStore used in tests.
type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: make(map[string][]byte)} }

func (m *memStore) Get(_ context.Context, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[name]
	if !ok {
		return nil, platform.ErrSecretNotFound
	}
	return v, nil
}

func (m *memStore) Set(_ context.Context, name string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[name] = value
	return nil
}

func (m *memStore) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, name)
	return nil
}

func TestSigningKeyGeneratedWhenAbsent(t *testing.T) {
	mgr := New(newMemStore())
	key, err := mgr.SigningKey(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("expected a non-empty key")
	}
}

func TestSigningKeyStableAcrossCalls(t *testing.T) {
	mgr := New(newMemStore())
	first, err := mgr.SigningKey(context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := mgr.SigningKey(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(first) != string(second) {
		t.Error("signing key changed between calls — should be stable once generated")
	}
}

func TestDeploySigningKeyGeneratedWhenAbsent(t *testing.T) {
	mgr := New(newMemStore())
	key, err := mgr.DeploySigningKey(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(key) == 0 {
		t.Fatal("expected a non-empty key")
	}
}

func TestDeploySigningKeyStableAcrossCalls(t *testing.T) {
	mgr := New(newMemStore())
	first, err := mgr.DeploySigningKey(context.Background())
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := mgr.DeploySigningKey(context.Background())
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if string(first) != string(second) {
		t.Error("deploy signing key changed between calls — should be stable once generated")
	}
}

func TestSigningKeyAndDeploySigningKeyAreDistinct(t *testing.T) {
	mgr := New(newMemStore())
	signingKey, err := mgr.SigningKey(context.Background())
	if err != nil {
		t.Fatalf("SigningKey: %v", err)
	}
	deployKey, err := mgr.DeploySigningKey(context.Background())
	if err != nil {
		t.Fatalf("DeploySigningKey: %v", err)
	}
	if string(signingKey) == string(deployKey) {
		t.Error("signing key and deploy signing key must be distinct")
	}
}

func TestMigrateIsNoOpWithEmptyList(t *testing.T) {
	mgr := New(newMemStore())
	if err := mgr.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
}

func TestRenameSecretMigration(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()
	_ = store.Set(ctx, "old-key", []byte("secret-value"))

	mig := renameSecret("old-key", "new-key")
	if err := mig(ctx, store); err != nil {
		t.Fatalf("migration error: %v", err)
	}

	if _, err := store.Get(ctx, "old-key"); err == nil {
		t.Error("old-key should have been deleted")
	}
	val, err := store.Get(ctx, "new-key")
	if err != nil {
		t.Fatalf("new-key not found: %v", err)
	}
	if string(val) != "secret-value" {
		t.Errorf("new-key value = %q, want %q", val, "secret-value")
	}
}

func TestRenameSecretMigrationIsNoOpWhenSourceAbsent(t *testing.T) {
	store := newMemStore()
	mig := renameSecret("old-key", "new-key")
	if err := mig(context.Background(), store); err != nil {
		t.Fatalf("migration should be a no-op but got error: %v", err)
	}
}

func TestDeleteSecretMigration(t *testing.T) {
	store := newMemStore()
	ctx := context.Background()
	_ = store.Set(ctx, "stale-key", []byte("garbage"))

	mig := deleteSecret("stale-key")
	if err := mig(ctx, store); err != nil {
		t.Fatalf("migration error: %v", err)
	}
	if _, err := store.Get(ctx, "stale-key"); err == nil {
		t.Error("stale-key should have been deleted")
	}
}

func TestDeleteSecretMigrationIsNoOpWhenAbsent(t *testing.T) {
	store := newMemStore()
	mig := deleteSecret("nonexistent")
	if err := mig(context.Background(), store); err != nil {
		t.Fatalf("migration should be a no-op but got error: %v", err)
	}
}

// --- parseMigrationScript tests ---------------------------------------------

func TestParseMigrationScriptRename(t *testing.T) {
	migs, err := parseMigrationScript("test.secrets.txt", []byte(`rename "old-key" "new-key"`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(migs) != 1 {
		t.Fatalf("want 1 migration, got %d", len(migs))
	}
	if !strings.Contains(migs[0].name, "old-key") || !strings.Contains(migs[0].name, "new-key") {
		t.Errorf("migration name %q does not reference both keys", migs[0].name)
	}
}

func TestParseMigrationScriptDelete(t *testing.T) {
	migs, err := parseMigrationScript("test.secrets.txt", []byte(`delete "stale-key"`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(migs) != 1 {
		t.Fatalf("want 1 migration, got %d", len(migs))
	}
	if !strings.Contains(migs[0].name, "stale-key") {
		t.Errorf("migration name %q does not reference key", migs[0].name)
	}
}

func TestParseMigrationScriptIgnoresCommentsAndBlanks(t *testing.T) {
	script := `# this is a comment

# another comment
delete "x"
`
	migs, err := parseMigrationScript("test.secrets.txt", []byte(script))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(migs) != 1 {
		t.Fatalf("want 1 migration, got %d", len(migs))
	}
}

func TestParseMigrationScriptUnknownDirective(t *testing.T) {
	_, err := parseMigrationScript("test.secrets.txt", []byte(`frobnicate "foo"`))
	if err == nil {
		t.Error("expected error for unknown directive")
	}
}

func TestParseMigrationScriptRenameWrongArgCount(t *testing.T) {
	_, err := parseMigrationScript("test.secrets.txt", []byte(`rename "only-one"`))
	if err == nil {
		t.Error("expected error for rename with one argument")
	}
}

func TestParseMigrationScriptDeleteWrongArgCount(t *testing.T) {
	_, err := parseMigrationScript("test.secrets.txt", []byte(`delete "a" "b"`))
	if err == nil {
		t.Error("expected error for delete with two arguments")
	}
}

// --- loadMigrations tests ----------------------------------------------------

func TestLoadMigrationsRunsInFilenameOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"002_second.secrets.txt": {Data: []byte(`delete "b"`)},
		"001_first.secrets.txt":  {Data: []byte(`delete "a"`)},
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migs) != 2 {
		t.Fatalf("want 2 migrations, got %d", len(migs))
	}
	if !strings.Contains(migs[0].name, "001") {
		t.Errorf("first migration should come from 001_first, got %q", migs[0].name)
	}
	if !strings.Contains(migs[1].name, "002") {
		t.Errorf("second migration should come from 002_second, got %q", migs[1].name)
	}
}

func TestLoadMigrationsSkipsNonSecretFiles(t *testing.T) {
	fsys := fstest.MapFS{
		"001_real.secrets.txt": {Data: []byte(`delete "x"`)},
		"README.md":            {Data: []byte(`# ignored`)},
		"notes.txt":            {Data: []byte(`ignored`)},
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migs) != 1 {
		t.Fatalf("want 1 migration (only .secrets.txt), got %d", len(migs))
	}
}

func TestLoadMigrationsEmptyDirReturnsNone(t *testing.T) {
	fsys := fstest.MapFS{}
	migs, err := loadMigrations(fsys)
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migs) != 0 {
		t.Errorf("want 0 migrations, got %d", len(migs))
	}
}
