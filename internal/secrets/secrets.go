// Package secrets manages the lifecycle of all service-level secrets.
// It wraps a platform.SecretStore and provides typed accessors for named
// secrets, plus a Migrate method that loads and runs versioned migration
// scripts from the embedded migrations/ folder on startup.
package secrets

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/platform"
)

//go:embed migrations/*.secrets.txt
var migrationsFS embed.FS

const (
	signingKeyName       = "morsel-signing-key"
	deploySigningKeyName = "local-deploy-signing-key"
)

// Manager wraps a platform.SecretStore and provides typed accessors for each
// service-level secret. Callers should call Migrate on startup before
// accessing any secret so that key renames and deletions are applied first.
type Manager struct {
	store platform.SecretStore
}

// New creates a Manager backed by store.
func New(store platform.SecretStore) *Manager {
	return &Manager{store: store}
}

// SigningKey returns the HMAC-SHA256 key used to sign and verify Morsel access
// tokens. If the key does not exist it is generated and persisted before returning.
func (m *Manager) SigningKey(ctx context.Context) ([]byte, error) {
	key, err := m.store.Get(ctx, signingKeyName)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, platform.ErrSecretNotFound) {
		return nil, fmt.Errorf("load signing key: %w", err)
	}
	key, err = tokens.GenerateKey()
	if err != nil {
		return nil, err
	}
	if err := m.store.Set(ctx, signingKeyName, key); err != nil {
		return nil, fmt.Errorf("persist signing key: %w", err)
	}
	return key, nil
}

// DeploySigningKey returns the HMAC-SHA256 key used by the local platform to
// sign and verify deploy identity tokens. If absent it is generated and
// persisted before returning. Only used by the local platform implementation;
// cloud platforms use Workload Identity Federation instead.
func (m *Manager) DeploySigningKey(ctx context.Context) ([]byte, error) {
	key, err := m.store.Get(ctx, deploySigningKeyName)
	if err == nil {
		return key, nil
	}
	if !errors.Is(err, platform.ErrSecretNotFound) {
		return nil, fmt.Errorf("load deploy signing key: %w", err)
	}
	key, err = tokens.GenerateKey()
	if err != nil {
		return nil, err
	}
	if err := m.store.Set(ctx, deploySigningKeyName, key); err != nil {
		return nil, fmt.Errorf("persist deploy signing key: %w", err)
	}
	return key, nil
}

// Migrate loads all *.secrets.txt migration scripts from the embedded
// migrations/ folder in filename order and runs each directive. It is
// idempotent and safe to call on every startup.
func (m *Manager) Migrate(ctx context.Context) error {
	logger := ctxlog.From(ctx)
	fsys, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("secrets migrations fs: %w", err)
	}
	migs, err := loadMigrations(fsys)
	if err != nil {
		return fmt.Errorf("load secret migrations: %w", err)
	}
	for _, mig := range migs {
		if err := mig.run(ctx, m.store); err != nil {
			return fmt.Errorf("secret migration %q: %w", mig.name, err)
		}
		logger.Debug("secret migration ok", "migration", mig.name)
	}
	return nil
}
