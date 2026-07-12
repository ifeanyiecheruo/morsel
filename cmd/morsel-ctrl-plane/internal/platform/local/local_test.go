package local_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform/local"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

var ctx = ctxlog.With(context.Background(), slog.Default())

// memSecretStore is a thread-safe in-memory SecretStore for tests.
type memSecretStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemSecretStore() *memSecretStore {
	return &memSecretStore{data: make(map[string][]byte)}
}

func (m *memSecretStore) Get(_ context.Context, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[name]
	if !ok {
		return nil, platform.ErrSecretNotFound
	}
	return v, nil
}

func (m *memSecretStore) Set(_ context.Context, name string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[name] = value
	return nil
}

func (m *memSecretStore) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, name)
	return nil
}

func TestGetAmbientTokenReturnsEmpty(t *testing.T) {
	plat := local.NewWithSecretStore(nil, newMemSecretStore(), 443)
	token, err := plat.Tokens().GetAmbientToken(ctx)
	if err != nil {
		t.Fatalf("GetAmbientToken: unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty string", token)
	}
}

func TestVerifyDeployTokenRejectsInvalidToken(t *testing.T) {
	plat := platWithSecrets(t)
	_, err := plat.Tokens().VerifyDeployToken(ctx, "not-a-valid-token")
	if err == nil {
		t.Error("VerifyDeployToken: expected error for invalid token, got nil")
	}
}

func TestDeployTokenRoundTrip(t *testing.T) {
	plat := platWithSecrets(t)

	token, err := plat.Tokens().CreateDeployToken(ctx, "localhost/test-repo")
	if err != nil {
		t.Fatalf("CreateDeployToken: %v", err)
	}
	slug, err := plat.Tokens().VerifyDeployToken(ctx, token)
	if err != nil {
		t.Fatalf("VerifyDeployToken: %v", err)
	}
	if slug == "" {
		t.Error("VerifyDeployToken: returned empty slug")
	}
}

// platWithSecrets returns a LocalPlatform (no DB store) backed by a fresh in-memory SecretStore.
func platWithSecrets(t *testing.T) *local.LocalPlatform {
	t.Helper()
	return local.NewWithSecretStore(nil, newMemSecretStore(), 443)
}

func TestDNSCreateRecordIsNoop(t *testing.T) {
	plat := local.NewWithSecretStore(nil, newMemSecretStore(), 443)
	if err := plat.DNS().CreateRecord(ctx, "zone", "name", "A", "1.2.3.4", 60); err != nil {
		t.Errorf("CreateRecord: unexpected error: %v", err)
	}
}

func TestDNSDeleteRecordIsNoop(t *testing.T) {
	plat := local.NewWithSecretStore(nil, newMemSecretStore(), 443)
	if err := plat.DNS().DeleteRecord(ctx, "zone", "name", "A"); err != nil {
		t.Errorf("DeleteRecord: unexpected error: %v", err)
	}
}

func TestDNSRecordExistsReturnsFalse(t *testing.T) {
	plat := local.NewWithSecretStore(nil, newMemSecretStore(), 443)
	exists, err := plat.DNS().RecordExists(ctx, "zone", "name", "A")
	if err != nil {
		t.Fatalf("RecordExists: unexpected error: %v", err)
	}
	if exists {
		t.Error("RecordExists = true, want false")
	}
}

func TestPricesFetchedAtIsSet(t *testing.T) {
	plat := local.NewWithSecretStore(nil, newMemSecretStore(), 443)
	prices, err := plat.Pricing().Prices(ctx)
	if err != nil {
		t.Fatalf("Prices: unexpected error: %v", err)
	}
	if prices.FetchedAt.IsZero() {
		t.Error("Prices.FetchedAt is zero, want non-zero")
	}
}

func TestBlobsGetNotImplemented(t *testing.T) {
	plat := local.NewWithSecretStore(nil, newMemSecretStore(), 443)
	if _, err := plat.Blobs().Get(ctx, "bucket", "key"); !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("Blobs.Get: err = %v, want ErrNotImplemented", err)
	}
}
