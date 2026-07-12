package local

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// SecretStore is the low-level storage backend for raw secret bytes.
// The production implementation writes to Kubernetes Opaque Secrets;
// tests may supply an in-memory implementation instead.
type SecretStore interface {
	Get(ctx context.Context, name string) ([]byte, error)
	Set(ctx context.Context, name string, value []byte) error
	Delete(ctx context.Context, name string) error
}

const (
	signingKeyName       = "morsel-signing-keys"
	deploySigningKeyName = "local-deploy-signing-keys"
	bootstrapTokenName   = "morsel-bootstrap-token"
)

// ── kube-backed secret store ─────────────────────────────────────────────────

// kubeSecretStore persists secrets as Kubernetes Opaque Secrets in the control-plane namespace.
// The kube client is built lazily on first access using the in-cluster service-account config.
type kubeSecretStore struct {
	once  sync.Once
	kc    *kube.Client
	kcErr error
	ns    string
}

func newKubeSecretStore(ns string) *kubeSecretStore {
	return &kubeSecretStore{ns: ns}
}

func (ks *kubeSecretStore) client() (*kube.Client, error) {
	ks.once.Do(func() { ks.kc, ks.kcErr = kube.NewInCluster() })
	return ks.kc, ks.kcErr
}

func (ks *kubeSecretStore) Get(ctx context.Context, name string) ([]byte, error) {
	kc, err := ks.client()
	if err != nil {
		return nil, fmt.Errorf("secret store requires in-cluster kubernetes access: %w", err)
	}
	data, err := kc.GetOpaqueSecret(ctx, ks.ns, name)
	if errors.Is(err, kube.ErrSecretNotFound) {
		return nil, platform.ErrSecretNotFound
	}
	return data, err
}

func (ks *kubeSecretStore) Set(ctx context.Context, name string, value []byte) error {
	kc, err := ks.client()
	if err != nil {
		return fmt.Errorf("secret store requires in-cluster kubernetes access: %w", err)
	}
	return kc.SetOpaqueSecret(ctx, ks.ns, name, value)
}

func (ks *kubeSecretStore) Delete(ctx context.Context, name string) error {
	kc, err := ks.client()
	if err != nil {
		return fmt.Errorf("secret store requires in-cluster kubernetes access: %w", err)
	}
	return kc.DeleteOpaqueSecret(ctx, ks.ns, name)
}

// ── platform.Secrets implementation ─────────────────────────────────────────

// localSecrets implements platform.Secrets (key management only).
type localSecrets struct {
	store SecretStore
}

var _ platform.Secrets = (*localSecrets)(nil)

// getKeyArray reads a stored key array. Returns nil if the key is absent.
func (ls *localSecrets) getKeyArray(ctx context.Context, name string) ([][]byte, error) {
	raw, err := ls.store.Get(ctx, name)
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
	return ls.store.Set(ctx, name, raw)
}

func (ls *localSecrets) appendNewKey(ctx context.Context, name string, base [][]byte) ([][]byte, error) {
	key, err := generateKey()
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
	err := ls.store.Delete(ctx, signingKeyName)
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
	err := ls.store.Delete(ctx, deploySigningKeyName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return nil
	}
	return err
}

// ── bootstrap token ──────────────────────────────────────────────────────────

func (ls *localSecrets) GetBootstrapToken(ctx context.Context) (string, error) {
	raw, err := ls.store.Get(ctx, bootstrapTokenName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return "", platform.ErrSecretNotFound
	}
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (ls *localSecrets) DeleteBootstrapToken(ctx context.Context) error {
	err := ls.store.Delete(ctx, bootstrapTokenName)
	if errors.Is(err, platform.ErrSecretNotFound) {
		return nil
	}
	return err
}

func (ls *localSecrets) Migrate(ctx context.Context) error {
	if _, err := ls.EnsureSigningKey(ctx); err != nil {
		return fmt.Errorf("ensure signing key: %w", err)
	}
	if _, err := ls.EnsureDeploySigningKey(ctx); err != nil {
		return fmt.Errorf("ensure deploy signing key: %w", err)
	}
	return nil
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

func generateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	return key, nil
}
