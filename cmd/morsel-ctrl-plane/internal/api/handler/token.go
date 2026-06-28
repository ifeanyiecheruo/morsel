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

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

const operatorRefreshTTL = 90 * 24 * time.Hour

func (h *Handler) TokenDeploy(ctx context.Context, req *server.TokenDeployReq) (server.TokenDeployRes, error) {
	if req.Token == "" {
		return &server.TokenDeployBadRequest{Error: server.ErrorDetail{
			Code:    "invalid_request",
			Message: "request body must include a non-empty token field",
			Remedy:  "provide a valid deploy identity token obtained from your CI platform",
		}}, nil
	}

	slug, err := h.plat.Tokens().VerifyDeployToken(ctx, req.Token)
	if err != nil {
		return &server.TokenDeployUnauthorized{Error: server.ErrorDetail{
			Code:    "invalid_token",
			Message: "deploy identity token is invalid or expired",
			Remedy:  "re-run morsel app deploy to obtain a fresh deploy identity token",
		}}, nil
	}

	accessToken, err := tokens.IssueToken(h.signingKey, tokens.CreateDeployClaims(slug))
	if err != nil {
		return nil, fmt.Errorf("issue deploy token: %w", err)
	}

	return &server.TokenDeployOK{AccessToken: accessToken, ExpiresIn: 600}, nil
}

func (h *Handler) TokenOIDC(ctx context.Context, req *server.TokenOIDCReq) (server.TokenOIDCRes, error) {
	log := ctxlog.From(ctx)
	log.Info("operator login attempt", "username", req.Username)
	subject, err := h.plat.Tokens().ValidateOperatorCredential(ctx, req.Username, req.Password)
	if errors.Is(err, platform.ErrPrincipalNotAuthorized) {
		log.Warn("operator login rejected", "username", req.Username, "reason", err)
		return &server.ErrorResponse{Error: server.ErrorDetail{
			Code:    "invalid_token",
			Message: "operator identity could not be verified",
			Remedy:  "ensure your principal is in the operator list and re-authenticate",
		}}, nil
	}
	if err != nil {
		log.Error("operator login error", "username", req.Username, "err", err)
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

	if err := h.store.InsertRefreshToken(ctx, id, tokens.HashRefreshToken(raw), subject, tokens.RoleOperator, time.Now().Add(operatorRefreshTTL)); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	return &server.TokenPairResponse{
		AccessToken:      accessToken,
		RefreshToken:     encoded,
		ExpiresIn:        int(tokens.OperatorTokenTTL.Seconds()),
		RefreshExpiresIn: int(operatorRefreshTTL.Seconds()),
	}, nil
}

func (h *Handler) TokenRefresh(ctx context.Context, req *server.TokenRefreshReq) (server.TokenRefreshRes, error) {
	if req.RefreshToken == "" {
		return &server.TokenRefreshBadRequest{Error: server.ErrorDetail{
			Code:    "invalid_request",
			Message: "request body must include a non-empty refresh_token field",
			Remedy:  "provide a valid refresh token",
		}}, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(req.RefreshToken)
	if err != nil {
		return &server.TokenRefreshUnauthorized{Error: server.ErrorDetail{
			Code:    "invalid_token",
			Message: "refresh token is malformed",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}

	found, findErr := h.store.GetRefreshTokenByHash(ctx, tokens.HashRefreshToken(raw))
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
	if err := h.store.RotateRefreshToken(ctx, rt.ID, tokens.HashRefreshToken(newRaw), time.Now().Add(operatorRefreshTTL)); err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}

	return &server.TokenPairResponse{
		AccessToken:      accessToken,
		RefreshToken:     newEncoded,
		ExpiresIn:        int(tokens.OperatorTokenTTL.Seconds()),
		RefreshExpiresIn: int(operatorRefreshTTL.Seconds()),
	}, nil
}

// validateRefreshToken checks db-level validity of a refresh token row.
// Returns the row on success, a typed TokenRefreshRes on a 4xx condition,
// or an error for infrastructure failures.
func validateRefreshToken(rt store.RefreshToken, lookupErr error) (*store.RefreshToken, server.TokenRefreshRes, error) {
	if errors.Is(lookupErr, sql.ErrNoRows) {
		return nil, &server.TokenRefreshUnauthorized{Error: server.ErrorDetail{
			Code:    "invalid_token",
			Message: "refresh token not found or already rotated",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}
	if lookupErr != nil {
		return nil, nil, fmt.Errorf("get refresh token: %w", lookupErr)
	}
	if rt.RevokedAt.Valid {
		return nil, &server.TokenRefreshUnauthorized{Error: server.ErrorDetail{
			Code:    "token_revoked",
			Message: "refresh token has been revoked",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, &server.TokenRefreshUnauthorized{Error: server.ErrorDetail{
			Code:    "token_expired",
			Message: "refresh token has expired",
			Remedy:  "re-authenticate to obtain a new refresh token",
		}}, nil
	}
	if rt.Role != tokens.RoleOperator {
		return nil, &server.TokenRefreshUnauthorized{Error: server.ErrorDetail{
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
