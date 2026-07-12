package handler

import (
	"context"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
)

// hibernateApp scales the app to zero and records hibernation. Run in a goroutine.
func hibernateApp(ctx context.Context, s *store.Store, d AppDeployer, platNS string, opID string, appID int64, appType, appName, ns, host string) {
	logger := ctxlog.From(ctx).With("op", opID, "namespace", ns)
	if err := s.StartOperation(ctx, opID); err != nil {
		logger.Error("start hibernate operation", "err", err)
	}

	switch appType {
	case "http":
		if err := d.RouteToWakeProxy(ctx, ns, host, platNS, kube.GatewayExternal); err != nil {
			logger.Error("route to wake proxy", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
		if err := d.ScaleDeployment(ctx, ns, 0); err != nil {
			logger.Error("scale to zero", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
	case "worker":
		if err := d.ScaleDeployment(ctx, ns, 0); err != nil {
			logger.Error("scale to zero", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
	case "cron":
		if err := d.SuspendCronJob(ctx, ns); err != nil {
			logger.Error("suspend cron job", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
	}
	if err := s.RecordScaleEvent(ctx, ns, appName, "scale_to_0"); err != nil {
		logger.Warn("record scale event", "err", err)
	}
	if err := s.SetAppHibernated(ctx, appID, "manual"); err != nil {
		logger.Error("set app hibernated", "err", err)
	}
	if err := s.UpdateAppStatus(ctx, appID, "hibernated"); err != nil {
		logger.Warn("update app status", "err", err)
	}
	if err := s.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed hibernate operation", "err", err)
	}
}

// wakeApp brings an app back from hibernation. Run in a goroutine.
func wakeApp(ctx context.Context, s *store.Store, d AppDeployer, platNS string, opID string, appID int64, appType, appName, ns, host string) {
	logger := ctxlog.From(ctx).With("op", opID, "namespace", ns)
	if err := s.StartOperation(ctx, opID); err != nil {
		logger.Error("start wake operation", "err", err)
	}

	switch appType {
	case "http":
		if err := d.ScaleDeployment(ctx, ns, 1); err != nil {
			logger.Error("scale up", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
		if err := d.WatchDeploymentReady(ctx, ns, 5*time.Minute); err != nil {
			logger.Error("wait for ready", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
		if err := d.RestoreHTTPRoute(ctx, ns, host, platNS, kube.GatewayExternal, 8080); err != nil {
			logger.Error("restore http route", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
	case "worker":
		if err := d.ScaleDeployment(ctx, ns, 1); err != nil {
			logger.Error("scale up", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
	case "cron":
		if err := d.UnsuspendCronJob(ctx, ns); err != nil {
			logger.Error("unsuspend cron job", "err", err)
			if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
				logger.Warn("fail operation", "err", fErr)
			}
			return
		}
	}
	if err := s.RecordScaleEvent(ctx, ns, appName, "scale_to_1"); err != nil {
		logger.Warn("record scale event", "err", err)
	}
	if err := s.SetAppAwake(ctx, appID); err != nil {
		logger.Error("set app awake", "err", err)
	}
	if err := s.UpdateAppStatus(ctx, appID, "running"); err != nil {
		logger.Warn("update app status", "err", err)
	}
	if err := s.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed wake operation", "err", err)
	}
}

// deleteApp removes all Kubernetes resources for an app. Run in a goroutine.
func deleteApp(ctx context.Context, s *store.Store, d AppDeployer, opID string, appID int64, ns string) {
	logger := ctxlog.From(ctx).With("op", opID, "namespace", ns)
	if err := s.StartOperation(ctx, opID); err != nil {
		logger.Error("start delete operation", "err", err)
	}
	if err := d.Delete(ctx, ns); err != nil {
		logger.Error("delete kubernetes resources", "err", err)
		if fErr := s.FailOperation(ctx, opID, err.Error()); fErr != nil {
			logger.Warn("fail operation", "err", fErr)
		}
		return
	}
	if err := s.UpdateAppStatus(ctx, appID, "deleted"); err != nil {
		logger.Warn("update app status", "err", err)
	}
	if err := s.SucceedOperation(ctx, opID); err != nil {
		logger.Error("succeed delete operation", "err", err)
	}
}
