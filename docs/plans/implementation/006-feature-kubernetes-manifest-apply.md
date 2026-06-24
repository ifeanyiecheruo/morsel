# Feature 06 — Kubernetes Manifest Apply

_Delivers: upserted apps actually run as pods in the cluster._

**Direct dependencies:** [F05](005-feature-app-lifecycle-api.md)

## Tasks

- [x] `client-go` integration — kubeconfig detection (in-cluster via service account; local via `~/.kube/config`)
- [x] Namespace create-or-ensure
- [x] `ResourceQuota` + `LimitRange` apply (hardcoded `small` tier defaults until F14)
- [x] `NetworkPolicy` apply — allow ingress from load balancer and other app pods; deny cross-sidecar access
- [x] `ServiceAccount` apply
- [x] `Deployment` apply for `type: http` and `type: worker`
- [x] `CronJob` apply for `type: cronjob`
- [x] Rollout watch — poll deployment rollout via `client-go`; timeout from `health_check.timeout`
- [x] Rollback on failed rollout — re-apply `last-healthy` image digest
- [x] Image digest tracking in SQLite (`current`, `last-healthy` per app)
- [x] `GET /api/repos/:slug/apps/:name/status` — reflect actual Kubernetes pod state
