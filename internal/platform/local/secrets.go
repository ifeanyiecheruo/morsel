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
	return &localFileSecretStore{
		path: filepath.Join(localDataDir(), "secrets.json"),
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
	signingKeyName       = "morsel-signing-keys"
	deploySigningKeyName = "local-deploy-signing-keys"
)

// localSecrets implements platform.Secrets (key management only).
type localSecrets struct {
	fileStore *localFileSecretStore
}

var _ platform.Secrets = (*localSecrets)(nil)

// getKeyArray reads a stored key array. Returns nil if the key is absent.
func (ls *localSecrets) getKeyArray(ctx context.Context, name string) ([][]byte, error) {
	raw, err := ls.fileStore.get(ctx, name)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys [][]byte
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("parse key array %q: %w", name, err)
	}
	return keys, nil
}

func (ls *localSecrets) setKeyArray(ctx context.Context, name string, keys [][]byte) error {
	raw, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("marshal key array %q: %w", name, err)
	}
	return ls.fileStore.set(ctx, name, raw)
}

// appendNewKey generates a fresh key, appends it to base, persists, and returns the result.
func (ls *localSecrets) appendNewKey(ctx context.Context, name string, base [][]byte) ([][]byte, error) {
	key, err := tokens.GenerateKey()
	if err != nil {
		return nil, err
	}
	updated := append(base, key)
	if err := ls.setKeyArray(ctx, name, updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// ── signing key ──────────────────────────────────────────────────────────────

func (ls *localSecrets) GetSigningKeys(ctx context.Context) ([][]byte, error) {
	return ls.getKeyArray(ctx, signingKeyName)
}

func (ls *localSecrets) EnsureSigningKey(ctx context.Context) ([][]byte, error) {
	keys, err := ls.getKeyArray(ctx, signingKeyName)
	if err != nil {
		return nil, err
	}
	for len(keys) < 2 {
		if keys, err = ls.appendNewKey(ctx, signingKeyName, keys); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (ls *localSecrets) RotateSigningKey(ctx context.Context) ([][]byte, error) {
	keys, err := ls.getKeyArray(ctx, signingKeyName)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return ls.appendNewKey(ctx, signingKeyName, nil)
	}
	// Drop oldest (index 0), append new — array length stays constant.
	tail := make([][]byte, len(keys)-1)
	copy(tail, keys[1:])
	return ls.appendNewKey(ctx, signingKeyName, tail)
}

func (ls *localSecrets) DeleteSigningKey(ctx context.Context) error {
	err := ls.fileStore.delete(ctx, signingKeyName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return nil
	}
	return err
}

// ── deploy signing key ───────────────────────────────────────────────────────

func (ls *localSecrets) GetDeploySigningKeys(ctx context.Context) ([][]byte, error) {
	return ls.getKeyArray(ctx, deploySigningKeyName)
}

func (ls *localSecrets) EnsureDeploySigningKey(ctx context.Context) ([][]byte, error) {
	keys, err := ls.getKeyArray(ctx, deploySigningKeyName)
	if err != nil {
		return nil, err
	}
	for len(keys) < 2 {
		if keys, err = ls.appendNewKey(ctx, deploySigningKeyName, keys); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

func (ls *localSecrets) RotateDeploySigningKey(ctx context.Context) ([][]byte, error) {
	keys, err := ls.getKeyArray(ctx, deploySigningKeyName)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return ls.appendNewKey(ctx, deploySigningKeyName, nil)
	}
	tail := make([][]byte, len(keys)-1)
	copy(tail, keys[1:])
	return ls.appendNewKey(ctx, deploySigningKeyName, tail)
}

func (ls *localSecrets) DeleteDeploySigningKey(ctx context.Context) error {
	err := ls.fileStore.delete(ctx, deploySigningKeyName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return nil
	}
	return err
}

func (ls *localSecrets) Migrate(ctx context.Context) error {
	return runFileMigrations(ctx, ls.fileStore)
}

// ── platform.Tokens implementation ──────────────────────────────────────────

// localTokens implements platform.Tokens. It reads key material from localSecrets
// and performs principal lookups via the DB store.
type localTokens struct {
	secrets *localSecrets
	store   *store.Store // nil in CLI contexts that don't need principal validation
}

var _ platform.Tokens = (*localTokens)(nil)

func (lt *localTokens) GetAmbientToken(_ context.Context) (string, error) { return "", nil }

func (lt *localTokens) CreateDeployToken(ctx context.Context, repository string) (string, error) {
	keys, err := lt.secrets.EnsureDeploySigningKey(ctx)
	if err != nil {
		return "", fmt.Errorf("ensure deploy signing key: %w", err)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("deploy signing key not provisioned")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"repository": repository,
	})
	return tok.SignedString(keys[0])
}

func (lt *localTokens) VerifyDeployToken(ctx context.Context, tokenStr string) (string, error) {
	keys, err := lt.secrets.GetDeploySigningKeys(ctx)
	if err != nil {
		return "", fmt.Errorf("get deploy signing keys: %w", err)
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("deploy signing key not provisioned")
	}
	// Try every key — during rotation multiple keys may be valid.
	var lastErr error
	for _, key := range keys {
		tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return key, nil
		})
		if err != nil || !tok.Valid {
			lastErr = err
			continue
		}
		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			lastErr = fmt.Errorf("invalid token claims")
			continue
		}
		repo, ok := claims["repository"].(string)
		if !ok || repo == "" {
			lastErr = fmt.Errorf("token missing repository claim")
			continue
		}
		return repo, nil
	}
	if lastErr != nil {
		return "", fmt.Errorf("invalid deploy token: %w", lastErr)
	}
	return "", fmt.Errorf("invalid deploy token")
}

func (lt *localTokens) ValidateOperatorCredential(ctx context.Context, username, _ string) (string, error) {
	if username == "" {
		return "", platform.ErrPrincipalNotAuthorized
	}
	if lt.store == nil {
		return "", platform.ErrPrincipalNotAuthorized
	}
	ctxlog.From(ctx).Info("validating operator credential", "username", username)
	exists, err := lt.store.PrincipalExists(ctx, username)
	if err != nil {
		return "", fmt.Errorf("validate operator credential: %w", err)
	}
	if !exists {
		return "", platform.ErrPrincipalNotAuthorized
	}
	return username, nil
}
