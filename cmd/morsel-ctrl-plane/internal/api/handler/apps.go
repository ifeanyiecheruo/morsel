package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// ── App-scoped handlers ───────────────────────────────────────────────────────

func (h *Handler) ListApps(ctx context.Context, params server.ListAppsParams) (server.ListAppsRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	apps, err := h.store.ListApps(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	out := make(server.ListAppsOKApplicationJSON, len(apps))
	for i, app := range apps {
		out[i] = dbAppToOAS(app)
	}
	return &out, nil
}

func (h *Handler) UpsertApp(ctx context.Context, spec *server.AppSpec, params server.UpsertAppParams) (server.UpsertAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	name := spec.Name.Or("")
	slug := names.RepoSlug(params.Org, params.Repo)
	ns := names.AppNamespace(names.RepoSlug(params.Org, params.Repo), name)

	defaultTier := h.store.GetDefaultTierName(ctx)
	repo, err := h.store.GetOrCreateRepo(ctx, slug, defaultTier)
	if err != nil {
		return nil, fmt.Errorf("get or create repo: %w", err)
	}

	// Enforce app count quota: only check for new apps (not re-deploys).
	if _, appErr := h.store.GetApp(ctx, slug, name); errors.Is(appErr, sql.ErrNoRows) {
		tier, tierErr := h.store.GetTier(ctx, repo.Tier)
		if tierErr == nil {
			count, countErr := h.store.CountAppsByRepo(ctx, slug)
			if countErr == nil && count >= tier.MaxApps {
				return nil, &apiError{
					httpStatus: http.StatusUnprocessableEntity,
					code:       "quota_exceeded",
					message:    fmt.Sprintf("repo is at its app limit of %d for tier %q", tier.MaxApps, repo.Tier),
					remedy:     "contact your platform operator to request a tier upgrade",
				}
			}
		}
	}

	app, err := h.store.UpsertApp(ctx, slug, name, string(spec.Type), ns, spec.Image, spec.IdleAfter.Or(""))
	if err != nil {
		return nil, fmt.Errorf("upsert app: %w", err)
	}

	opID, err := newOperationID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	if _, err := h.store.CreateOperation(ctx, opID, slug, name, "deploy"); err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}

	var env map[string]string
	if spec.Env.Set {
		env = map[string]string(spec.Env.Value)
	}
	manifest := kube.AppManifest{
		Namespace:  ns,
		AppName:    name,
		RepoName:   params.Repo,
		Type:       string(spec.Type),
		Image:      spec.Image,
		Port:       int32(spec.Port.Or(0)),
		Env:        env,
		Schedule:   spec.Schedule.Or(""),
		Private:    spec.Private.Or(false),
		BaseDomain: h.plat.BaseDomain(),
		GatewayNS:  h.plat.Namespace(),
	}
	lastHealthy := ""
	if app.ImageLastHealthy.Valid {
		lastHealthy = app.ImageLastHealthy.String
	}

	go h.runDeploy(context.WithoutCancel(ctx), opID, app.ID, manifest, lastHealthy)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) runDeploy(ctx context.Context, opID string, appID int64, m kube.AppManifest, lastHealthyImage string) {
	logger := ctxlog.From(ctx).With("op", opID, "app", m.AppName)

	if err := h.store.StartOperation(ctx, opID); err != nil {
		logger.Error("start operation", "err", err)
	}
	if err := h.store.UpdateAppStatus(ctx, appID, "deploying"); err != nil {
		logger.Error("update app status deploying", "err", err)
	}

	if err := h.deployer.Apply(ctx, m); err != nil {
		logger.Error("apply manifests", "err", err)
		_ = h.store.FailOperation(ctx, opID, err.Error())
		_ = h.store.UpdateAppStatus(ctx, appID, "failed")
		return
	}

	if m.Type != "cron" {
		if err := h.deployer.WatchDeploymentRollout(ctx, m.Namespace); err != nil {
			logger.Error("rollout failed, rolling back", "err", err)
			if lastHealthyImage != "" {
				if rbErr := h.deployer.RollbackDeployment(ctx, m.Namespace, lastHealthyImage); rbErr != nil {
					logger.Error("rollback failed", "err", rbErr)
				} else {
					_ = h.store.UpdateAppImages(ctx, appID, lastHealthyImage, lastHealthyImage)
				}
			}
			_ = h.store.FailOperation(ctx, opID, err.Error())
			_ = h.store.UpdateAppStatus(ctx, appID, "failed")
			return
		}
	}

	_ = h.store.UpdateAppImages(ctx, appID, m.Image, m.Image)
	_ = h.store.UpdateAppStatus(ctx, appID, "running")
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed operation", "err", err)
	}
}

