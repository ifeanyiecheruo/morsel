package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
)

// requireOperatorHTTP validates the Bearer token in a non-ogen HTTP handler.
// Returns false and writes a 401/403 response when auth fails — callers must not
// write further on a false return.
func (h *Handler) requireOperatorHTTP(w http.ResponseWriter, r *http.Request) bool {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if tok == "" {
		adminWriteError(w, http.StatusUnauthorized, "invalid_token", "Authorization: Bearer header is required")
		return false
	}
	claims, err := tokens.VerifyToken(h.signingKey, tok)
	if err != nil || claims.Role != tokens.RoleOperator {
		adminWriteError(w, http.StatusForbidden, "insufficient_role", "operator role required")
		return false
	}
	return true
}

type adminAppRow struct {
	RepoSlug    string    `json:"repo_slug"`
	Name        string    `json:"name"`
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
		adminWriteError(w, http.StatusInternalServerError, "internal_error", "list apps failed")
		return
	}

	prices := h.latestPrices(ctx)
	now := time.Now().UTC()

	out := make([]adminAppRow, 0, len(allApps))
	for _, app := range allApps {
		repo, _ := h.store.GetRepo(ctx, app.RepoSlug)
		tier, _ := h.store.GetTier(ctx, repo.Tier)
		appCost := h.appCostMonthly(ctx, app, tier, prices, now)
		out = append(out, adminAppRow{
			RepoSlug:    app.RepoSlug,
			Name:        app.Name,
			Status:      app.Status,
			Hibernated:  app.Hibernated != 0,
			Tier:        repo.Tier,
			CostMonthly: appCost,
			LastDeploy:  app.UpdatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
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
		adminWriteError(w, http.StatusInternalServerError, "internal_error", "list apps failed")
		return
	}

	suppressed, _ := h.store.ListActiveStaleSuppressed(ctx)
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
	_ = json.NewEncoder(w).Encode(rows)
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
	_ = h.store.SuppressStaleApp(r.Context(), slug, appName, until)
	w.WriteHeader(http.StatusNoContent)
}

func adminWriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	type errBody struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.NewEncoder(w).Encode(map[string]errBody{"error": {Code: code, Message: message}})
}
