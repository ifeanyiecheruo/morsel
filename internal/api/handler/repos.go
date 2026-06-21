package handler

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

// ── Repo-scoped stubs ─────────────────────────────────────────────────────────

func (h *Handler) ListRepos(_ context.Context, _ oas.ListReposParams) (oas.ListReposRes, error) {
	return nil, errNotImplemented
}

func (h *Handler) GetRepo(_ context.Context, _ oas.GetRepoParams) (oas.GetRepoRes, error) {
	return nil, errNotImplemented
}

func (h *Handler) DeleteRepo(ctx context.Context, params oas.DeleteRepoParams) (oas.DeleteRepoRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) SyncRepo(ctx context.Context, _ *oas.SyncRepoReq, params oas.SyncRepoParams) (oas.SyncRepoRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) ListRepoApprovals(ctx context.Context, params oas.ListRepoApprovalsParams) (oas.ListRepoApprovalsRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}
