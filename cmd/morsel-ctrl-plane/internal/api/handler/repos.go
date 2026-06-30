package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// ── Repo-scoped handlers ──────────────────────────────────────────────────────

func (h *Handler) ListRepos(ctx context.Context, params server.ListReposParams) (server.ListReposRes, error) {
	claims := claimsFromContext(ctx)
	if claims == nil {
		return nil, &apiError{
			httpStatus: 401,
			code:       "invalid_token",
			message:    "request is missing an Authorization: Bearer header",
			remedy:     "include a valid Morsel access token in the Authorization header",
		}
	}

	// Operators with ?all=true list all repos. Developers list only their own.
	if claims.Role == tokens.RoleOperator && params.All.Or(false) {
		repos, err := h.store.ListRepos(ctx)
		if err != nil {
			return nil, fmt.Errorf("list repos: %w", err)
		}
		out := make(server.ListReposOKApplicationJSON, len(repos))
		for i, repo := range repos {
			count, countErr := h.store.CountAppsByRepo(ctx, repo.Slug)
			if countErr != nil {
				return nil, fmt.Errorf("count apps: %w", countErr)
			}
			out[i] = dbRepoToOAS(repo, count)
		}
		return &out, nil
	}

	// Developer (or operator without ?all=true) — return only the repo from the token claim.
	var slug string
	if claims.Role == tokens.RoleDeveloper {
		slug = claims.Repo
	} else {
		// Operator without all=true: return empty list (no repo claim to filter by).
		out := make(server.ListReposOKApplicationJSON, 0)
		return &out, nil
	}

	repo, err := h.store.GetRepo(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		out := make(server.ListReposOKApplicationJSON, 0)
		return &out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	count, err := h.store.CountAppsByRepo(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("count apps: %w", err)
	}
	out := server.ListReposOKApplicationJSON{dbRepoToOAS(repo, count)}
	return &out, nil
}

func (h *Handler) GetRepo(ctx context.Context, params server.GetRepoParams) (server.GetRepoRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := repoSlug(params.Org, params.Repo)
	repo, err := h.store.GetRepo(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.GetRepoNotFound{Error: server.ErrorDetail{
			Code:    "not_found",
			Message: "repo not found",
			Remedy:  "deploy an app to register the repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	count, err := h.store.CountAppsByRepo(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("count apps: %w", err)
	}
	out := dbRepoToOAS(repo, count)
	return &out, nil
}

func (h *Handler) DeleteRepo(ctx context.Context, params server.DeleteRepoParams) (server.DeleteRepoRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) SyncRepo(ctx context.Context, req *server.SyncRepoReq, params server.SyncRepoParams) (server.SyncRepoRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := repoSlug(params.Org, params.Repo)

	if _, err := h.store.GetOrCreateRepo(ctx, slug); err != nil {
		return nil, fmt.Errorf("get or create repo: %w", err)
	}

	// Build a set of desired app names for diffing.
	desired := make(map[string]struct{}, len(req.Apps))
	for _, spec := range req.Apps {
		name := spec.Name.Or("")
		desired[name] = struct{}{}
		ns := kube.AppNamespace(repoSlug(params.Org, params.Repo), name)
		if _, err := h.store.UpsertApp(ctx, slug, name, string(spec.Type), ns, spec.Image); err != nil {
			return nil, fmt.Errorf("upsert app %q: %w", name, err)
		}
	}

	// Mark apps absent from the desired list as deletion_pending.
	existing, err := h.store.ListApps(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	for _, app := range existing {
		if _, ok := desired[app.Name]; !ok {
			if err := h.store.MarkAppDeletionPending(ctx, app.ID); err != nil {
				return nil, fmt.Errorf("mark deletion pending for %q: %w", app.Name, err)
			}
		}
	}

	opID, err := newOperationID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	if _, err := h.store.CreateOperation(ctx, opID, slug, "", "sync"); err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		return nil, fmt.Errorf("succeed operation: %w", err)
	}

	return &server.AcceptedOperationHeaders{
		Location:   "/api/repos/" + params.Org + "/" + params.Repo + "/operations/" + opID,
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) ListRepoApprovals(ctx context.Context, params server.ListRepoApprovalsParams) (server.ListRepoApprovalsRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}
