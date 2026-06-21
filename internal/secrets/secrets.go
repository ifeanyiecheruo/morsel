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
	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

//go:embed migrations/*.secrets.txt
var migrationsFS embed.FS

const (
	signingKeyName        = "morsel-signing-key"
	deploySigningKeyName  = "local-deploy-signing-key"
	operatorPrincipalsKey = "operator-principals"
	bootstrapConfigKey    = "morsel-bootstrap-config"
)

// TODO: the secrets manager is introducing dependency complications, we need to revisit its design
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

// DeploySigningKeyExists reports whether a deploy signing key has been provisioned
// without generating one if absent.
func (m *Manager) DeploySigningKeyExists(ctx context.Context) (bool, error) {
	_, err := m.store.Get(ctx, deploySigningKeyName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
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

// TODO: we probably want to manage principals and credientials as a database table of principal ids and hashed credentials
// with a salt stored in the secrets manager
// but lets wait for an overall redesign of the secrets manager before redesigning this part
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

// BootstrapConfig returns the wizard answers stored at bootstrap time.
// Returns nil when the secret is absent — not yet bootstrapped, not an error.
func (m *Manager) BootstrapConfig(ctx context.Context) (map[string]string, error) {
	raw, err := m.store.Get(ctx, bootstrapConfigKey)
	if err != nil {
		if errors.Is(err, platform.ErrSecretNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read bootstrap config: %w", err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse bootstrap config: %w", err)
	}
	return cfg, nil
}

// DeleteBootstrapState removes the bootstrap config and deploy signing key so
// that the next bootstrap run starts from a clean slate. The Morsel signing
// key (for service tokens) is intentionally preserved — issued tokens would
// expire naturally even if the key survived.
func (m *Manager) DeleteBootstrapState(ctx context.Context) error {
	for _, key := range []string{bootstrapConfigKey, deploySigningKeyName} {
		if err := m.store.Delete(ctx, key); err != nil {
			return fmt.Errorf("delete %q: %w", key, err)
		}
	}
	return nil
}

// SetBootstrapConfig persists the wizard answers from bootstrap.
func (m *Manager) SetBootstrapConfig(ctx context.Context, cfg map[string]string) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal bootstrap config: %w", err)
	}
	return m.store.Set(ctx, bootstrapConfigKey, raw)
}

// SetOperatorPrincipals replaces the operator principals list.
func (m *Manager) SetOperatorPrincipals(ctx context.Context, principals []string) error {
	raw, err := json.Marshal(principals)
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
