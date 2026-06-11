package local

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ifeanyiecheruo/morsel/internal/secrets"
	"github.com/ifeanyiecheruo/morsel/platform"
)

type localCredentialProvider struct {
	secretMgr *secrets.Manager
}

// AmbientToken returns "" — no ambient cloud identity is needed locally.
func (lc *localCredentialProvider) AmbientToken(_ context.Context) (string, error) { return "", nil }

// DeployToken generates a signed JWT for the current repo. The signing key is
// loaded (or generated on first use) via the secrets manager.
func (lc *localCredentialProvider) DeployToken(ctx context.Context) (string, error) {
	key, err := lc.secretMgr.DeploySigningKey(ctx)
	if err != nil {
		return "", fmt.Errorf("deploy signing key: %w", err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"repository": "localhost/local",
	})
	return token.SignedString(key)
}

func (lc *localCredentialProvider) ValidateOperatorToken(ctx context.Context, r *http.Request) (string, error) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		return "", platform.ErrPrincipalNotAuthorized
	}

	principals, err := lc.secretMgr.OperatorPrincipals(ctx)
	if err != nil {
		return "", fmt.Errorf("validate operator token: %w", err)
	}

	for _, p := range principals {
		if p == body.Email {
			return body.Email, nil
		}
	}
	return "", platform.ErrPrincipalNotAuthorized
}

// ValidateDeployToken validates a local deploy JWT and returns the repo slug.
// The signing key is loaded (or generated on first use) via the secrets manager.
func (lc *localCredentialProvider) ValidateDeployToken(ctx context.Context, tokenStr string) (string, error) {
	key, err := lc.secretMgr.DeploySigningKey(ctx)
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
