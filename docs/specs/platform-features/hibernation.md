Up: [Index](README.md) · Prev: [Authentication](authentication.md) · Next: [Cost Controls](cost-controls.md)

---

# Platform Feature — Hibernation

> **Status:** Draft · **Date:** May 2026

---

## Summary

Apps that receive no traffic (HTTP apps) or have no external queue messages (worker apps) for their configured idle threshold are automatically scaled to zero replicas. Persistence is retained. HTTP apps wake transparently on the next request via a lightweight proxy that holds the connection while the pod starts. Workers wake when a queue receives an external message.

Hibernation is the primary cost control mechanism for compute. An app that is not being used costs nothing in CPU or memory.

---

## Why Hibernation

Non-production apps are frequently idle. A demo app used for a weekly meeting runs one hour per week. Without hibernation it consumes resources 168 hours per week. With hibernation it costs approximately 1/168th as much in compute.

The tradeoff is cold-start latency — the first request after hibernation takes 5–15 seconds rather than milliseconds. This is acceptable for demos, experiments, and internal tools. It is not acceptable for production workloads, which is why Morsel is not intended for production.

---

## Idle Threshold

Each app declares its idle threshold in `morsel.json`:

```json
{ "idle_after": "24h" }
```

Valid duration strings: `"1h"`, `"24h"`, `"48h"`, `"72h"`, etc.

If `idle_after` is omitted, the platform-wide default applies (configurable by the operator via `PATCH /api/operator/config`, default `24h`).

Developers control their own cold-start tradeoff: a shorter threshold saves more money but produces more cold starts.

---

## HTTP App Hibernation

### Idle Detection

The control plane watcher goroutine runs on a configurable tick interval (default 60 seconds). At each tick it reads the total HTTP request count for each app from Envoy Gateway's Prometheus metrics endpoint. If the count has increased since the previous tick, the app's `last_active_at` timestamp in SQLite is updated to now. An app is considered idle when `now − last_active_at` exceeds its `idle_after` threshold.

Precision is one tick interval — an app that receives its last request just after a tick may remain running for up to `idle_after + tick_interval` before hibernating. This is acceptable; the idle threshold is measured in hours.

### Scale to Zero

When idle threshold is exceeded, the control plane issues a `scale to 0` command via `client-go`. Kubernetes terminates the pod. The platform gateway entry for the app is updated to route to the wake-on-request proxy instead of the (now absent) pod.

### Wake on Request

When a request arrives for a hibernated HTTP app:

1. Platform gateway routes the request to the wake-on-request proxy
2. The proxy reads the `Host` header to identify the target app
3. The proxy calls the control plane internal wake endpoint (`POST /internal/wake?host={hostname}`)
4. The control plane scales the Deployment to 1 and returns `202 Accepted` immediately — it does not wait for the pod to become ready. The readiness wait and `HTTPRoute` restoration happen in the background
5. The proxy responds to the original request with an HTML interstitial page ("Waking `{app}`…") that self-refreshes every 5 seconds — it does not hold the connection open
6. On each refresh, the gateway still routes to the wake-on-request proxy until the background wake finishes, so the operator keeps seeing the interstitial
7. Once the background wake completes, the control plane restores the `HTTPRoute` to the app's own Service
8. The next refresh is routed by the gateway directly to the app's Service, bypassing the wake proxy — the real response is what the browser finally renders

**Cold start time:** Typically 5–15 seconds depending on container image size and application startup time. Returning the interstitial immediately avoids tripping the gateway's upstream request timeout on slower-starting apps — the wait is visible to the caller instead of surfacing as a request timeout.

### Subsequent Requests

After wake, requests route directly to the pod. The proxy is no longer in the path. The idle timer resets on each request.

---

## Wake-on-Request Proxy

The wake-on-request proxy is a shared lightweight Deployment running in the `morsel-services` namespace. It is not per-app — all hibernated HTTP apps share a single proxy instance.

### Routing

When an app hibernates, its `HTTPRoute` is updated to point to the wake-proxy Service. On wake completion, the control plane restores the `HTTPRoute` to the app's own Service. The proxy is only in the request path while an app is in the process of waking.

### Deployment

| Property | Value |
|---|---|
| Namespace | `morsel-services` |
| Replicas | 1 |
| RBAC | Read-only on Pods in app namespaces (to verify readiness watch result) |
| Auth to control plane internal API | Shared token stored in a `morsel-services` Kubernetes Secret |

The internal wake endpoint (`POST /internal/wake/{namespace}/{name}`) is cluster-internal only — bound to `127.0.0.1` or reachable only within the cluster via `NetworkPolicy`. It is not part of the public control plane.

