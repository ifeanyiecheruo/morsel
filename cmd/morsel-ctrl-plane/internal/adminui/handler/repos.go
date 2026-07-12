package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

type apiRepo struct {
	Slug               string  `json:"slug"`
	Tier               string  `json:"tier"`
	AppCount           int     `json:"app_count"`
	EstimatedCostMonth float64 `json:"estimated_cost_month"`
}

type apiApp struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type apiTier struct {
	Name    string `json:"name"`
	MaxApps int    `json:"max_apps"`
}

// ServeRepos handles GET /repos.
func (h *Handler) ServeRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resp, err := h.apiGet(ctx, "/api/repos?all=true")
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

	var apiRepos []apiRepo
	if err := json.NewDecoder(resp.Body).Decode(&apiRepos); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows := make([]pages.RepoRow, 0, len(apiRepos))
	for _, repo := range apiRepos {
		rows = append(rows, pages.RepoRow{
			Slug:     repo.Slug,
			AppCount: int64(repo.AppCount),
			Cost:     repo.EstimatedCostMonth,
			Tier:     repo.Tier,
		})
	}

	if err := pages.ReposPage(pages.ReposPageData{Repos: rows}).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render repos page", "err", err)
	}
}

// ServeRepoDeleteAllConfirm handles GET /repos/{org}/{repo}/delete-all.
func (h *Handler) ServeRepoDeleteAllConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := repoSlug(r)
	org, repo, _ := strings.Cut(slug, "/")

	resp, err := h.apiGet(ctx, "/api/repos/"+org+"/"+repo)
	count := 0
	if err == nil && resp.StatusCode == http.StatusOK {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close response body", "err", closeErr)
			}
		}()
		var repoData apiRepo
		if json.NewDecoder(resp.Body).Decode(&repoData) == nil {
			count = repoData.AppCount
		}
	} else if resp != nil {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}

	if err := pages.RepoDeleteAllConfirmPage(slug, int64(count)).Render(ctx, w); err != nil {
		ctxlog.From(ctx).Warn("render repo delete confirm page", "err", err)
	}
}

// HandleRepoDeleteAll handles POST /repos/{org}/{repo}/delete-all.
// It lists all apps in the repo and deletes each one via the REST API.
func (h *Handler) HandleRepoDeleteAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	slug := repoSlug(r)
	org, repo, _ := strings.Cut(slug, "/")

	resp, err := h.apiGet(ctx, "/api/repos/"+org+"/"+repo+"/apps")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var apps []apiApp
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		ctxlog.From(ctx).Warn("decode apps response", "err", err)
	}

	for _, app := range apps {
		delResp, delErr := h.apiDelete(ctx, "/api/repos/"+org+"/"+repo+"/apps/"+app.Name)
		if delResp != nil {
			if closeErr := delResp.Body.Close(); closeErr != nil {
				ctxlog.From(ctx).Warn("close delete response body", "err", closeErr)
			}
		}
		if delErr != nil {
			ctxlog.From(ctx).Warn("delete app", "app", app.Name, "err", delErr)
		}
	}

	http.Redirect(w, r, "/repos", http.StatusSeeOther)
}

// HandleRepoPromote handles POST /repos/{org}/{repo}/promote.
func (h *Handler) HandleRepoPromote(w http.ResponseWriter, r *http.Request) {
	h.handleRepoTierChange(w, r, +1)
}

// HandleRepoDemote handles POST /repos/{org}/{repo}/demote.
func (h *Handler) HandleRepoDemote(w http.ResponseWriter, r *http.Request) {
	h.handleRepoTierChange(w, r, -1)
}

func (h *Handler) handleRepoTierChange(w http.ResponseWriter, r *http.Request, direction int) {
	ctx := r.Context()
	slug := repoSlug(r)
	org, repo, _ := strings.Cut(slug, "/")

	repoResp, err := h.apiGet(ctx, "/api/repos/"+org+"/"+repo)
	if err != nil {
		http.Redirect(w, r, "/repos", http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := repoResp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var repoData struct {
		Tier string `json:"tier"`
	}
	if err := json.NewDecoder(repoResp.Body).Decode(&repoData); err != nil {
		http.Redirect(w, r, "/repos", http.StatusSeeOther)
		return
	}

	tiersResp, err := h.apiGet(ctx, "/api/operator/tiers")
	if err != nil {
		http.Redirect(w, r, "/repos", http.StatusSeeOther)
		return
	}
	defer func() {
		if closeErr := tiersResp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var tiers []apiTier
	if err := json.NewDecoder(tiersResp.Body).Decode(&tiers); err != nil || len(tiers) == 0 {
		http.Redirect(w, r, "/repos", http.StatusSeeOther)
		return
	}

	idx := -1
	for i, t := range tiers {
		if t.Name == repoData.Tier {
			idx = i
			break
		}
	}
	next := idx + direction
	if next < 0 || next >= len(tiers) {
		http.Redirect(w, r, "/repos", http.StatusSeeOther)
		return
	}

	newTier := tiers[next].Name
	body, bodyErr := jsonBody(map[string]string{"tier": newTier})
	if bodyErr != nil {
		ctxlog.From(ctx).Warn("encode tier patch body", "err", bodyErr)
	}
	patchResp, patchErr := h.apiDo(ctx, http.MethodPatch, "/api/operator/repos/"+org+"/"+repo, body)
	if patchResp != nil {
		if closeErr := patchResp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close patch response body", "err", closeErr)
		}
	}
	if patchErr != nil {
		ctxlog.From(ctx).Warn("patch repo tier", "err", patchErr)
	}
	http.Redirect(w, r, "/repos", http.StatusSeeOther)
}
