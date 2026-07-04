// Package watchers contains background loop services that run alongside the
// control-plane API: budget enforcement and idle-hibernation.
package watchers

import (
	"context"
	"log/slog"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/cost"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

const defaultBudgetCheckInterval = 5 * time.Minute

// BudgetDeployer is the subset of kube.Client methods the budget watcher uses.
type BudgetDeployer interface {
	ScaleDeployment(ctx context.Context, namespace string, replicas int32) error
	SuspendCronJob(ctx context.Context, namespace string) error
	RouteToWakeProxy(ctx context.Context, namespace, host, gatewayNS, gatewayName string) error
}

// Budget evaluates estimated spend on each tick and enforces budget limits.
type Budget struct {
	store    *store.Store
	deployer BudgetDeployer
	plat     platform.Platform
	interval time.Duration
}

// NewBudget constructs a Budget watcher. Pass interval=0 to use the default (5 min).
func NewBudget(s *store.Store, deployer BudgetDeployer, plat platform.Platform, interval time.Duration) *Budget {
	if interval <= 0 {
		interval = defaultBudgetCheckInterval
	}
	return &Budget{store: s, deployer: deployer, plat: plat, interval: interval}
}

// Run starts the budget enforcement loop. It blocks until ctx is cancelled.
func (w *Budget) Run(ctx context.Context) {
	logger := ctxlog.From(ctx).With("component", "budget-watcher")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx, logger)
		}
	}
}

func (w *Budget) tick(ctx context.Context, logger *slog.Logger) {
	cfg, err := w.store.GetPlatformConfig(ctx)
	if err != nil {
		logger.Error("budget watcher: get platform config", "err", err)
		return
	}

	now := time.Now().UTC()
	currentPeriod := cost.BillingPeriodKey(now)

	// Billing period rollover: clear enforcement flags and expire period exemptions.
	if cfg.BillingPeriod != "" && cfg.BillingPeriod != currentPeriod {
		if err := w.store.ExpirePeriodExemptions(ctx, now); err != nil {
			logger.Error("budget watcher: expire period exemptions", "err", err)
		}
		cfg.BudgetSoftLimitActive = 0
		cfg.BudgetHardLimitActive = 0
	}
	if cfg.BillingPeriod != currentPeriod {
		cfg.BillingPeriod = currentPeriod
		if cfg, err = w.store.UpdatePlatformConfig(ctx, cfg); err != nil {
			logger.Error("budget watcher: update billing period", "err", err)
			return
		}
	}

	// Compute total estimated cost across all repos.
	total := w.totalCost(ctx, now)

	ceiling := cfg.BudgetCeilingMonthly
	softThreshold := ceiling * cfg.SoftLimitPct
	hardThreshold := ceiling * cfg.HardLimitPct

	prevSoft := cfg.BudgetSoftLimitActive
	prevHard := cfg.BudgetHardLimitActive

	switch {
	case total >= hardThreshold:
		cfg.BudgetSoftLimitActive = 1
		cfg.BudgetHardLimitActive = 1
	case total >= softThreshold:
		cfg.BudgetSoftLimitActive = 1
		cfg.BudgetHardLimitActive = 0
	default:
		cfg.BudgetSoftLimitActive = 0
		cfg.BudgetHardLimitActive = 0
	}

	// Force-hibernate non-exempt running apps when the hard limit is newly hit.
	if cfg.BudgetHardLimitActive != 0 && prevHard == 0 {
		logger.Warn("budget hard limit hit; force-hibernating non-exempt apps",
			"total_usd", total, "ceiling_usd", ceiling)
		w.forceHibernateAll(ctx, logger)
	} else if cfg.BudgetSoftLimitActive != prevSoft || cfg.BudgetHardLimitActive != prevHard {
		logger.Info("budget limit state changed",
			"soft", cfg.BudgetSoftLimitActive, "hard", cfg.BudgetHardLimitActive,
			"total_usd", total, "ceiling_usd", ceiling)
	}

	if cfg.BudgetSoftLimitActive != prevSoft || cfg.BudgetHardLimitActive != prevHard {
		if _, err := w.store.UpdatePlatformConfig(ctx, cfg); err != nil {
			logger.Error("budget watcher: persist limit flags", "err", err)
		}
	}
}

func (w *Budget) totalCost(ctx context.Context, now time.Time) float64 {
	prices, err := w.store.GetLatestPriceSnapshot(ctx)
	if err != nil {
		return 0
	}

	repos, err := w.store.ListRepos(ctx)
	if err != nil {
		return 0
	}

	periodStart := cost.BillingPeriodStart(now)
	var total float64
	for _, repo := range repos {
		total += w.repoCost(ctx, repo, prices, periodStart, now)
	}
	return total
}

func (w *Budget) repoCost(ctx context.Context, repo store.Repo, prices store.PriceSnapshot, periodStart, now time.Time) float64 {
	tier, err := w.store.GetTier(ctx, repo.Tier)
	if err != nil {
		return 0
	}
	apps, err := w.store.ListApps(ctx, repo.Slug)
	if err != nil {
		return 0
	}
	var total float64
	for _, app := range apps {
		if !app.Namespace.Valid || app.Namespace.String == "" {
			continue
		}
		events, err := w.store.ListScaleEventsSince(ctx, app.Namespace.String, app.Name, periodStart)
		if err != nil {
			continue
		}
		hours := cost.RunningHours(events, periodStart, now, app.Hibernated != 0)
		total += cost.EstimateAppCostMonthly(tier, hours, prices)
	}
	return total
}

func (w *Budget) forceHibernateAll(ctx context.Context, logger *slog.Logger) {
	apps, err := w.store.ListAllApps(ctx)
	if err != nil {
		logger.Error("budget watcher: list apps for force-hibernate", "err", err)
		return
	}

	for _, app := range apps {
		if app.Hibernated != 0 || !app.Namespace.Valid {
			continue
		}
		exempt, err := w.store.IsAppExempt(ctx, app.RepoSlug, app.Name)
		if err != nil || exempt {
			continue
		}

		ns := app.Namespace.String
		logger.Info("force-hibernating app (hard budget limit)", "app", app.Name, "repo", app.RepoSlug)

		switch app.Type {
		case "http":
			host := names.AppHostname(app.Name, names.RepoName(app.RepoSlug), w.plat.BaseDomain())
			if err := w.deployer.RouteToWakeProxy(ctx, ns, host, w.plat.Namespace(), kube.GatewayExternal); err != nil {
				logger.Error("budget watcher: route to wake proxy", "app", app.Name, "err", err)
				continue
			}
			if err := w.deployer.ScaleDeployment(ctx, ns, 0); err != nil {
				logger.Error("budget watcher: scale to zero", "app", app.Name, "err", err)
				continue
			}
		case "worker":
			if err := w.deployer.ScaleDeployment(ctx, ns, 0); err != nil {
				logger.Error("budget watcher: scale to zero", "app", app.Name, "err", err)
				continue
			}
		case "cron":
			if err := w.deployer.SuspendCronJob(ctx, ns); err != nil {
				logger.Error("budget watcher: suspend cron", "app", app.Name, "err", err)
				continue
			}
		}

		_ = w.store.RecordScaleEvent(ctx, ns, app.Name, "scale_to_0")
		_ = w.store.SetAppHibernated(ctx, app.ID, "budget")
		_ = w.store.UpdateAppStatus(ctx, app.ID, "hibernated")
	}
}