### Wake Failure

The proxy's call to `/internal/wake` is a fast fire-and-trigger request (a few seconds at most) — it only fails if the app can't be found, the wake token is invalid, or the platform is over its cost budget. In those cases the proxy returns `503` immediately rather than showing the interstitial:

```json
HTTP 503 Service Unavailable
{ "error": { "code": "budget_soft_limit", "message": "platform is over budget for this period" } }
```

If the background scale-up itself fails (e.g. the readiness probe never passes), the operation is recorded as failed but the caller isn't notified directly — the interstitial keeps refreshing and the app remains routed through the wake proxy. An operator can see the failure in the app's operation history.

---

## Worker App Hibernation

Workers have no HTTP interface, so request-based idle detection does not apply. Workers hibernate when all of their declared queues are idle.

### Queue Idle Detection

The watcher goroutine polls `GET /internal/queues/{namespace}/{app-name}` on the queue service at each tick. A worker hibernates when all of its queues report `idle: true`. The queue service's `last_external_enqueue_at` tracking ensures a worker cannot keep itself alive by processing only its own messages. See [components/queue-service.md — Hibernation](../components/queue-service.md) for the idle flag logic.

### Worker Wake

Workers wake on the next watcher tick after an external message is enqueued — the queue becomes `idle: false`, the watcher scales the Deployment to 1. There is no wake-on-request proxy for workers; the first external message may wait in the queue for up to one tick interval plus the pod cold-start duration before being processed.

---

## CronJob Hibernation

CronJobs are hibernated by suspending the Kubernetes CronJob spec (`spec.suspend: true`). The schedule is paused — no new Job pods are created while suspended.

On wake, the CronJob spec is unsuspended. Missed schedules during hibernation are not retroactively executed (Kubernetes default behaviour for suspended CronJobs).

CronJobs are hibernated by the same idle detection mechanism as HTTP apps — they are considered idle if their associated HTTP health endpoint (if declared) receives no traffic.

---

## Force Hibernate and Wake

Developers and operators can force hibernate or wake any app regardless of idle threshold:

```
POST /api/repos/:slug/apps/:name/hibernate
POST /api/repos/:slug/apps/:name/wake
```

Both are asynchronous — they return `202 Accepted` with an operation ID immediately, and the scale operation runs in the background. Poll `GET /api/repos/:slug/apps/:name/operations/:id` to track completion.

Force hibernate is useful for temporarily suspending an app to save cost during a known period of non-use. Wake is useful to pre-warm an app before a demo.

---

## Cost Impact

| App state | Compute cost | Persistence cost |
|---|---|---|
| Running | CPU + memory per resource request | Database + blob + queue storage |
| Hibernated | Zero compute | Database + blob + queue storage |

Hibernation eliminates compute cost entirely for idle apps. Persistence costs continue regardless of app state — data is retained while hibernated.

At the platform level, hibernation is the primary mechanism for keeping the Kubernetes cluster right-sized. A cluster of 50 apps where 40 are typically hibernated at any given time requires far fewer nodes than one where all 50 are running.

---

## Configuration Summary

| Config | Where set | Default |
|---|---|---|
| Per-app idle threshold | `idle_after` in `morsel.json` | Platform default |
| Platform-wide default | `PATCH /api/operator/config` → `default_idle_after` | `24h` |
| Watcher tick interval | `PATCH /api/operator/config` → `hibernation_check_interval` | `60s` |

---

## Component Contributions

### Control Plane
Owns the watcher goroutine, idle detection, scale-to-zero via `client-go`, the internal wake endpoint, `HTTPRoute` updates on hibernate and wake, and wake_timeout enforcement. See [components/control-plane.md — Hibernation](../components/control-plane.md).

### Wake Proxy

Shared Deployment in `morsel-services`. Holds inbound TCP connections for hibernated apps, calls the control plane internal wake endpoint, and forwards the buffered request once the app is ready. Has no direct write access to Kubernetes — all scale and route operations are delegated to the control plane.

### Queue Service
Reports per-queue `idle` status including self-consume detection. See [components/queue-service.md — Hibernation](../components/queue-service.md).

### Database Service
No active role in hibernation. Database connection is retained while the app is hibernated; PGBouncer sidecar is removed when the pod scales to zero and recreated on wake. See [components/database-service.md — Hibernation](../components/database-service.md).

### Admin UI
Surfaces hibernation status, last activity time, and hibernate/wake controls per app. See [components/admin-ui.md — Hibernation](../components/admin-ui.md).

---

Up: [Index](README.md) · Prev: [Authentication](authentication.md) · Next: [Cost Controls](cost-controls.md)
