package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

// ── App-scoped handlers ───────────────────────────────────────────────────────

func (h *Handler) ListApps(ctx context.Context, params oas.ListAppsParams) (oas.ListAppsRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := repoSlug(params.Org, params.Repo)
	apps, err := h.store.ListApps(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	out := make(oas.ListAppsOKApplicationJSON, len(apps))
	for i, app := range apps {
		out[i] = dbAppToOAS(app)
	}
	return &out, nil
}

func (h *Handler) UpsertApp(ctx context.Context, spec *oas.AppSpec, params oas.UpsertAppParams) (oas.UpsertAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	name := spec.Name.Or("")
	slug := repoSlug(params.Org, params.Repo)
	ns := appNamespace(params.Org, params.Repo, name)

	if _, err := h.store.GetOrCreateRepo(ctx, slug); err != nil {
		return nil, fmt.Errorf("get or create repo: %w", err)
	}

	if _, err := h.store.UpsertApp(ctx, slug, name, string(spec.Type), ns, spec.Image); err != nil {
		return nil, fmt.Errorf("upsert app: %w", err)
	}

	opID, err := newOperationID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	if _, err := h.store.CreateOperation(ctx, opID, slug, name, "deploy"); err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		return nil, fmt.Errorf("succeed operation: %w", err)
	}

	return &oas.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, name, opID),
		RetryAfter: oas.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   oas.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) GetApp(ctx context.Context, params oas.GetAppParams) (oas.GetAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	app, err := h.store.GetApp(ctx, repoSlug(params.Org, params.Repo), params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &oas.GetAppNotFound{Error: oas.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	out := dbAppToOAS(app)
	return &out, nil
}

func (h *Handler) DeleteApp(ctx context.Context, params oas.DeleteAppParams) (oas.DeleteAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := repoSlug(params.Org, params.Repo)
	app, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &oas.DeleteAppNotFound{Error: oas.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app for deletion: %w", err)
	}

	if err := h.store.MarkAppDeletionPending(ctx, app.ID); err != nil {
		return nil, fmt.Errorf("mark deletion pending: %w", err)
	}

	opID, err := newOperationID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	if _, err := h.store.CreateOperation(ctx, opID, slug, params.Name, "delete"); err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		return nil, fmt.Errorf("succeed operation: %w", err)
	}

	return &oas.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: oas.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   oas.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) GetAppStatus(ctx context.Context, params oas.GetAppStatusParams) (oas.GetAppStatusRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	_, err := h.store.GetApp(ctx, repoSlug(params.Org, params.Repo), params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &oas.GetAppStatusNotFound{Error: oas.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	// Kubernetes runtime state is not available until F06.
	return &oas.GetAppStatusOK{Status: "unknown"}, nil
}

func (h *Handler) GetAppHistory(ctx context.Context, params oas.GetAppHistoryParams) (oas.GetAppHistoryRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := repoSlug(params.Org, params.Repo)
	_, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &oas.GetAppHistoryNotFound{Error: oas.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	ops, err := h.store.ListAppOperations(ctx, slug, params.Name)
	if err != nil {
		return nil, fmt.Errorf("list operations: %w", err)
	}
	out := make(oas.GetAppHistoryOKApplicationJSON, len(ops))
	for i, op := range ops {
		out[i] = dbOperationToOAS(op)
	}
	return &out, nil
}

func (h *Handler) GetAppUtilisation(ctx context.Context, params oas.GetAppUtilisationParams) (oas.GetAppUtilisationRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	return nil, &apiError{
		httpStatus: http.StatusNotImplemented,
		code:       "not_implemented",
		message:    "utilisation metrics are not yet available",
		remedy:     "check back in a future release",
	}
}

func (h *Handler) GetOperation(ctx context.Context, params oas.GetOperationParams) (oas.GetOperationRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	op, err := h.store.GetOperation(ctx, params.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return &oas.GetOperationNotFound{Error: oas.ErrorDetail{
			Code:    "not_found",
			Message: "operation not found",
			Remedy:  "check the operation ID",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get operation: %w", err)
	}
	out := dbOperationToOAS(op)
	return &out, nil
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
