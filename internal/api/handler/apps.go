package handler

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

// ── App-scoped stubs ──────────────────────────────────────────────────────────

func (h *Handler) ListApps(ctx context.Context, params oas.ListAppsParams) (oas.ListAppsRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpsertApp(ctx context.Context, _ *oas.AppSpec, params oas.UpsertAppParams) (oas.UpsertAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetApp(ctx context.Context, params oas.GetAppParams) (oas.GetAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) DeleteApp(ctx context.Context, params oas.DeleteAppParams) (oas.DeleteAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetAppStatus(ctx context.Context, params oas.GetAppStatusParams) (oas.GetAppStatusRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetAppHistory(ctx context.Context, params oas.GetAppHistoryParams) (oas.GetAppHistoryRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetAppUtilisation(ctx context.Context, params oas.GetAppUtilisationParams) (oas.GetAppUtilisationRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperation(ctx context.Context, params oas.GetOperationParams) (oas.GetOperationRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) HibernateApp(ctx context.Context, params oas.HibernateAppParams) (oas.HibernateAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) WakeApp(ctx context.Context, params oas.WakeAppParams) (oas.WakeAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}
