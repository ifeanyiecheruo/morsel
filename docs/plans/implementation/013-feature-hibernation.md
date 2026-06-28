# Feature 13 — Hibernation

_Delivers: idle apps automatically scale to zero; first request after hibernation wakes the app transparently._

**Direct dependencies:** [F06](006-feature-kubernetes-manifest-apply.md), [F07](007-feature-bootstrap-local-platform.md), [F08](008-feature-local-platform-deploy-path.md), [F09](009-feature-networking.md), [F12](012-feature-queue-service.md)

## Tasks

- [ ] Hibernation watcher goroutine — tick on configurable interval (`hibernation_check_interval`, default 60s)
- [ ] HTTP idle detection — read Envoy Gateway Prometheus metrics per app at each tick; compare request count to previous tick; update `last_active_at` in SQLite when count increases; hibernate when `now − last_active_at > idle_after`
- [ ] Worker idle detection — poll `GET /internal/queues/{namespace}/{app-name}` on queue service at each tick; hibernate worker when all queues return `idle: true`
- [ ] Idle threshold evaluation per app (`idle_after` from morsel.json or platform default `default_idle_after`)
- [ ] Scale-to-zero via `client-go` on idle threshold exceeded
- [ ] App hibernation state persisted in SQLite (`hibernated_at`, `hibernation_reason`)
- [ ] `HTTPRoute` update — route hibernated app's subdomain to wake-proxy Service in `morsel-services`
- [ ] Wake proxy binary — read `Host` header; call `POST /internal/wake/{namespace}/{name}` on control plane; forward held request to returned Service address on success; return `503 wake_timeout` on timeout
- [ ] Wake proxy Deployment + Service + NetworkPolicy in `morsel-services`; shared token Secret for control plane auth
- [ ] control plane internal wake endpoint — scale to 1, watch readiness, restore `HTTPRoute`, return Service address; cluster-internal only
- [ ] `HTTPRoute` restore — control plane restores subdomain to app Service as part of wake completion
- [ ] Worker hibernation — queue service idle flag polling; scale-to-zero when all queues idle
- [ ] `CronJob` suspend via `spec.suspend: true`; unsuspend on wake
- [ ] `POST /api/repos/:slug/apps/:name/hibernate` — force hibernate (synchronous)
- [ ] `POST /api/repos/:slug/apps/:name/wake` — force wake (synchronous); check soft/hard budget limit
- [ ] `GET /api/repos/:slug/apps/:name/status` — include `hibernated`, `hibernated_at`, `idle_since`
