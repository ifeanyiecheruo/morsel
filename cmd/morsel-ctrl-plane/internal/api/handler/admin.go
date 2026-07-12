package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

// requireOperatorHTTP validates the Bearer token in a non-ogen HTTP handler.
// Returns false and writes a 401/403 response when auth fails — callers must not
// write further on a false return.
func (h *Handler) requireOperatorHTTP(w http.ResponseWriter, r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		adminWriteError(r.Context(), w, http.StatusUnauthorized, "invalid_token", "Authorization: Bearer header is required")
		return false
	}
	claims, err := tokens.VerifyToken(h.signingKey, tok)
	if err != nil || !tokens.IsOperatorRole(claims.Role) {
		adminWriteError(r.Context(), w, http.StatusForbidden, "insufficient_role", "operator role required")
		return false
	}
	return true
}

// requireAdminHTTP validates the Bearer token and enforces admin role in a non-ogen HTTP handler.
func (h *Handler) requireAdminHTTP(w http.ResponseWriter, r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		adminWriteError(r.Context(), w, http.StatusUnauthorized, "invalid_token", "Authorization: Bearer header is required")
		return false
	}
	claims, err := tokens.VerifyToken(h.signingKey, tok)
	if err != nil || claims.Role != tokens.RoleAdmin {
		adminWriteError(r.Context(), w, http.StatusForbidden, "insufficient_role", "admin role required")
		return false
	}
	return true
}

type adminAppRow struct {
	RepoSlug    string    `json:"repo_slug"`
	Name        string    `json:"name"`
	URL         string    `json:"url,omitempty"`
	Status      string    `json:"status"`
	Hibernated  bool      `json:"hibernated"`
	Tier        string    `json:"tier"`
	CostMonthly float64   `json:"cost_monthly"`
	LastDeploy  time.Time `json:"last_deploy"`
}

// HandleAdminListApps handles GET /api/operator/apps and returns all apps across
// all repos, enriched with tier and estimated monthly cost.
func (h *Handler) HandleAdminListApps(w http.ResponseWriter, r *http.Request) {
	if !h.requireOperatorHTTP(w, r) {
		return
	}
	ctx := r.Context()

	allApps, err := h.store.ListAllApps(ctx)
	if err != nil {
		adminWriteError(ctx, w, http.StatusInternalServerError, "internal_error", "list apps failed")
		return
	}

	prices := h.latestPrices(ctx)
	now := time.Now().UTC()

	baseDomain := h.plat.BaseDomain()
	out := make([]adminAppRow, 0, len(allApps))
	for _, app := range allApps {
		repo, repoErr := h.store.GetRepo(ctx, app.RepoSlug)
		if repoErr != nil {
			ctxlog.From(ctx).Warn("get repo for app", "repo", app.RepoSlug, "err", repoErr)
		}
		tier, tierErr := h.store.GetTier(ctx, repo.Tier)
		if tierErr != nil {
			ctxlog.From(ctx).Warn("get tier for repo", "tier", repo.Tier, "err", tierErr)
		}
		appCost := h.appCostMonthly(ctx, app, tier, prices, now)
		var appURL string
		if app.Type == "http" {
			appURL = names.AppURL(app.Name, names.RepoName(app.RepoSlug), baseDomain, h.plat.GatewayPort())
		}
		out = append(out, adminAppRow{
			RepoSlug:    app.RepoSlug,
			Name:        app.Name,
			URL:         appURL,
			Status:      app.Status,
			Hibernated:  app.Hibernated != 0,
			Tier:        repo.Tier,
			CostMonthly: appCost,
			LastDeploy:  app.UpdatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		ctxlog.From(ctx).Warn("write response", "err", err)
	}
}

type adminStaleRow struct {
	ID         int64     `json:"id"`
	RepoSlug   string    `json:"repo_slug"`
	AppName    string    `json:"app_name"`
	LastDeploy time.Time `json:"last_deploy"`
}

const adminStaleThreshold = 30 * 24 * time.Hour

// HandleAdminListStale handles GET /api/operator/stale and returns apps that
// have not been deployed in 30 days and are not suppressed.
func (h *Handler) HandleAdminListStale(w http.ResponseWriter, r *http.Request) {
	if !h.requireOperatorHTTP(w, r) {
		return
	}
	ctx := r.Context()

	allApps, err := h.store.ListAllApps(ctx)
	if err != nil {
		adminWriteError(ctx, w, http.StatusInternalServerError, "internal_error", "list apps failed")
		return
	}

	suppressed, err := h.store.ListActiveStaleSuppressed(ctx)
	if err != nil {
		ctxlog.From(ctx).Warn("list active stale suppressed", "err", err)
	}
	suppressedSet := map[string]struct{}{}
	for _, s := range suppressed {
		suppressedSet[s.RepoSlug+"/"+s.AppName] = struct{}{}
	}

	now := time.Now()
	rows := make([]adminStaleRow, 0)
	for _, app := range allApps {
		if _, ok := suppressedSet[app.RepoSlug+"/"+app.Name]; ok {
			continue
		}
		if now.Sub(app.UpdatedAt) >= adminStaleThreshold {
			rows = append(rows, adminStaleRow{
				ID:         app.ID,
				RepoSlug:   app.RepoSlug,
				AppName:    app.Name,
				LastDeploy: app.UpdatedAt,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rows); err != nil {
		ctxlog.From(ctx).Warn("write response", "err", err)
	}
}

// HandleAdminIgnoreStale handles POST /api/operator/stale/{org}/{repo}/{appName}/ignore
// and suppresses stale notifications for the given app for 30 days.
func (h *Handler) HandleAdminIgnoreStale(w http.ResponseWriter, r *http.Request) {
	if !h.requireOperatorHTTP(w, r) {
		return
	}
	slug := r.PathValue("org") + "/" + r.PathValue("repo")
	appName := r.PathValue("appName")
	until := time.Now().Add(30 * 24 * time.Hour)
	if err := h.store.SuppressStaleApp(r.Context(), slug, appName, until); err != nil {
		ctxlog.From(r.Context()).Warn("suppress stale app", "err", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func adminWriteError(ctx context.Context, w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	type errBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewEncoder(w).Encode(map[string]errBody{"error": {Code: code, Message: message}}); err != nil {
		ctxlog.From(ctx).Warn("write error response", "err", err)
	}
}
