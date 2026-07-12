package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// ── Tier handlers ─────────────────────────────────────────────────────────────

func (h *Handler) ListTiers(ctx context.Context) (server.ListTiersRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	tiers, err := h.store.ListTiers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tiers: %w", err)
	}
	out := make(server.ListTiersOKApplicationJSON, len(tiers))
	for i, t := range tiers {
		out[i] = dbTierToOAS(t)
	}
	return &out, nil
}

func (h *Handler) CreateTier(ctx context.Context, req *server.CreateTierReq) (server.CreateTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	t, err := h.store.InsertTier(ctx,
		req.Name,
		int64(req.MaxApps),
		int64(req.CPUMilli),
		int64(req.MemoryMB),
		int64(req.BlobGB),
		int64(req.DatabaseGB),
		int64(req.QueueGB),
		req.HibernateAfter,
	)
	if err != nil {
		// SQLite UNIQUE constraint violation on the name primary key.
		if isUniqueViolation(err) {
			return nil, &apiError{
				httpStatus: http.StatusConflict,
				code:       "tier_exists",
				message:    fmt.Sprintf("tier %q already exists", req.Name),
				remedy:     "choose a different name or edit the existing tier",
			}
		}
		return nil, fmt.Errorf("create tier: %w", err)
	}
	out := dbTierToOAS(t)
	return &out, nil
}

func (h *Handler) UpdateTier(ctx context.Context, req *server.UpdateTierReq, params server.UpdateTierParams) (server.UpdateTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	t, err := h.store.PatchTier(ctx, params.Name,
		int64(req.MaxApps.Or(0)),
		int64(req.CPUMilli.Or(0)),
		int64(req.MemoryMB.Or(0)),
		int64(req.BlobGB.Or(0)),
		int64(req.DatabaseGB.Or(0)),
		int64(req.QueueGB.Or(0)),
		req.HibernateAfter.Or(""),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "tier_not_found",
			message:    fmt.Sprintf("tier %q not found", params.Name),
			remedy:     "check the tier name with: morsel operator tier list",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("update tier: %w", err)
	}

	// Propagate ResourceQuota and LimitRange changes to all affected namespaces.
	go h.propagateTierToNamespaces(context.WithoutCancel(ctx), t.Name,
		kube.TierLimits{CPUMilli: int(t.CpuMilli), MemoryMB: int(t.MemoryMb)})

	out := dbTierToOAS(t)
	return &out, nil
}

func (h *Handler) DeleteTier(ctx context.Context, params server.DeleteTierParams) (server.DeleteTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	t, err := h.store.GetTier(ctx, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "tier_not_found",
			message:    fmt.Sprintf("tier %q not found", params.Name),
			remedy:     "check the tier name with: morsel operator tier list",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get tier: %w", err)
	}
	if t.IsDefault != 0 {
		return nil, &apiError{
			httpStatus: http.StatusConflict,
			code:       "tier_is_default",
			message:    fmt.Sprintf("tier %q is the platform default and cannot be deleted", params.Name),
			remedy:     "set a different default first with: morsel operator tier set-default --name <other-tier>",
		}
	}
	count, err := h.store.CountReposByTier(ctx, params.Name)
	if err != nil {
		return nil, fmt.Errorf("count repos: %w", err)
	}
	if count > 0 {
		return nil, &apiError{
			httpStatus: http.StatusConflict,
			code:       "tier_in_use",
			message:    fmt.Sprintf("tier %q is assigned to %d repo(s) and cannot be deleted", params.Name, count),
			remedy:     "reassign all repos to another tier first",
		}
	}
	if err := h.store.DeleteTier(ctx, params.Name); err != nil {
		return nil, fmt.Errorf("delete tier: %w", err)
	}
	return &server.DeleteTierNoContent{}, nil
}

func (h *Handler) SetDefaultTier(ctx context.Context, params server.SetDefaultTierParams) (server.SetDefaultTierRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	_, err := h.store.GetTier(ctx, params.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "tier_not_found",
			message:    fmt.Sprintf("tier %q not found", params.Name),
			remedy:     "check the tier name with: morsel operator tier list",
		}
	}
	if err != nil {
		return nil, fmt.Errorf("get tier: %w", err)
	}
	if err := h.store.SetDefaultTier(ctx, params.Name); err != nil {
		return nil, fmt.Errorf("set default tier: %w", err)
	}
	t, err := h.store.GetTier(ctx, params.Name)
	if err != nil {
		return nil, fmt.Errorf("get updated tier: %w", err)
	}
	out := dbTierToOAS(t)
	return &out, nil
}

// propagateTierToNamespaces updates ResourceQuota and LimitRange in all app
// namespaces belonging to repos assigned to the given tier.
func (h *Handler) propagateTierToNamespaces(ctx context.Context, tierName string, limits kube.TierLimits) {
	logger := ctxlog.From(ctx).With("tier", tierName)
	repos, err := h.store.ListReposByTier(ctx, tierName)
	if err != nil {
		logger.Error("list repos by tier", "err", err)
		return
	}
	for _, repo := range repos {
		apps, err := h.store.ListApps(ctx, repo.Slug)
		if err != nil {
			logger.Error("list apps for tier propagation", "repo", repo.Slug, "err", err)
			continue
		}
		for _, app := range apps {
			if !app.Namespace.Valid || app.Namespace.String == "" {
				continue
			}
			if err := h.deployer.ApplyNamespaceTier(ctx, app.Namespace.String, limits); err != nil {
				logger.Warn("apply namespace tier", "app", app.Name, "err", err)
			}
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func dbTierToOAS(t store.Tier) server.Tier {
	out := server.Tier{
		Name:           t.Name,
		MaxApps:        int(t.MaxApps),
		CPUMilli:       int(t.CpuMilli),
		MemoryMB:       int(t.MemoryMb),
		BlobGB:         int(t.BlobGb),
		DatabaseGB:     int(t.DatabaseGb),
		QueueGB:        int(t.QueueGb),
		HibernateAfter: t.HibernateAfter,
		IsDefault:      t.IsDefault != 0,
		CreatedAt:      server.NewOptDateTime(t.CreatedAt),
		UpdatedAt:      server.NewOptDateTime(t.UpdatedAt),
	}
	return out
}

// isUniqueViolation returns true when err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