func (h *Handler) GetApp(ctx context.Context, params server.GetAppParams) (server.GetAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	app, err := h.store.GetApp(ctx, names.RepoSlug(params.Org, params.Repo), params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.GetAppNotFound{Error: server.ErrorDetail{
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

func (h *Handler) DeleteApp(ctx context.Context, params server.DeleteAppParams) (server.DeleteAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	app, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.DeleteAppNotFound{Error: server.ErrorDetail{
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

	ns := names.AppNamespace(names.RepoSlug(params.Org, params.Repo), params.Name)
	go h.runDelete(context.WithoutCancel(ctx), opID, app.ID, ns)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) runDelete(ctx context.Context, opID string, appID int64, namespace string) {
	logger := ctxlog.From(ctx).With("op", opID, "namespace", namespace)

	if err := h.store.StartOperation(ctx, opID); err != nil {
		logger.Error("start delete operation", "err", err)
	}

	if err := h.deployer.Delete(ctx, namespace); err != nil {
		logger.Error("delete kubernetes resources", "err", err)
		_ = h.store.FailOperation(ctx, opID, err.Error())
		return
	}

	_ = h.store.UpdateAppStatus(ctx, appID, "deleted")
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed delete operation", "err", err)
	}
}

func (h *Handler) GetAppStatus(ctx context.Context, params server.GetAppStatusParams) (server.GetAppStatusRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	app, err := h.store.GetApp(ctx, names.RepoSlug(params.Org, params.Repo), params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.GetAppStatusNotFound{Error: server.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}
	ns := ""
	if app.Namespace.Valid {
		ns = app.Namespace.String
	}
	desired, ready := h.deployer.AppReplicaCounts(ctx, ns, app.Type)
	status := h.deployer.AppStatus(ctx, ns, app.Type)
	out := server.GetAppStatusOK{
		Status:        status,
		Replicas:      server.NewOptInt(int(desired)),
		ReadyReplicas: server.NewOptInt(int(ready)),
		Hibernated:    server.NewOptBool(app.Hibernated != 0),
	}
	if app.HibernatedAt.Valid {
		out.HibernatedAt = server.NewOptDateTime(app.HibernatedAt.Time)
	}
	if app.LastActiveAt.Valid {
		out.IdleSince = server.NewOptDateTime(app.LastActiveAt.Time)
	}
	return &out, nil
}

func (h *Handler) GetAppHistory(ctx context.Context, params server.GetAppHistoryParams) (server.GetAppHistoryRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	_, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.GetAppHistoryNotFound{Error: server.ErrorDetail{
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
	out := make(server.GetAppHistoryOKApplicationJSON, len(ops))
	for i, op := range ops {
		out[i] = dbOperationToOAS(op)
	}
	return &out, nil
}

func (h *Handler) GetAppUtilisation(ctx context.Context, params server.GetAppUtilisationParams) (server.GetAppUtilisationRes, error) {
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

func (h *Handler) GetOperation(ctx context.Context, params server.GetOperationParams) (server.GetOperationRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	op, err := h.store.GetOperation(ctx, params.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.GetOperationNotFound{Error: server.ErrorDetail{
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

func (h *Handler) HibernateApp(ctx context.Context, params server.HibernateAppParams) (server.HibernateAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	app, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.HibernateAppNotFound{Error: server.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	opID, err := newOperationID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	if _, err := h.store.CreateOperation(ctx, opID, slug, params.Name, "hibernate"); err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}

	ns := ""
	if app.Namespace.Valid {
		ns = app.Namespace.String
	}
	host := names.AppHostname(params.Name, params.Repo, h.plat.BaseDomain())
	go h.runHibernate(context.WithoutCancel(ctx), opID, app.ID, app.Type, ns, host)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) runHibernate(ctx context.Context, opID string, appID int64, appType, namespace, host string) {
	logger := ctxlog.From(ctx).With("op", opID, "namespace", namespace)
	if err := h.store.StartOperation(ctx, opID); err != nil {
		logger.Error("start hibernate operation", "err", err)
	}

	switch appType {
	case "http":
		if err := h.deployer.RouteToWakeProxy(ctx, namespace, host, h.plat.Namespace(), kube.GatewayExternal); err != nil {
			logger.Error("route to wake proxy", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
		if err := h.deployer.ScaleDeployment(ctx, namespace, 0); err != nil {
			logger.Error("scale to zero", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
	case "worker":
		if err := h.deployer.ScaleDeployment(ctx, namespace, 0); err != nil {
			logger.Error("scale to zero", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
	case "cron":
		if err := h.deployer.SuspendCronJob(ctx, namespace); err != nil {
			logger.Error("suspend cron job", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
	}

	if err := h.store.SetAppHibernated(ctx, appID, "manual"); err != nil {
		logger.Error("set app hibernated", "err", err)
	}
	_ = h.store.UpdateAppStatus(ctx, appID, "hibernated")
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed hibernate operation", "err", err)
	}
}

func (h *Handler) WakeApp(ctx context.Context, params server.WakeAppParams) (server.WakeAppRes, error) {
	if err := checkRepoAccess(ctx, params.Org, params.Repo); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	app, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.WakeAppNotFound{Error: server.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	opID, err := newOperationID()
	if err != nil {
		return nil, fmt.Errorf("generate operation id: %w", err)
	}
	if _, err := h.store.CreateOperation(ctx, opID, slug, params.Name, "wake"); err != nil {
		return nil, fmt.Errorf("create operation: %w", err)
	}

	ns := ""
	if app.Namespace.Valid {
		ns = app.Namespace.String
	}
	host := names.AppHostname(params.Name, params.Repo, h.plat.BaseDomain())
	go h.runWake(context.WithoutCancel(ctx), opID, app.ID, app.Type, ns, host)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
}

func (h *Handler) runWake(ctx context.Context, opID string, appID int64, appType, namespace, host string) {
	logger := ctxlog.From(ctx).With("op", opID, "namespace", namespace)
	if err := h.store.StartOperation(ctx, opID); err != nil {
		logger.Error("start wake operation", "err", err)
	}

	switch appType {
	case "http":
		if err := h.deployer.ScaleDeployment(ctx, namespace, 1); err != nil {
			logger.Error("scale up", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
		if err := h.deployer.WatchDeploymentReady(ctx, namespace, 5*time.Minute); err != nil {
			logger.Error("wait for ready", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
		if err := h.deployer.RestoreHTTPRoute(ctx, namespace, host, h.plat.Namespace(), kube.GatewayExternal, 8080); err != nil {
			logger.Error("restore http route", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
	case "worker":
		if err := h.deployer.ScaleDeployment(ctx, namespace, 1); err != nil {
			logger.Error("scale up", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
	case "cron":
		if err := h.deployer.UnsuspendCronJob(ctx, namespace); err != nil {
			logger.Error("unsuspend cron job", "err", err)
			_ = h.store.FailOperation(ctx, opID, err.Error())
			return
		}
	}

	if err := h.store.SetAppAwake(ctx, appID); err != nil {
		logger.Error("set app awake", "err", err)
	}
	_ = h.store.UpdateAppStatus(ctx, appID, "running")
	if err := h.store.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed wake operation", "err", err)
	}
}
