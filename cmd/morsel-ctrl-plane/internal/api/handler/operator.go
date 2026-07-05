package handler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/api/server"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// ── Platform config ───────────────────────────────────────────────────────────

func (h *Handler) GetOperatorConfig(ctx context.Context) (server.GetOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	cfg, err := h.store.GetPlatformConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("get platform config: %w", err)
	}
	return platformConfigToOAS(cfg), nil
}

func (h *Handler) UpdateOperatorConfig(ctx context.Context, req *server.PlatformConfig) (server.UpdateOperatorConfigRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	updated, err := h.store.PatchPlatformConfig(ctx,
		req.BudgetCeilingMonthly.Or(0),
		req.SoftLimitPct.Or(0),
		req.HardLimitPct.Or(0),
		req.DefaultIdleAfter.Or(""),
	)
	if err != nil {
		return nil, fmt.Errorf("update platform config: %w", err)
	}
	return platformConfigToOAS(updated), nil
}

func platformConfigToOAS(cfg store.PlatformConfig) *server.PlatformConfig {
	return &server.PlatformConfig{
		BudgetCeilingMonthly: server.NewOptFloat64(cfg.BudgetCeilingMonthly),
		SoftLimitPct:         server.NewOptFloat64(cfg.SoftLimitPct),
		HardLimitPct:         server.NewOptFloat64(cfg.HardLimitPct),
		DefaultIdleAfter:     server.NewOptString(cfg.DefaultIdleAfter),
	}
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
	return h.listPrincipalsResponse(ctx)
}

func (h *Handler) AddOperatorPrincipal(ctx context.Context, req *server.PrincipalReq) (server.AddOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.AddPrincipal(ctx, req.Principal); err != nil {
		return nil, fmt.Errorf("add principal: %w", err)
	}
	return h.listPrincipalsResponse(ctx)
}

func (h *Handler) RemoveOperatorPrincipal(ctx context.Context, params server.RemoveOperatorPrincipalParams) (server.RemoveOperatorPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.RemovePrincipal(ctx, params.Principal); err != nil {
		return nil, fmt.Errorf("remove principal: %w", err)
	}
	return h.listPrincipalsResponse(ctx)
}

func (h *Handler) RequirePasswordResetForPrincipal(ctx context.Context, params server.RequirePasswordResetForPrincipalParams) (server.RequirePasswordResetForPrincipalRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	exists, err := h.store.PrincipalExists(ctx, params.Principal)
	if err != nil {
		return nil, fmt.Errorf("check principal: %w", err)
	}
	if !exists {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "not_found",
			message:    fmt.Sprintf("principal %q not found", params.Principal),
			remedy:     "check the principal identity with: morsel operator principal list",
		}
	}
	if err := h.store.SetPasswordResetRequired(ctx, params.Principal, true); err != nil {
		return nil, fmt.Errorf("set password reset required: %w", err)
	}
	return &server.RequirePasswordResetForPrincipalNoContent{}, nil
}

