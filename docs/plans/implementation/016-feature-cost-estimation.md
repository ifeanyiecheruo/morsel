# Feature 16 — Cost Estimation

_Delivers: operators can see estimated monthly spend per app and platform-wide._

**Direct dependencies:** [F13](013-feature-hibernation.md)

> Can be developed in parallel with F15 once F13 is done. F17 (Budget Enforcement) blocks on this.

## Tasks

- [x] `scale_events` table in SQLite — columns: `id`, `namespace`, `app`, `event` (`scale_to_1` / `scale_to_0`), `occurred_at`; written on every hibernation and wake transition
- [x] Daily price-fetch goroutine in control plane — call `Platform.Pricing().Prices()` once per day
- [x] `LocalPlatform.Pricing()` — returns `Prices{}` with all-zero fields (LocalPlatform has no billing)
- [x] `price_snapshots` table in SQLite — one immutable row per fetch; columns match `Prices` struct fields + `fetched_at`
- [x] 48-hour staleness check — emit `prices_stale` warning in `GET /api/operator/status` if last snapshot is older than 48h
- [x] Cost estimation function — compute `running_hours_this_period` from `scale_events` log; multiply by resource requests × latest snapshot prices
- [x] `GET /api/repos/:slug/apps/:name/utilisation` — resource usage + `estimated_cost_month`
- [x] `GET /api/repos/:slug` — include per-repo `estimated_cost_month` (sum of apps)
- [x] `GET /api/operator/cost` — `estimated_total_month`, `prices_fetched_at`, per-repo breakdown
- [x] `GET /api/operator/prices/history` — full snapshot list for debugging
