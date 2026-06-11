package secrets

import (
	"context"
	"sync"
	"testing"

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
