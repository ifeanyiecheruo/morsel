# Feature 14 — Quota Tiers

_Delivers: operators control resource limits per repo; Kubernetes enforces them._

**Direct dependencies:** [F05](005-feature-app-lifecycle-api.md), [F06](006-feature-kubernetes-manifest-apply.md)

> Can be developed in parallel with F07, F08, F09, F10, F11. F15 (Approvals) blocks on this.

## Tasks

- [ ] `tiers` table in SQLite; seed built-in `small`, `medium`, `large` on migration
- [ ] `GET /api/operator/tiers` — list all tiers
- [ ] `POST /api/operator/tiers` — create tier
- [ ] `PATCH /api/operator/tiers/:name` — edit tier; propagate `ResourceQuota`/`LimitRange` changes to all namespaces on that tier
- [ ] `DELETE /api/operator/tiers/:name` — reject if any repo is on it or it is the platform default
- [ ] `POST /api/operator/tiers/:name/set-default` — update platform default
- [ ] Tier assignment on repo creation (use platform default)
- [ ] `PATCH /api/operator/repos/:slug` — promote or demote tier; update all app namespaces
- [ ] App count enforcement at deploy time — `quota_exceeded` if repo is at its app limit
- [ ] Tier demotion guard — reject if current usage exceeds lower tier limits
- [ ] `morsel operator tier *` CLI commands (thin wrappers over operator API)
