// Package secrets manages the lifecycle of all service-level secrets.
// It wraps a platform.SecretStore and provides typed accessors for named
// secrets, plus a Migrate method that runs versioned migrations on startup
// so that key renames and stale-secret cleanup happen automatically.
package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/platform"
)

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

// Migrate runs all pending secret migrations in order. Each migration is
// idempotent, so Migrate is safe to call on every startup.
// A failed migration is fatal — it is returned immediately without running
// subsequent migrations, since later steps may depend on earlier ones.
func (m *Manager) Migrate(ctx context.Context) error {
	logger := ctxlog.From(ctx)
	for _, mig := range migrations {
		if err := mig.run(ctx, m.store); err != nil {
			return fmt.Errorf("secret migration %q: %w", mig.name, err)
		}
		logger.Debug("secret migration ok", "migration", mig.name)
	}
	return nil
}

// --- Migration registry -----------------------------------------------------

type migration struct {
	name string
	run  func(ctx context.Context, store platform.SecretStore) error
}

// migrations is the ordered list of all secret lifecycle migrations.
// Append new entries at the end; never remove or reorder existing ones.
var migrations = []migration{
	// Placeholder: no migrations yet.
	// Example entries when needed:
	//   { name: "rename-foo-v1-to-foo", run: renameSecret("foo-v1", "foo") },
	//   { name: "delete-legacy-bar",    run: deleteSecret("bar-legacy")     },
}

// --- Migration helpers -------------------------------------------------------

// renameSecret returns an idempotent migration that copies src → dst then
// deletes src. If src is absent the migration is a no-op.
func renameSecret(src, dst string) func(context.Context, platform.SecretStore) error {
	return func(ctx context.Context, store platform.SecretStore) error {
		value, err := store.Get(ctx, src)
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := store.Set(ctx, dst, value); err != nil {
			return err
		}
		return store.Delete(ctx, src)
	}
}

// deleteSecret returns an idempotent migration that removes name. If name is
// absent the migration is a no-op.
func deleteSecret(name string) func(context.Context, platform.SecretStore) error {
	return func(ctx context.Context, store platform.SecretStore) error {
		err := store.Delete(ctx, name)
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil
		}
		return err
	}
}
