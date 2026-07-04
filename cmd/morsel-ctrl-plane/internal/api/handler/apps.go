package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/cost"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
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
	if err := h.store.RecordScaleEvent(ctx, m.Namespace, m.AppName, "scale_to_1"); err != nil {
		logger.Error("record scale event", "err", err)
	}
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
	go deleteApp(context.WithoutCancel(ctx), h.store, h.deployer, opID, app.ID, ns)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
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
	slug := names.RepoSlug(params.Org, params.Repo)
	app, err := h.store.GetApp(ctx, slug, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return &server.GetAppUtilisationNotFound{Error: server.ErrorDetail{
			Code:    "not_found",
			Message: "app not found",
			Remedy:  "check the app name and repo",
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	repo, err := h.store.GetRepo(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	tier, err := h.store.GetTier(ctx, repo.Tier)
	if err != nil {
		return nil, fmt.Errorf("get tier: %w", err)
	}

	prices := h.latestPrices(ctx)
	cost := h.appCostMonthly(ctx, app, tier, prices, time.Now().UTC())

	cpuCores := float64(tier.CpuMilli) / 1000.0
	memGB := float64(tier.MemoryMb) / 1024.0

	out := &server.GetAppUtilisationOK{
		CPUCores:            server.NewOptFloat64(cpuCores),
		MemoryGB:            server.NewOptFloat64(memGB),
		CostEstimateMonthly: server.NewOptFloat64(cost),
	}
	return out, nil
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
	go hibernateApp(context.WithoutCancel(ctx), h.store, h.deployer, h.plat.Namespace(), opID, app.ID, app.Type, params.Name, ns, host)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
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

	// Budget enforcement: block wake when a limit is active unless exempt or operator.
	if blocked, err := h.checkBudgetAndMaybeExempt(ctx, slug, params.Name); err != nil {
		return nil, err
	} else if blocked {
		return nil, &apiError{
			httpStatus: http.StatusServiceUnavailable,
			code:       "budget_soft_limit",
			message:    "platform is over budget for this period",
			remedy:     "contact your platform operator or wait for the next billing period",
		}
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
	go wakeApp(context.WithoutCancel(ctx), h.store, h.deployer, h.plat.Namespace(), opID, app.ID, app.Type, params.Name, ns, host)

	return &server.AcceptedOperationHeaders{
		Location:   operationLocation(params.Org, params.Repo, params.Name, opID),
		RetryAfter: server.NewOptInt(int(retryAfterDeploy.Seconds())),
		Response:   server.AcceptedOperation{OperationID: opID},
	}, nil
}

// checkBudgetAndMaybeExempt returns (true, nil) when the caller should be blocked,
// (false, nil) when wake may proceed, and (false, err) on internal error.
// Operators who wake during an active limit automatically receive a period exemption.
func (h *Handler) checkBudgetAndMaybeExempt(ctx context.Context, repoSlug, appName string) (blocked bool, _ error) {
	cfg, err := h.store.GetPlatformConfig(ctx)
	if err != nil {
		return false, fmt.Errorf("get platform config: %w", err)
	}
	if cfg.BudgetSoftLimitActive == 0 && cfg.BudgetHardLimitActive == 0 {
		return false, nil
	}

	exempt, err := h.store.IsAppExempt(ctx, repoSlug, appName)
	if err != nil {
		return false, fmt.Errorf("check exemption: %w", err)
	}
	if exempt {
		return false, nil
	}

	claims := claimsFromContext(ctx)
	if claims != nil && claims.Role == tokens.RoleOperator {
		// Operator override: grant a period exemption for the rest of the billing period.
		periodEnd := cost.NextBillingPeriodStart(time.Now())
		if err := h.store.AddPeriodExemption(ctx, repoSlug, appName, periodEnd); err != nil {
			return false, fmt.Errorf("add period exemption: %w", err)
		}
		return false, nil
	}

	return true, nil
}
