// Package secrets manages the lifecycle of all service-level secrets.
// It wraps a platform.SecretStore and provides typed accessors for named
// secrets, plus a Migrate method that loads and runs versioned migration
// scripts from the embedded migrations/ folder on startup.
package secrets

import (
	"context"
	"embed"
	"encoding/json"
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
	signingKeyName        = "morsel-signing-key"
	deploySigningKeyName  = "local-deploy-signing-key"
	operatorPrincipalsKey = "operator-principals"
)

// Call Migrate on startup before accessing any secret so that key renames and deletions are applied first.
type Manager struct {
	store platform.SecretStore
}

func New(store platform.SecretStore) *Manager {
	return &Manager{store: store}
}

// Generated and persisted on first call if absent.
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

// Generated and persisted on first call if absent.
// Only used by the local platform; cloud platforms use Workload Identity Federation.
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

// OperatorPrincipals returns the list of authorised operator emails.
// Returns nil when the secret is absent — an empty list, not an error.
func (m *Manager) OperatorPrincipals(ctx context.Context) ([]string, error) {
	raw, err := m.store.Get(ctx, operatorPrincipalsKey)
	if err != nil {
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read operator principals: %w", err)
	}
	var principals []string
	if err := json.Unmarshal(raw, &principals); err != nil {
		return nil, fmt.Errorf("parse operator principals: %w", err)
	}
	return principals, nil
}

// TODO: We need a good pattern for getting, setting secrets instead of special verbs like seed
// SeedOperatorPrincipal writes [principal] as the operator principals list if
// the list is currently absent. Idempotent once any principals exist.
func (m *Manager) SeedOperatorPrincipal(ctx context.Context, principal string) error {
	existing, err := m.OperatorPrincipals(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	raw, err := json.Marshal([]string{principal})
	if err != nil {
		return fmt.Errorf("marshal operator principals: %w", err)
	}
	return m.store.Set(ctx, operatorPrincipalsKey, raw)
}

// Idempotent; safe to call on every startup.
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
