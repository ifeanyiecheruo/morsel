package watchers

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
	defaultHibernationCheckInterval = 5 * time.Minute
	defaultIdleAfter                = 24 * time.Hour
	wakeTimeout                     = 5 * time.Minute
)

// HibernationDeployer is the subset of kube.Client methods the hibernation watcher uses.
type HibernationDeployer interface {
	ScaleDeployment(ctx context.Context, namespace string, replicas int32) error
	SuspendCronJob(ctx context.Context, namespace string) error
	RouteToWakeProxy(ctx context.Context, namespace, host, gatewayNS, gatewayName string) error
	AppReplicaCounts(ctx context.Context, namespace, appType string) (desired, ready int32)
}

// Hibernation checks all apps periodically and hibernates those that have been
// idle longer than their configured idle_after duration.
type Hibernation struct {
	store    *store.Store
	deployer HibernationDeployer
	plat     platform.Platform
	interval time.Duration
}

// NewHibernation constructs a Hibernation watcher. Pass interval=0 to use the default (5 min).
func NewHibernation(s *store.Store, deployer HibernationDeployer, plat platform.Platform, interval time.Duration) *Hibernation {
	if interval <= 0 {
		interval = defaultHibernationCheckInterval
	}
	return &Hibernation{store: s, deployer: deployer, plat: plat, interval: interval}
}

// Run starts the hibernation loop. It blocks until ctx is cancelled.
func (w *Hibernation) Run(ctx context.Context) {
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

func (w *Hibernation) checkAll(ctx context.Context, logger *slog.Logger) {
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

func (w *Hibernation) checkHTTP(ctx context.Context, logger *slog.Logger, appID int64, name, repoSlug, appType, namespace string, now time.Time, idleAfter time.Duration) {
	desired, _ := w.deployer.AppReplicaCounts(ctx, namespace, appType)
	if desired == 0 {
		return
	}

	app, err := w.store.GetAppByNamespace(ctx, namespace)
	if err != nil {
		return
	}
	var lastActive time.Time
	if app.LastActiveAt.Valid {
		lastActive = app.LastActiveAt.Time
	} else {
		return
	}

	if now.Sub(lastActive) < idleAfter {
		return
	}

	logger.Info("hibernating idle http app", "app", name, "repo", repoSlug, "idle", now.Sub(lastActive))
	w.hibernateHTTP(ctx, logger, appID, name, repoSlug, namespace)
}

func (w *Hibernation) hibernateHTTP(ctx context.Context, logger *slog.Logger, appID int64, name, repoSlug, namespace string) {
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

func (w *Hibernation) checkWorker(ctx context.Context, logger *slog.Logger, appID int64, name, repoSlug, namespace string, now time.Time, idleAfter time.Duration) {
	q := w.plat.Queues(repoSlug, name)
	infos, err := q.IdleStatus(ctx, idleAfter)
	if err != nil {
		logger.Error("hibernation: queue idle status", "app", name, "err", err)
		return
	}

	for _, info := range infos {
		if !info.Idle {
			_ = w.store.UpdateLastActiveAt(ctx, appID)
			return
		}
	}

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
}
