package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

func writeJSONError(ctx context.Context, w http.ResponseWriter, status int, code, message, remedy string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	type detail struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Remedy  string `json:"remedy,omitempty"`
	}
	if err := json.NewEncoder(w).Encode(struct {
		Error detail `json:"error"`
	}{Error: detail{Code: code, Message: message, Remedy: remedy}}); err != nil {
		ctxlog.From(ctx).Warn("write error response", "err", err)
	}
}

// githubUser is the subset of the GitHub /user API response we care about.
type githubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
}

// fetchGitHubUser calls the GitHub /user API and returns the authenticated user's
// identity. The token must be a valid GitHub user access token (OAuth or Device Flow).
func fetchGitHubUser(ctx context.Context, token string) (githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return githubUser{}, fmt.Errorf("build github request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return githubUser{}, fmt.Errorf("call github api: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			ctxlog.From(ctx).Warn("close github api response body", "err", err)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return githubUser{}, errors.New("github token rejected")
	}
	if resp.StatusCode != http.StatusOK {
		return githubUser{}, fmt.Errorf("github api returned %d", resp.StatusCode)
	}

	var u githubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return githubUser{}, fmt.Errorf("decode github user: %w", err)
	}
	if u.ID == 0 || u.Login == "" {
		return githubUser{}, errors.New("github returned empty user identity")
	}
	return u, nil
}

// HandleGitHubConfig handles GET /github/config.
// Returns the GitHub OAuth App client ID so CLI tools can initiate Device Flow without
// hardcoding the client ID. Returns an empty string when GitHub OAuth is not configured.
func (h *Handler) HandleGitHubConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		ClientID string `json:"client_id"`
	}{ClientID: h.githubClientID}); err != nil {
		ctxlog.From(ctx).Warn("write github config response", "err", err)
	}
}

type githubAuthRequest struct {
	Token string `json:"token"`
}

// HandleGitHubAuth handles POST /token/oidc. Behaviour depends on bootstrap mode:
//
//   - Bootstrap mode (bootstrap token secret present): verifies the GitHub token and
//     returns a short-lived Morsel identity token (role=bootstrap_id). No principal is
//     created yet. The caller must present this token to POST /bootstrap together with
//     the bootstrap token to complete the first-admin setup.
//
//   - Normal mode: upserts the principal and issues operator/admin API tokens.
//     Returns 403 when the principal has no elevated role.
func (h *Handler) HandleGitHubAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(r.Context(), w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", "")
		return
	}

	var req githubAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeJSONError(r.Context(), w, http.StatusBadRequest, "invalid_request", "body must include a non-empty token field", "")
		return
	}

	ctx := r.Context()

	ghUser, err := fetchGitHubUser(ctx, req.Token)
	if err != nil {
		writeJSONError(ctx, w, http.StatusUnauthorized, "invalid_token", "GitHub token could not be verified", "ensure the token is a valid GitHub user access token")
		return
	}

	// Bootstrap mode: return an identity-only token. The caller must combine it
	// with the bootstrap token to create the first admin via POST /bootstrap.
	if _, btErr := h.plat.Secrets().GetBootstrapToken(ctx); btErr == nil {
		idToken, err := tokens.IssueToken(h.signingKey, tokens.CreateBootstrapIDClaims(ghUser.Login, ghUser.ID))
		if err != nil {
			writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not issue identity token", "")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}{AccessToken: idToken, ExpiresIn: 300}); err != nil {
			ctxlog.From(ctx).Warn("write bootstrap id token response", "err", err)
		}
		return
	}

	principal, err := h.store.UpsertPrincipal(ctx, ghUser.ID, ghUser.Login)
	if err != nil {
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not record principal", "")
		return
	}

	var role string
	switch {
	case principal.IsAdmin != 0:
		role = tokens.RoleAdmin
	case principal.IsOperator != 0:
		role = tokens.RoleOperator
	default:
		writeJSONError(ctx, w, http.StatusForbidden, "insufficient_role",
			"this GitHub account has viewer access only — API tokens require operator or admin role",
			"ask a platform admin to grant you operator access")
		return
	}

	subject := principal.GithubLogin

	var claims tokens.Claims
	if role == tokens.RoleAdmin {
		claims = tokens.CreateAdminAPIClaims(subject)
	} else {
		claims = tokens.CreateOperatorClaims(subject)
	}

	accessToken, err := tokens.IssueToken(h.signingKey, claims)
	if err != nil {
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not issue access token", "")
		return
	}

	raw, encoded, err := tokens.GenerateRefreshToken()
	if err != nil {
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not generate refresh token", "")
		return
	}

	id, err := newTokenID()
	if err != nil {
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not generate token id", "")
		return
	}

	const refreshTTL = 90 * 24 * time.Hour
	if err := h.store.InsertRefreshToken(ctx, id, tokens.HashRefreshToken(raw), subject, role, time.Now().Add(refreshTTL)); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not store refresh token", "")
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int    `json:"expires_in"`
		RefreshExpiresIn int    `json:"refresh_expires_in"`
	}{
		AccessToken:      accessToken,
		RefreshToken:     encoded,
		ExpiresIn:        int(tokens.OperatorTokenTTL.Seconds()),
		RefreshExpiresIn: int(refreshTTL.Seconds()),
	}); err != nil {
		ctxlog.From(ctx).Warn("write auth token response", "err", err)
	}
}
