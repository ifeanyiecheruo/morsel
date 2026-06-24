package local

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

// ── file-backed secret store ─────────────────────────────────────────────────

// localFileSecretStore persists secrets as a JSON map at ~/.morsel/local/secrets.json.
// Values are base64-encoded to handle arbitrary byte payloads.
type localFileSecretStore struct {
	mu   sync.Mutex
	path string
}

func newLocalFileSecretStore() *localFileSecretStore {
	home, _ := os.UserHomeDir()
	return &localFileSecretStore{
		path: filepath.Join(home, ".morsel", "local", "secrets.json"),
	}
}

func (fs *localFileSecretStore) get(_ context.Context, name string) ([]byte, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	data, err := fs.load()
	if err != nil {
		return nil, err
	}
	encoded, ok := data[name]
	if !ok {
		return nil, platform.ErrSecretNotFound
	}
	return base64.StdEncoding.DecodeString(encoded)
}

func (fs *localFileSecretStore) set(_ context.Context, name string, value []byte) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	data, err := fs.load()
	if err != nil && !errors.Is(err, platform.ErrSecretNotFound) {
		return err
	}
	if data == nil {
		data = make(map[string]string)
	}
	data[name] = base64.StdEncoding.EncodeToString(value)
	return fs.save(data)
}

func (fs *localFileSecretStore) delete(_ context.Context, name string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	data, err := fs.load()
	if err != nil {
		return err
	}
	delete(data, name)
	return fs.save(data)
}

func (fs *localFileSecretStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(fs.path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, err
	}
	var data map[string]string
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (fs *localFileSecretStore) save(data map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(fs.path), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(fs.path, raw, 0600)
}

// ── platform.Secrets implementation ─────────────────────────────────────────

const (
	signingKeyName       = "morsel-signing-key"
	deploySigningKeyName = "local-deploy-signing-key"
)

// localSecrets implements platform.Secrets. Keys and bootstrap config are
// stored in the JSON file; operator principal validation queries the DB store.
type localSecrets struct {
	fileStore *localFileSecretStore
	store     *store.Store // nil in CLI contexts that don't need principal validation
}

var _ platform.Secrets = (*localSecrets)(nil)

func (ls *localSecrets) SigningKey(ctx context.Context) ([]byte, error) {
	key, err := ls.fileStore.get(ctx, signingKeyName)
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
	if err := ls.fileStore.set(ctx, signingKeyName, key); err != nil {
		return nil, fmt.Errorf("persist signing key: %w", err)
	}
	return key, nil
}

func (ls *localSecrets) DeploySigningKey(ctx context.Context) ([]byte, error) {
	key, err := ls.fileStore.get(ctx, deploySigningKeyName)
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
	if err := ls.fileStore.set(ctx, deploySigningKeyName, key); err != nil {
		return nil, fmt.Errorf("persist deploy signing key: %w", err)
	}
	return key, nil
}

func (ls *localSecrets) DeploySigningKeyExists(ctx context.Context) (bool, error) {
	_, err := ls.fileStore.get(ctx, deploySigningKeyName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (ls *localSecrets) AmbientToken(_ context.Context) (string, error) { return "", nil }

func (ls *localSecrets) DeployToken(ctx context.Context) (string, error) {
	key, err := ls.DeploySigningKey(ctx)
	if err != nil {
		return "", fmt.Errorf("deploy signing key: %w", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"repository": "localhost/local",
	})
	return token.SignedString(key)
}

func (ls *localSecrets) ValidateDeployToken(ctx context.Context, tokenStr string) (string, error) {
	key, err := ls.DeploySigningKey(ctx)
	if err != nil {
		return "", fmt.Errorf("deploy signing key: %w", err)
	}
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	})
	if err != nil || !token.Valid {
		return "", fmt.Errorf("invalid deploy token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}
	repo, ok := claims["repository"].(string)
	if !ok || repo == "" {
		return "", fmt.Errorf("token missing repository claim")
	}
	return repo, nil
}

func (ls *localSecrets) ValidateOperatorCredential(ctx context.Context, username, _ string) (string, error) {
	if username == "" {
		return "", platform.ErrPrincipalNotAuthorized
	}
	if ls.store == nil {
		return "", platform.ErrPrincipalNotAuthorized
	}
	log := ctxlog.From(ctx)
	log.Info("validating operator credential", "username", username)
	exists, err := ls.store.PrincipalExists(ctx, username)
	if err != nil {
		return "", fmt.Errorf("validate operator credential: %w", err)
	}
	if !exists {
		return "", platform.ErrPrincipalNotAuthorized
	}
	return username, nil
}

func (ls *localSecrets) Migrate(ctx context.Context) error {
	return runFileMigrations(ctx, ls.fileStore)
}