func (h *Handler) ResetOperatorPassword(ctx context.Context, req *server.ChangePasswordReq) (server.ResetOperatorPasswordRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	claims := claimsFromContext(ctx)
	if claims == nil {
		return nil, &apiError{
			httpStatus: http.StatusUnauthorized,
			code:       "invalid_token",
			message:    "could not identify operator from token",
			remedy:     "re-authenticate to obtain a fresh token",
		}
	}

	storedHash, err := h.store.GetPrincipalPasswordHash(ctx, claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("get password hash: %w", err)
	}
	if !storedHash.Valid || storedHash.String == "" {
		return nil, &apiError{
			httpStatus: http.StatusUnprocessableEntity,
			code:       "no_password_set",
			message:    "no password is set for this principal",
			remedy:     "ask an admin to set a new password for your account",
		}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash.String), []byte(req.CurrentPassword)); err != nil {
		return nil, &apiError{
			httpStatus: http.StatusUnprocessableEntity,
			code:       "invalid_current_password",
			message:    "current password is incorrect",
			remedy:     "provide the correct current password",
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := h.store.AddPrincipalWithPasswordHash(ctx, claims.Subject, string(newHash)); err != nil {
		return nil, fmt.Errorf("set password: %w", err)
	}
	if err := h.store.SetPasswordChangedAt(ctx, claims.Subject, time.Now()); err != nil {
		return nil, fmt.Errorf("record password change: %w", err)
	}
	return &server.ResetOperatorPasswordNoContent{}, nil
}

func (h *Handler) SetOperatorPrincipalPassword(ctx context.Context, req *server.SetPrincipalPasswordReq, params server.SetOperatorPrincipalPasswordParams) (server.SetOperatorPrincipalPasswordRes, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	exists, err := h.store.PrincipalExists(ctx, params.Principal)
	if err != nil {
		return nil, fmt.Errorf("check principal: %w", err)
	}
	if !exists {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "not_found",
			message:    fmt.Sprintf("principal %q not found", params.Principal),
			remedy:     "check the principal identity with: morsel operator principal list",
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	if err := h.store.AddPrincipalWithPasswordHash(ctx, params.Principal, string(hash)); err != nil {
		return nil, fmt.Errorf("set password: %w", err)
	}
	now := time.Now()
	if err := h.store.SetPasswordChangedAt(ctx, params.Principal, now); err != nil {
		return nil, fmt.Errorf("record password change: %w", err)
	}
	if req.Invalidate.Or(false) {
		if err := h.store.SetPasswordResetRequired(ctx, params.Principal, true); err != nil {
			return nil, fmt.Errorf("set password reset required: %w", err)
		}
	}
	return &server.SetOperatorPrincipalPasswordNoContent{}, nil
}

func (h *Handler) InvalidateOperatorPrincipalPassword(ctx context.Context, params server.InvalidateOperatorPrincipalPasswordParams) (server.InvalidateOperatorPrincipalPasswordRes, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	exists, err := h.store.PrincipalExists(ctx, params.Principal)
	if err != nil {
		return nil, fmt.Errorf("check principal: %w", err)
	}
	if !exists {
		return nil, &apiError{
			httpStatus: http.StatusNotFound,
			code:       "not_found",
			message:    fmt.Sprintf("principal %q not found", params.Principal),
			remedy:     "check the principal identity with: morsel operator principal list",
		}
	}
	if err := h.store.InvalidatePrincipalPassword(ctx, params.Principal, time.Now()); err != nil {
		return nil, fmt.Errorf("invalidate password: %w", err)
	}
	return &server.InvalidateOperatorPrincipalPasswordNoContent{}, nil
}

// listPrincipalsResponse returns an OperatorPrincipals response populated with details.
func (h *Handler) listPrincipalsResponse(ctx context.Context) (*server.OperatorPrincipals, error) {
	details, err := h.store.ListPrincipalsWithDetails(ctx)
	if err != nil {
		return nil, fmt.Errorf("read principals: %w", err)
	}
	items := make([]server.OperatorPrincipalDetail, len(details))
	for i, d := range details {
		items[i] = server.OperatorPrincipalDetail{
			Username:              d.Username,
			PasswordResetRequired: d.PasswordResetRequired,
			IsAdmin:               d.IsAdmin,
		}
	}
	return &server.OperatorPrincipals{Principals: items}, nil
}

// ── Exemptions ────────────────────────────────────────────────────────────────

func (h *Handler) AddAppExemption(ctx context.Context, req *server.AppExemptionReq) (server.AddAppExemptionRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.AddAppExemption(ctx, req.RepoSlug, req.AppName); err != nil {
		return nil, fmt.Errorf("add app exemption: %w", err)
	}
	return &server.AddAppExemptionNoContent{}, nil
}

func (h *Handler) RemoveAppExemption(ctx context.Context, params server.RemoveAppExemptionParams) (server.RemoveAppExemptionRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	if err := h.store.RemoveAppExemption(ctx, slug, params.Name); err != nil {
		return nil, fmt.Errorf("remove app exemption: %w", err)
	}
	return &server.RemoveAppExemptionNoContent{}, nil
}

func (h *Handler) AddRepoExemption(ctx context.Context, req *server.RepoExemptionReq) (server.AddRepoExemptionRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	if err := h.store.AddRepoExemption(ctx, req.RepoSlug); err != nil {
		return nil, fmt.Errorf("add repo exemption: %w", err)
	}
	return &server.AddRepoExemptionNoContent{}, nil
}

func (h *Handler) RemoveRepoExemption(ctx context.Context, params server.RemoveRepoExemptionParams) (server.RemoveRepoExemptionRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	slug := names.RepoSlug(params.Org, params.Repo)
	if err := h.store.RemoveRepoExemption(ctx, slug); err != nil {
		return nil, fmt.Errorf("remove repo exemption: %w", err)
	}
	return &server.RemoveRepoExemptionNoContent{}, nil
}

func (h *Handler) ListExemptions(ctx context.Context) (server.ListExemptionsRes, error) {
	if err := requireOperator(ctx); err != nil {
		return nil, err
	}
	exemptions, err := h.store.ListExemptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list exemptions: %w", err)
	}
	items := make([]server.ExemptionListExemptionsItem, len(exemptions))
	for i, ex := range exemptions {
		item := server.ExemptionListExemptionsItem{
			Kind:     ex.Kind,
			RepoSlug: ex.RepoSlug,
			Type:     ex.Type,
		}
		if ex.AppName != "" {
			item.AppName = server.NewOptString(ex.AppName)
		}
		if ex.ExpiresAt.Valid {
			item.ExpiresAt = server.NewOptDateTime(ex.ExpiresAt.Time)
		}
		items[i] = item
	}
	return &server.ExemptionList{Exemptions: items}, nil
}
