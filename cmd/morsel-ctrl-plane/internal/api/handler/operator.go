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
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// ── Operator stubs ────────────────────────────────────────────────────────────

func (h *Handler) GetOperatorConfig(ctx context.Context) (server.GetOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpdateOperatorConfig(ctx context.Context, _ *server.PlatformConfig) (server.UpdateOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) UpdateRepoTier(ctx context.Context, req *server.UpdateRepoTierReq, params server.UpdateRepoTierParams) (server.UpdateRepoTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)

	repo, err := h.store.GetRepo(ctx, slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "not_found",
			message:    "repo not found",
			remedy:     "check the org and repo name",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}

	newTier, err := h.store.GetTier(ctx, req.Tier)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "tier_not_found",
			message:    fmt.Sprintf("tier %q not found", req.Tier),
			remedy:     "check the tier name with: morsel operator tier list",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get tier: %w", err)
	}

	// Demotion guard: reject if current app count exceeds the target tier's max.
	if newTier.MaxApps < h.currentMaxAppsFor(ctx, slug, repo.Tier) {
		count, countErr := h.store.CountAppsByRepo(ctx, slug)
		if countErr == nil && count > newTier.MaxApps {
			return nil, &apiError{
				httpStatus: http.StatusConflict,
				code:       "tier_demotion_blocked",
				message:    fmt.Sprintf("repo has %d apps; tier %q allows %d", count, req.Tier, newTier.MaxApps),
				remedy:     "delete apps to meet the lower tier limit before demoting",
			}
		}
	}

	updated, err := h.store.SetRepoTier(ctx, slug, req.Tier)
	if err != nil {
		return nil, fmt.Errorf("set repo tier: %w", err)
	}

	// Propagate new quota limits to all app namespaces in this repo.
	limits := kube.TierLimits{CPUMilli: int(newTier.CpuMilli), MemoryMB: int(newTier.MemoryMb)}
	apps, _ := h.store.ListApps(ctx, slug)
	for _, app := range apps {
		if app.Namespace.Valid && app.Namespace.String != "" {
			_ = h.deployer.ApplyNamespaceTier(ctx, app.Namespace.String, limits)
		}
	}

	count, _ := h.store.CountAppsByRepo(ctx, slug)
	prices := h.latestPrices(ctx)
	cost := h.repoCostMonthly(ctx, slug, prices, time.Now().UTC())
	out := dbRepoToOAS(updated, count, cost)
	return &out, nil
}

// currentMaxAppsFor returns the max_apps of the repo's current tier, or a
// large number if the tier can't be looked up (safe for demotion guard).
func (h *Handler) currentMaxAppsFor(ctx context.Context, _ string, tierName string) int64 {
	cur, err := h.store.GetTier(ctx, tierName)
	if err != nil {
		return 1<<62 - 1
	}
	return cur.MaxApps
}

func (h *Handler) ListOperatorApprovals(ctx context.Context) (server.ListOperatorApprovalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorApproval(ctx context.Context, _ server.GetOperatorApprovalParams) (server.GetOperatorApprovalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) BatchActionApprovals(ctx context.Context, _ *server.BatchActionApprovalsReq) (server.BatchActionApprovalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return nil, errNotImplemented
}

func (h *Handler) GetOperatorCost(ctx context.Context) (server.GetOperatorCostRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}

	prices := h.latestPrices(ctx)
	now := time.Now().UTC()

	repos, err := h.store.ListRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("list repos: %w", err)
	}

	byRepo := make([]server.GetOperatorCostOKByRepoItem, 0, len(repos))
	var total float64
	for _, repo := range repos {
		cost := h.repoCostMonthly(ctx, repo.Slug, prices, now)
		total += cost
		byRepo = append(byRepo, server.GetOperatorCostOKByRepoItem{
			Slug:             repo.Slug,
			EstimatedMonthly: cost,
		})
	}

	out := &server.GetOperatorCostOK{
		EstimatedMonthly: server.NewOptFloat64(total),
		ByRepo:           byRepo,
	}
	if !prices.FetchedAt.IsZero() {
		out.PricesFetchedAt = server.NewOptDateTime(prices.FetchedAt)
	}
	return out, nil
}

func (h *Handler) GetOperatorPricesHistory(ctx context.Context) (server.GetOperatorPricesHistoryRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}

	snapshots, err := h.store.ListPriceSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list price snapshots: %w", err)
	}

	items := make([]server.GetOperatorPricesHistoryOKSnapshotsItem, len(snapshots))
	for i, s := range snapshots {
		items[i] = server.GetOperatorPricesHistoryOKSnapshotsItem{
			FetchedAt:             s.FetchedAt,
			ComputeCPUPerCoreHour: s.ComputeCpuPerCoreHour,
			ComputeMemPerGBHour:   s.ComputeMemPerGbHour,
			StoragePerGBMonth:     s.StoragePerGbMonth,
			RegistryPerGBMonth:    s.RegistryPerGbMonth,
		}
	}
	return &server.GetOperatorPricesHistoryOK{Snapshots: items}, nil
}

func (h *Handler) GetDeploymentInfo(ctx context.Context) (server.GetDeploymentInfoRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	return &server.DeploymentInfo{Namespace: h.plat.Namespace(), Platform: h.plat.Name()}, nil
}

func (h *Handler) GetOperatorStatus(ctx context.Context) (server.GetOperatorStatusRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}

	expiry, err := h.deployer.GetTLSCertExpiry(ctx, h.plat.Namespace(), kube.MorselTLSSecret)
	if err != nil {
		return nil, fmt.Errorf("get tls cert expiry: %w", err)
	}

	resp := &server.GetOperatorStatusOK{}
	if expiry != nil && time.Until(*expiry) < 30*24*time.Hour {
		resp.Certs = server.NewOptGetOperatorStatusOKCerts(server.GetOperatorStatusOKCerts{
			ExpiringSoon: []string{"*." + h.plat.BaseDomain()},
		})
	}

	snap, snapErr := h.store.GetLatestPriceSnapshot(ctx)
	if snapErr != nil || time.Since(snap.FetchedAt) > 48*time.Hour {
		resp.PricesStale = server.NewOptBool(true)
	}

	return resp, nil
}

// ── Operator principals ───────────────────────────────────────────────────────

func (h *Handler) ListOperatorPrincipals(ctx context.Context) (server.ListOperatorPrincipalsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	principals, err := h.store.ListPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &server.OperatorPrincipals{Principals: principals}, nil
}

func (h *Handler) AddOperatorPrincipal(ctx context.Context, req *server.PrincipalReq) (server.AddOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.AddPrincipal(ctx, req.Principal); err != nil {
		return nil, fmt.Errorf("add principal: %w", err)
	}
	principals, err := h.store.ListPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &server.OperatorPrincipals{Principals: principals}, nil
}

func (h *Handler) RemoveOperatorPrincipal(ctx context.Context, params server.RemoveOperatorPrincipalParams) (server.RemoveOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.RemovePrincipal(ctx, params.Principal); err != nil {
		return nil, fmt.Errorf("remove principal: %w", err)
	}
	principals, err := h.store.ListPrincipals(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	return &server.OperatorPrincipals{Principals: principals}, nil
}
