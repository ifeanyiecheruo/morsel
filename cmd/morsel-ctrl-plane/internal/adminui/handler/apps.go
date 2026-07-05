package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type apiAppRow struct {
	RepoSlug    string    `json:"repo_slug"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Status      string    `json:"status"`
	Hibernated  bool      `json:"hibernated"`
	Tier        string    `json:"tier"`
	CostMonthly float64   `json:"cost_monthly"`
	LastDeploy  time.Time `json:"last_deploy"`
}

// ServeApps handles GET /apps.
func (h *Handler) ServeApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	repoFilter := r.URL.Query().Get("repo")

	resp, err := h.apiGet(ctx, "/api/operator/apps")
	if err != nil || resp.StatusCode == http.StatusUnauthorized {
		if resp != nil {
			_ = resp.Body.Close()
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var apiRows []apiAppRow
	if err := json.NewDecoder(resp.Body).Decode(&apiRows); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	repoSet := map[string]struct{}{}
	var rows []pages.AppRow
	for _, a := range apiRows {
		repoSet[a.RepoSlug] = struct{}{}
		if repoFilter != "" && a.RepoSlug != repoFilter {
			continue
		}
		rows = append(rows, pages.AppRow{
			RepoSlug:   a.RepoSlug,
			Name:       a.Name,
			URL:        a.URL,
			Status:     a.Status,
			Hibernated: a.Hibernated,
			Tier:       a.Tier,
			Cost:       a.CostMonthly,
			LastDeploy: a.LastDeploy,
		})
	}

	repos := make([]string, 0, len(repoSet))
	for slug := range repoSet {
		repos = append(repos, slug)
	}

	_ = pages.AppsPage(pages.AppsPageData{
		Apps:       rows,
		Total:      len(rows),
		Repos:      repos,
		RepoFilter: repoFilter,
	}).Render(ctx, w)
}

// ServeAppDeleteConfirm handles GET /apps/{org}/{repo}/{appName}/delete.
func (h *Handler) ServeAppDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	slug, appName := appParams(r)
	_ = pages.AppDeleteConfirmPage(slug, appName).Render(r.Context(), w)
}

// HandleAppDelete handles POST /apps/{org}/{repo}/{appName}/delete.
// It calls DELETE /api/repos/{org}/{repo}/apps/{name} on the REST API.
func (h *Handler) HandleAppDelete(w http.ResponseWriter, r *http.Request) {
	slug, appName := appParams(r)
	org, repo, _ := strings.Cut(slug, "/")
	path := "/api/repos/" + org + "/" + repo + "/apps/" + appName

	resp, err := h.apiDelete(r.Context(), path)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

// HandleAppHibernate handles POST /apps/{org}/{repo}/{appName}/hibernate.
func (h *Handler) HandleAppHibernate(w http.ResponseWriter, r *http.Request) {
	slug, appName := appParams(r)
	org, repo, _ := strings.Cut(slug, "/")
	path := "/api/repos/" + org + "/" + repo + "/apps/" + appName + "/hibernate"

	resp, err := h.apiPost(r.Context(), path, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

// HandleAppWake handles POST /apps/{org}/{repo}/{appName}/wake.
func (h *Handler) HandleAppWake(w http.ResponseWriter, r *http.Request) {
	slug, appName := appParams(r)
	org, repo, _ := strings.Cut(slug, "/")
	path := "/api/repos/" + org + "/" + repo + "/apps/" + appName + "/wake"

	resp, err := h.apiPost(r.Context(), path, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}
