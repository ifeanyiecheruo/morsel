# Feature 06 — Kubernetes Manifest Apply

_Delivers: upserted apps actually run as pods in the cluster._

**Direct dependencies:** [F05](005-feature-app-lifecycle-api.md)

## Tasks

- [ ] `client-go` integration — kubeconfig detection (in-cluster via service account; local via `~/.kube/config`)
- [ ] Namespace create-or-ensure
- [ ] `ResourceQuota` + `LimitRange` apply (hardcoded `small` tier defaults until F14)
- [ ] `NetworkPolicy` apply — allow ingress from load balancer and other app pods; deny cross-sidecar access
- [ ] `ServiceAccount` apply
- [ ] `Deployment` apply for `type: http` and `type: worker`
- [ ] `CronJob` apply for `type: cronjob`
- [ ] Rollout watch — poll deployment rollout via `client-go`; timeout from `health_check.timeout`
- [ ] Rollback on failed rollout — re-apply `last-healthy` image digest
- [ ] Image digest tracking in SQLite (`current`, `last-healthy` per app)
- [ ] `GET /api/repos/:slug/apps/:name/status` — reflect actual Kubernetes pod state
