package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

// localAdminID is the synthetic GitHub ID used when bootstrapping without OAuth.
// Negative so it never conflicts with a real GitHub numeric user ID.
const localAdminID = int64(-1)

// localAdminLogin is the GitHub login placeholder for the local-dev admin.
const localAdminLogin = "local-admin"

// HandleBootstrap completes the first-admin setup for a freshly provisioned instance.
//
// When GitHub OAuth is configured the caller must also present:
//   - Authorization: Bearer <morsel-id-token> — the identity token obtained from
//     POST /token/oidc while the instance was still in bootstrap mode
//
// When GitHub OAuth is NOT configured (githubClientID is empty) the endpoint
// accepts the bootstrap token alone and creates a synthetic "local-admin" principal.
// This supports the local development workflow where no OAuth App is available.
func (h *Handler) HandleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(r.Context(), w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required", "")
		return
	}

	ctx := r.Context()

	// Validate the bootstrap token first.
	bootstrapToken := r.Header.Get("X-Bootstrap-Token")
	expected, err := h.plat.Secrets().GetBootstrapToken(ctx)
	if errors.Is(err, platform.ErrSecretNotFound) || bootstrapToken == "" || bootstrapToken != expected {
		writeJSONError(ctx, w, http.StatusUnauthorized, "invalid_bootstrap_token",
			"bootstrap token is missing, invalid, or already consumed", "")
		return
	}
	if err != nil {
		ctxlog.From(ctx).Error("bootstrap: get bootstrap token", "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not verify bootstrap token", "")
		return
	}

	var githubLogin string
	var githubID int64

	rawIDToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if rawIDToken != "" {
		// GitHub OAuth is available — verify the Morsel identity token.
		idClaims, verifyErr := tokens.VerifyToken(h.signingKey, rawIDToken)
		if verifyErr != nil || idClaims.Role != tokens.RoleBootstrapID {
			writeJSONError(ctx, w, http.StatusUnauthorized, "invalid_id_token",
				"identity token is invalid, expired, or not a bootstrap identity token", "")
			return
		}
		if idClaims.Subject == "" || idClaims.GithubID == 0 {
			writeJSONError(ctx, w, http.StatusUnauthorized, "invalid_id_token", "identity token is missing subject or github_id", "")
			return
		}
		githubLogin = idClaims.Subject
		githubID = idClaims.GithubID
	} else if h.githubClientID == "" {
		// Local dev mode: no OAuth App configured, no ID token — use synthetic identity.
		githubLogin = localAdminLogin
		githubID = localAdminID
	} else {
		// OAuth is configured but no ID token was presented.
		writeJSONError(ctx, w, http.StatusUnauthorized, "missing_id_token",
			"Authorization: Bearer <morsel-id-token> header is required", "")
		return
	}

	log := ctxlog.From(ctx)

	// Create the first admin principal.
	if _, err := h.store.UpsertPrincipal(ctx, githubID, githubLogin); err != nil {
		log.Error("bootstrap: upsert principal", "login", githubLogin, "id", githubID, "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not create admin principal", "")
		return
	}
	if err := h.store.SetAdmin(ctx, githubID, true); err != nil {
		log.Error("bootstrap: set admin", "login", githubLogin, "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not grant admin role", "")
		return
	}
	if err := h.store.SetOperator(ctx, githubID, true); err != nil {
		log.Error("bootstrap: set operator", "login", githubLogin, "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not grant operator role", "")
		return
	}

	// Exit bootstrap mode by consuming the bootstrap token.
	if delErr := h.plat.Secrets().DeleteBootstrapToken(ctx); delErr != nil {
		log.Warn("bootstrap: delete bootstrap token", "err", delErr)
	}

	// Issue admin API tokens.
	accessToken, err := tokens.IssueToken(h.signingKey, tokens.CreateAdminAPIClaims(githubLogin))
	if err != nil {
		log.Error("bootstrap: issue access token", "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not issue access token", "")
		return
	}

	raw, encoded, err := tokens.GenerateRefreshToken()
	if err != nil {
		log.Error("bootstrap: generate refresh token", "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not generate refresh token", "")
		return
	}

	id, err := newTokenID()
	if err != nil {
		log.Error("bootstrap: generate token id", "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not generate token id", "")
		return
	}

	const refreshTTL = 90 * 24 * time.Hour
	if err := h.store.InsertRefreshToken(ctx, id, tokens.HashRefreshToken(raw), githubLogin, tokens.RoleAdmin, time.Now().Add(refreshTTL)); err != nil {
		log.Error("bootstrap: insert refresh token", "err", err)
		writeJSONError(ctx, w, http.StatusInternalServerError, "internal_error", "could not store refresh token", "")
		return
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
		log.Warn("write bootstrap response", "err", err)
	}
}
