// Package hibernation runs a background loop that scales idle apps to zero and
// wakes them when traffic or queue activity resumes.
package hibernation

import (
	"context"
	"log/slog"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/names"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

const (
	defaultCheckInterval = 5 * time.Minute
	defaultIdleAfter     = 24 * time.Hour
	wakeTimeout          = 5 * time.Minute
)

// Deployer is the subset of kube.Client methods the watcher uses.
type Deployer interface {
	ScaleDeployment(ctx context.Context, namespace string, replicas int32) error
	SuspendCronJob(ctx context.Context, namespace string) error
	RouteToWakeProxy(ctx context.Context, namespace, host, gatewayNS, gatewayName string) error
	AppReplicaCounts(ctx context.Context, namespace, appType string) (desired, ready int32)
}

// Watcher checks all apps periodically and hibernates those that have been idle
// longer than their configured idle_after duration.
type Watcher struct {
	store    *store.Store
	deployer Deployer
	plat     platform.Platform
	interval time.Duration
}

// New constructs a Watcher. interval is how often to check; pass 0 to use the default (5 min).
func New(s *store.Store, deployer Deployer, plat platform.Platform, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = defaultCheckInterval
	}
	return &Watcher{store: s, deployer: deployer, plat: plat, interval: interval}
}

// Run starts the hibernation loop. It blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	logger := ctxlog.From(ctx).With("component", "hibernation-watcher")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.checkAll(ctx, logger)
		}
	}
}

func (w *Watcher) checkAll(ctx context.Context, logger *slog.Logger) {
	apps, err := w.store.ListAllApps(ctx)
	if err != nil {
		logger.Error("hibernation watcher: list apps", "err", err)
		return
	}

	now := time.Now()
	for _, app := range apps {
		if app.Hibernated != 0 {
			continue
		}
		idleAfter := defaultIdleAfter
		if app.IdleAfter.Valid && app.IdleAfter.String != "" {
			d, err := time.ParseDuration(app.IdleAfter.String)
			if err != nil {
				logger.Warn("hibernation watcher: invalid idle_after", "app", app.Name, "value", app.IdleAfter.String)
				continue
			}
			idleAfter = d
		}

		ns := ""
		if app.Namespace.Valid {
			ns = app.Namespace.String
		}

		switch app.Type {
		case "http":
			w.checkHTTP(ctx, logger, app.ID, app.Name, app.RepoSlug, app.Type, ns, now, idleAfter)
		case "worker":
			w.checkWorker(ctx, logger, app.ID, app.Name, app.RepoSlug, ns, now, idleAfter)
		}
		// cron apps are not hibernated based on idle activity
	}
}

func (w *Watcher) checkHTTP(ctx context.Context, logger *slog.Logger, appID int64, name, repoSlug, appType, namespace string, now time.Time, idleAfter time.Duration) {
	// Skip if the deployment has already been scaled to zero externally.
	desired, _ := w.deployer.AppReplicaCounts(ctx, namespace, appType)
	if desired == 0 {
		return
	}

	// Use last_active_at as the idle baseline; if never set, use a recent sentinel
	// so we don't hibernate apps that were just deployed.
	app, err := w.store.GetAppByNamespace(ctx, namespace)
	if err != nil {
		return
	}
	var lastActive time.Time
	if app.LastActiveAt.Valid {
		lastActive = app.LastActiveAt.Time
	} else {
		// No activity recorded yet; treat the app as active to avoid immediate hibernation.
		return
	}

	if now.Sub(lastActive) < idleAfter {
		return
	}

	logger.Info("hibernating idle http app", "app", name, "repo", repoSlug, "idle", now.Sub(lastActive))
	w.hibernateHTTP(ctx, logger, appID, name, repoSlug, namespace)
}

func (w *Watcher) hibernateHTTP(ctx context.Context, logger *slog.Logger, appID int64, name, repoSlug, namespace string) {
	host := names.AppHostname(name, names.RepoName(repoSlug), w.plat.BaseDomain())

	if err := w.deployer.RouteToWakeProxy(ctx, namespace, host, w.plat.Namespace(), kube.GatewayExternal); err != nil {
		logger.Error("hibernation: route to wake proxy", "app", name, "err", err)
		return
	}
	if err := w.deployer.ScaleDeployment(ctx, namespace, 0); err != nil {
		logger.Error("hibernation: scale to zero", "app", name, "err", err)
		return
	}
	if err := w.store.RecordScaleEvent(ctx, namespace, name, "scale_to_0"); err != nil {
		logger.Error("hibernation: record scale event", "app", name, "err", err)
	}
	if err := w.store.SetAppHibernated(ctx, appID, "idle"); err != nil {
		logger.Error("hibernation: set app hibernated", "app", name, "err", err)
	}
	if err := w.store.UpdateAppStatus(ctx, appID, "hibernated"); err != nil {
		logger.Error("hibernation: update app status", "app", name, "err", err)
	}
}

func (w *Watcher) checkWorker(ctx context.Context, logger *slog.Logger, appID int64, name, repoSlug, namespace string, now time.Time, idleAfter time.Duration) {
	q := w.plat.Queues(repoSlug, name)
	infos, err := q.IdleStatus(ctx, idleAfter)
	if err != nil {
		logger.Error("hibernation: queue idle status", "app", name, "err", err)
		return
	}

	// If any queue has received external activity within idleAfter, the app is not idle.
	for _, info := range infos {
		if !info.Idle {
			// Update last_active_at so the watcher window resets.
			_ = w.store.UpdateLastActiveAt(ctx, appID)
			return
		}
	}

	// All queues are idle; check the last_active_at timestamp.
	app, err := w.store.GetAppByNamespace(ctx, namespace)
	if err != nil {
		return
	}
	if app.LastActiveAt.Valid && now.Sub(app.LastActiveAt.Time) < idleAfter {
		return
	}

	logger.Info("hibernating idle worker app", "app", name, "repo", repoSlug)
	if err := w.deployer.ScaleDeployment(ctx, namespace, 0); err != nil {
		logger.Error("hibernation: scale to zero", "app", name, "err", err)
		return
	}
	if err := w.store.RecordScaleEvent(ctx, namespace, name, "scale_to_0"); err != nil {
		logger.Error("hibernation: record scale event", "app", name, "err", err)
	}
	if err := w.store.SetAppHibernated(ctx, appID, "idle"); err != nil {
		logger.Error("hibernation: set app hibernated", "app", name, "err", err)
	}
	if err := w.store.UpdateAppStatus(ctx, appID, "hibernated"); err != nil {
		logger.Error("hibernation: update app status", "app", name, "err", err)
	}
	_ = now // suppress unused warning
}
