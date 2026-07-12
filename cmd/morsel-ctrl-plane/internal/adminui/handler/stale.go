package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type apiStaleRow struct {
	ID         int64     `json:"id"`
	RepoSlug   string    `json:"repo_slug"`
	AppName    string    `json:"app_name"`
	LastDeploy time.Time `json:"last_deploy"`
}

// ServeStale handles GET /stale.
func (h *Handler) ServeStale(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp, err := h.apiGet(ctx, "/api/operator/stale")
	if err != nil || resp.StatusCode == http.StatusUnauthorized {
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close response body", "err", closeErr)
			}
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var apiRows []apiStaleRow
	if err := json.NewDecoder(resp.Body).Decode(&apiRows); err != nil {
		ctxlog.From(ctx).Warn("decode stale rows", "err", err)
	}

	rows := make([]pages.StaleRow, 0, len(apiRows))
	for _, row := range apiRows {
		rows = append(rows, pages.StaleRow{
			ID:         row.ID,
			RepoSlug:   row.RepoSlug,
			AppName:    row.AppName,
			LastDeploy: row.LastDeploy,
		})
	}

	if err := pages.StalePage(pages.StalePageData{Rows: rows}).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render stale page", "err", err)
	}
}

// HandleStaleIgnore handles POST /stale/{org}/{repo}/{appName}/ignore.
func (h *Handler) HandleStaleIgnore(w http.ResponseWriter, r *http.Request) {
	slug, appName := appParams(r)
	org, repo, _ := strings.Cut(slug, "/")
	path := "/api/operator/stale/" + org + "/" + repo + "/" + appName + "/ignore"

	ctx := r.Context()
	resp, err := h.apiPost(ctx, path, nil)
	if resp != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}
	if err != nil {
		ctxlog.From(ctx).Warn("stale ignore api call", "err", err)
	}
	http.Redirect(w, r, "/stale", http.StatusSeeOther)
}
