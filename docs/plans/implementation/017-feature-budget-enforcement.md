# Feature 17 — Budget Enforcement

_Delivers: platform automatically enforces spend limits; no app can inadvertently blow the budget._

**Direct dependencies:** [F13](013-feature-hibernation.md), [F16](016-feature-cost-estimation.md)

## Tasks

- [ ] `platform_config` table in SQLite — `budget_ceiling`, `soft_limit_pct`, `hard_limit_pct`, `default_idle_after`
- [ ] `GET /api/operator/config` + `PATCH /api/operator/config`
- [ ] Cost enforcement watcher goroutine — runs on configurable tick interval (default 5 min)
- [ ] Soft limit — set `budget_soft_limit_active` flag; wake-on-request proxy returns `503` with `Retry-After`; explicit wake returns `budget_soft_limit`
- [ ] Hard limit — force-hibernate all running non-exempt apps; wake blocked
- [ ] Billing period rollover — first tick after calendar month rollover clears flags; expires period exemptions
- [ ] Operator wake override — wake during active limit grants period exemption for remainder of billing period
- [ ] `exemptions` table in SQLite — app-level and repo-level; explicit vs period
- [ ] `POST /api/operator/app-exemptions` + `DELETE` — explicit exemption add/remove
- [ ] `POST /api/operator/repo-exemptions` + `DELETE` — repo-level exemption
- [ ] `GET /api/operator/exemptions` — list all active exemptions
- [ ] `morsel operator app exempt / repo exempt` CLI commands
