package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/platform"
)

func (h *Handler) TokenDeploy(ctx context.Context, req *oas.TokenDeployReq) (oas.TokenDeployRes, error) {
	if req.Token == "" {
		return &oas.TokenDeployBadRequest{Error: oas.ErrorDetail{
			Code:    "invalid_request",
			Message: "request body must include a non-empty token field",
			Remedy:  "provide a valid deploy identity token obtained from your CI platform",
		}}, nil
	}

	slug, err := h.plat.Credentials().ValidateDeployToken(ctx, req.Token)
	if err != nil {
		return &oas.TokenDeployUnauthorized{Error: oas.ErrorDetail{
			Code:    "invalid_token",
			Message: "deploy identity token is invalid or expired",
			Remedy:  "re-run morsel app deploy to obtain a fresh deploy identity token",
		}}, nil
	}

	accessToken, err := tokens.IssueToken(h.signingKey, tokens.CreateDeployClaims(slug))
	if err != nil {
		return nil, fmt.Errorf("issue deploy token: %w", err)
	}

	return &oas.TokenResponse{AccessToken: accessToken, ExpiresIn: 600}, nil
}

func (h *Handler) TokenOIDC(ctx context.Context, req *oas.TokenOIDCReq) (oas.TokenOIDCRes, error) {
	subject, err := h.plat.Credentials().ValidateOperatorToken(ctx, req.Credential)
	if errors.Is(err, platform.ErrPrincipalNotAuthorized) {
		return &oas.TokenOIDCUnauthorized{Error: oas.ErrorDetail{
			Code:    "invalid_token",
			Message: "operator identity could not be verified",
			Remedy:  "ensure your principal is in the operator list and re-authenticate",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("validate operator token: %w", err)
	}

	accessToken, err := tokens.IssueToken(h.signingKey, tokens.CreateOperatorClaims(subject))
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	raw, encoded, err := tokens.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	id, err := newTokenID()
	if err != nil {
		return nil, fmt.Errorf("generate token id: %w", err)
	}

	if err := h.queries.InsertRefreshToken(ctx, dbqueries.InsertRefreshTokenParams{
		ID:        id,
		TokenHash: tokens.HashRefreshToken(raw),
		Subject:   subject,
		Role:      tokens.RoleOperator,
		ExpiresAt: time.Now().Add(tokens.OperatorRefreshTTL),
	}); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &oas.TokenPairResponse{
		AccessToken:  accessToken,
		RefreshToken: encoded,
		ExpiresIn:    int(tokens.OperatorTokenTTL.Seconds()),
	}, nil
}

func (h *Handler) TokenRefresh(ctx context.Context, req *oas.TokenRefreshReq) (oas.TokenRefreshRes, error) {
	if req.RefreshToken == "" {
		return &oas.TokenRefreshBadRequest{Error: oas.ErrorDetail{
			Code:    "invalid_request",
			Message: "request body must include a non-empty refresh_token field",
			Remedy:  "provide a valid refresh token",
		}}, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(req.RefreshToken)
	if err != nil {
		return &oas.TokenRefreshUnauthorized{Error: oas.ErrorDetail{
			Code:    "invalid_token",
			Message: "refresh token is malformed",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}

	found, findErr := h.queries.GetRefreshTokenByHash(ctx, tokens.HashRefreshToken(raw))
	rt, typedResp, err := validateRefreshToken(found, findErr)
	if err != nil {
		return nil, err
	}
	if typedResp != nil {
		return typedResp, nil
	}

	accessToken, err := tokens.IssueToken(h.signingKey, tokens.CreateOperatorClaims(rt.Subject))
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}

	newRaw, newEncoded, err := tokens.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}
	if _, err := h.queries.RotateRefreshToken(ctx, dbqueries.RotateRefreshTokenParams{
		ID:        rt.ID,
		TokenHash: tokens.HashRefreshToken(newRaw),
		ExpiresAt: time.Now().Add(tokens.OperatorRefreshTTL),
	}); err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	return &oas.TokenPairResponse{
		AccessToken:  accessToken,
		RefreshToken: newEncoded,
		ExpiresIn:    int(tokens.OperatorTokenTTL.Seconds()),
	}, nil
}

// validateRefreshToken checks db-level validity of a refresh token row.
// Returns the row on success, a typed TokenRefreshRes on a 4xx condition,
// or an error for infrastructure failures.
func validateRefreshToken(rt dbqueries.RefreshToken, lookupErr error) (*dbqueries.RefreshToken, oas.TokenRefreshRes, error) {
	if errors.Is(lookupErr, sql.ErrNoRows) {
		return nil, &oas.TokenRefreshUnauthorized{Error: oas.ErrorDetail{
			Code:    "invalid_token",
			Message: "refresh token not found or already rotated",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}
	if lookupErr != nil {
		return nil, nil, fmt.Errorf("get refresh token: %w", lookupErr)
	}
	if rt.RevokedAt.Valid {
		return nil, &oas.TokenRefreshUnauthorized{Error: oas.ErrorDetail{
			Code:    "token_revoked",
			Message: "refresh token has been revoked",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, &oas.TokenRefreshUnauthorized{Error: oas.ErrorDetail{
			Code:    "token_expired",
			Message: "refresh token has expired",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}
	if rt.Role != tokens.RoleOperator {
		return nil, &oas.TokenRefreshUnauthorized{Error: oas.ErrorDetail{
			Code:    "invalid_token",
			Message: "only operator tokens support refresh",
			Remedy:  "re-authenticate to obtain a new token",
		}}, nil
	}
	return &rt, nil, nil
}

func newTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
