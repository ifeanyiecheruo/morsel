# Feature 13 — Hibernation

_Delivers: idle apps automatically scale to zero; first request after hibernation wakes the app transparently._

**Direct dependencies:** [F06](006-feature-kubernetes-manifest-apply.md), [F07](007-feature-bootstrap-local-platform.md), [F08](008-feature-local-platform-deploy-path.md), [F09](009-feature-networking.md), [F12](012-feature-queue-service.md)

## Tasks

- [x] Hibernation watcher goroutine — tick on configurable interval (`hibernation_check_interval`, default 60s)
- [x] HTTP idle detection — update `last_active_at` in SQLite when count increases; hibernate when `now − last_active_at > idle_after`
- [x] Worker idle detection — poll `GET /internal/queues/{namespace}/{app-name}` on queue service at each tick; hibernate worker when all queues return `idle: true`
- [x] Idle threshold evaluation per app (`idle_after` from morsel.json or platform default `default_idle_after`)
- [x] Scale-to-zero via `client-go` on idle threshold exceeded
- [x] App hibernation state persisted in SQLite (`hibernated_at`, `hibernation_reason`)
- [x] `HTTPRoute` update — route hibernated app's subdomain to wake-proxy Service in `morsel-services`
- [x] Wake proxy — `morsel-ctrl-plane svc wake-proxy`; reads `Host` header; calls `POST /internal/wake?host=` on control plane; forwards held request to returned service address
- [x] Wake proxy Deployment + Service + NetworkPolicy in `morsel-services`; shared token Secret for control plane auth
- [x] control plane internal wake endpoint — scale to 1, watch readiness, restore `HTTPRoute`, return Service address; cluster-internal only
- [x] `HTTPRoute` restore — control plane restores subdomain to app Service as part of wake completion
- [x] Worker hibernation — queue service idle flag polling; scale-to-zero when all queues idle
- [x] `CronJob` suspend via `spec.suspend: true`; unsuspend on wake
- [x] `POST /api/repos/:slug/apps/:name/hibernate` — force hibernate (asynchronous)
- [x] `POST /api/repos/:slug/apps/:name/wake` — force wake (asynchronous)
- [x] `GET /api/repos/:slug/apps/:name/status` — include `hibernated`, `hibernated_at`, `idle_since`
