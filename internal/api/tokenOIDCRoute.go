package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/platform"
)

func handleTokenOIDCRoute(creds platform.CredentialProvider, queries *dbqueries.Queries, signingKey []byte) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		subject, err := creds.ValidateOperatorToken(req.Context(), req)
		if errors.Is(err, platform.ErrPrincipalNotAuthorized) {
			return &APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "operator identity could not be verified",
				Remedy:     "ensure your principal is in the operator list and re-authenticate",
			}
		}
		if err != nil {
			return fmt.Errorf("validate operator token: %w", err)
		}

		accessToken, err := tokens.IssueToken(signingKey, tokens.CreateOperatorClaims(subject))
		if err != nil {
			return fmt.Errorf("issue access token: %w", err)
		}

		raw, encoded, err := tokens.GenerateRefreshToken()
		if err != nil {
			return fmt.Errorf("generate refresh token: %w", err)
		}

		id, err := newTokenID()
		if err != nil {
			return fmt.Errorf("generate token id: %w", err)
		}

		ctx := req.Context()
		if err := queries.InsertRefreshToken(ctx, dbqueries.InsertRefreshTokenParams{
			ID:        id,
			TokenHash: tokens.HashRefreshToken(raw),
			Subject:   subject,
			Role:      tokens.RoleOperator,
			ExpiresAt: time.Now().Add(tokens.OperatorRefreshTTL),
		}); err != nil {
			return fmt.Errorf("store refresh token: %w", err)
		}

		resp.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(resp).Encode(struct { //nolint:errcheck
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
		}{
			AccessToken:  accessToken,
			RefreshToken: encoded,
			ExpiresIn:    int(tokens.OperatorTokenTTL.Seconds()),
		})
	}
}

func newTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
