# Feature 15 — Approvals

_Delivers: protected field changes (tier, resource limits) require operator sign-off before taking effect._

**Direct dependencies:** [F05](005-feature-app-lifecycle-api.md), [F14](014-feature-quota-tiers.md)

> Can be developed in parallel with F12 once F14 is done.

## Tasks

- [ ] `approvals` table in SQLite (`id`, `repo`, `app`, `field`, `current_value`, `requested_value`, `requested_at`, `expires_at`, `status`)
- [ ] Protected field detection in deploy handler — create approval record when a protected field changes from its current approved value
- [ ] Approval coalescing — if an approval for the same field already exists, update `requested_value` in-place; do not reset `expires_at`
- [ ] Deploy proceeds at current approved config; changed field reverts until approved
- [ ] `GET /api/repos/:slug/approvals` — list pending approvals for repo
- [ ] `GET /api/operator/approvals` — list all pending approvals
- [ ] `GET /api/operator/approvals/:id` — single approval detail
- [ ] `POST /api/operator/approvals/batch` — approve / reject (with reason) / ignore
- [ ] Approved field reconciliation — on approve, redeploy app with the now-approved value
- [ ] Approval expiry background goroutine — runs daily; marks expired approvals as `expired`; reverts field to current approved value
- [ ] `morsel app deploy` CI warning annotations for pending approvals
