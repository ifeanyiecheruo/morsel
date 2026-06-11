package api

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

func handleTokenRefreshRoute(queries *dbqueries.Queries, signingKey []byte) func(http.ResponseWriter, *http.Request) error {
	return func(resp http.ResponseWriter, req *http.Request) error {
		var body struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.RefreshToken == "" {
			return &APIError{
				HTTPStatus: http.StatusBadRequest,
				Code:       "invalid_request",
				Message:    "request body must include a non-empty refresh_token field",
				Remedy:     "provide a valid refresh token",
			}
		}

		raw, err := base64.RawURLEncoding.DecodeString(body.RefreshToken)
		if err != nil {
			return &APIError{
				HTTPStatus: http.StatusUnauthorized,
				Code:       "invalid_token",
				Message:    "refresh token is malformed",
				Remedy:     "re-authenticate to obtain a new refresh token",
			}
		}

		ctx := req.Context()
		found, findErr := queries.GetRefreshTokenByHash(ctx, tokens.HashRefreshToken(raw))
		rt, err := validateRefreshToken(found, findErr)
		if err != nil {
			return err
		}

		accessToken, err := tokens.IssueToken(signingKey, tokens.CreateOperatorClaims(rt.Subject))
		if err != nil {
			return fmt.Errorf("issue access token: %w", err)
		}

		newRaw, newEncoded, err := tokens.GenerateRefreshToken()
		if err != nil {
			return fmt.Errorf("generate refresh token: %w", err)
		}
		if _, err := queries.RotateRefreshToken(ctx, dbqueries.RotateRefreshTokenParams{
			ID:        rt.ID,
			TokenHash: tokens.HashRefreshToken(newRaw),
			ExpiresAt: time.Now().Add(tokens.OperatorRefreshTTL),
		}); err != nil {
			return fmt.Errorf("rotate refresh token: %w", err)
		}

		resp.Header().Set("Content-Type", "application/json")
		return json.NewEncoder(resp).Encode(struct { //nolint:errcheck
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			ExpiresIn    int    `json:"expires_in"`
		}{
			AccessToken:  accessToken,
			RefreshToken: newEncoded,
			ExpiresIn:    int(tokens.OperatorTokenTTL.Seconds()),
		})
	}
}

func validateRefreshToken(rt dbqueries.RefreshToken, lookupErr error) (*dbqueries.RefreshToken, error) {
	if errors.Is(lookupErr, sql.ErrNoRows) {
		return nil, &APIError{
			HTTPStatus: http.StatusUnauthorized,
			Code:       "invalid_token",
			Message:    "refresh token not found or already rotated",
			Remedy:     "re-authenticate to obtain a new refresh token",
		}
	}
	if lookupErr != nil {
		return nil, fmt.Errorf("get refresh token: %w", lookupErr)
	}
	if rt.RevokedAt.Valid {
		return nil, &APIError{
			HTTPStatus: http.StatusUnauthorized,
			Code:       "token_revoked",
			Message:    "refresh token has been revoked",
			Remedy:     "re-authenticate to obtain a new refresh token",
		}
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, &APIError{
			HTTPStatus: http.StatusUnauthorized,
			Code:       "token_expired",
			Message:    "refresh token has expired",
			Remedy:     "re-authenticate to obtain a new refresh token",
		}
	}
	if rt.Role != tokens.RoleOperator {
		return nil, &APIError{
			HTTPStatus: http.StatusUnauthorized,
			Code:       "invalid_token",
			Message:    "only operator tokens support refresh",
			Remedy:     "re-authenticate to obtain a new token",
		}
	}
	return &rt, nil
}
