Up: [Index](README.md) · Prev: [CLI](cli.md) · Next: [Queue Service](queue-service.md)

---

# Component — Blob Service

> **Status:** Draft · **Date:** May 2026

---

## Overview

The blob service is a lightweight HTTP object storage proxy running in the `morsel` namespace. It gives apps a simple key/value storage API backed by platform object storage while enforcing per-app namespace isolation and quota limits. Apps access it via the fixed hostname `blob.morsel.internal` with no SDK, no credentials, and no knowledge of the underlying storage bucket.

---

## Component Diagram

```
App pod (app namespace)
  │
  │  HTTP  GET /objects/{key}
  │        PUT /objects/{key}
  │        GET /objects?prefix=...
  ▼
blob.morsel.internal (ClusterIP Service)
  │
  ┌─────────────────────────────────────────┐
  │ Blob Service (morsel namespace)         │
  │                                         │
  │  HTTP handler                           │
  │    ├── Identify caller (pod SA token)   │
  │    ├── Namespace key: repo/app/{key}    │
  │    ├── Check / update quota (SQLite PV) │
  │    └── Proxy to object storage          │
  │                                         │
  │  SQLite (PersistentVolume)              │
  │    quota_usage: {namespace → bytes}     │
  │    quota_limits: {namespace → limit}    │
  └─────────────────────────────────────────┘
          │
          ▼ platform credentials
        Object storage (platform-provided)
```

---

## Key Namespacing

All keys are prefixed with the caller's repo and app before being forwarded to object storage:

```
Storage key:  {repo-slug}/{app-name}/{key}
```

The blob service derives the prefix from the caller's pod service account token — the app never sets it directly. This ensures an app can only reach keys under its own prefix regardless of what key it requests.

---

## Personas

**Developers** access blob storage from their app code via plain HTTP to `blob.morsel.internal`. No client configuration beyond the hostname.

**Operators** observe blob storage quotas via the cost dashboard and can adjust tier limits via tier promotion.

---

## API

All requests are authenticated by the pod's projected Kubernetes service account token — passed automatically by the Kubernetes pod runtime, not by the application.

```
GET  /objects/{key}
```
Retrieve an object. Returns `404` if not found.

```
PUT  /objects/{key}
Content-Type: application/octet-stream
{body}
```
Store an object. Returns `429 Too Many Requests` if the write would exceed the app's quota.

```
GET  /objects?prefix={prefix}&cursor={cursor}
```
List keys with optional prefix filter. Returns a page of keys and a cursor for the next page. `cursor` is omitted on the last page.

**Response shape for list:**
```json
{
  "keys": ["reports/2026-05.csv", "reports/2026-04.csv"],
  "next_cursor": "reports/2026-04.csv"
}
```

**Quota exceeded response:**
```json
{
  "error": "blob_quota_exceeded",
  "used_bytes": 1073741824,
  "limit_bytes": 1073741824,
  "remedy": "request a storage increase from your platform operator"
}
```

---

## Functionality

### Caller Identification

The blob service reads the Kubernetes service account token projected into every pod at a fixed path. It calls the Kubernetes TokenReview API to resolve the token to a service account name, then maps the service account to its app namespace (`{repo-slug}/{app-name}`). This happens transparently — the app passes nothing.

### Key Namespacing

The app uses bare keys (e.g., `reports/2026-05.csv`). The blob service prepends `{repo-slug}/{app-name}/` before any object storage operation. The full storage key is `{repo-slug}/{app-name}/reports/2026-05.csv`. The app never sees this prefix.

An app that passes a key starting with another app's namespace prefix will still be namespaced under its own prefix — collision is structurally impossible.

### Quota Enforcement

The blob service tracks per-app byte usage in a SQLite file on a PersistentVolume. On each `PUT`:
1. Calculate the size of the incoming body
2. Check whether `current_usage + new_size > limit`
3. If yes, return `429`
4. If no, write to object storage, then increment the usage counter

Usage is tracked at byte granularity. Over-counting due to object replacement is avoided by reading the current object size before overwriting and adjusting the delta.

### Quota Limit Updates

The control plane pushes updated quota limits to the blob service when an app's tier changes:

```http
POST /internal/quota/{namespace}/{app-name}
Authorization: Bearer <blob-internal-token>

{ "blob_bytes": 5368709120 }
```

The blob service rejects calls without a valid token with `401 Unauthorized`. The token is a shared secret provisioned at bootstrap, stored in the platform SecretStore under the key `blob-internal-token`, and mounted as the `BLOB_INTERNAL_TOKEN` environment variable in both the control plane pod and the blob service pod. It is independent of the queue service token — a compromised blob-service token cannot be replayed against the queue service.

The blob service does not pull limits on its own — limits are set only via this push.

### Object Storage Interaction

The blob service authenticates to object storage using ambient platform credentials. Object storage credentials are never visible outside the blob service process. All traffic to object storage uses the platform's internal network. See [platform/gcp.md](../platform/gcp.md) for GCP-specific details.

---

## Dollar Cost

| Resource | Allocation | Monthly estimate |
|---|---|---|
| CPU request | 0.1 cores | ~$2 |
| Memory request | 128 MB | ~$0.50 |
| PersistentVolume (quota tracking SQLite) | 1 GB SSD | ~$0.20 |
| Object storage (all apps combined) | Usage-based | Platform-dependent (see [platform/gcp.md](../platform/gcp.md)) |
| Object storage operations | Usage-based | Negligible for non-production volumes |

The blob service itself is cheap. Object storage cost scales with how much data apps store.

---

## Operational Cost

- **Upgrades** — rolling pod replacement during platform upgrade. Brief blob unavailability during switchover (seconds).
- **Quota adjustments** — the control plane pushes new limits automatically on tier change. No operator intervention in the blob service itself.
- **Monitoring** — blob service exposes a `/healthz` endpoint. Quota usage visible via `GET /api/repos/:slug/apps/:name/utilisation`.

---

## Scalability

The blob service is single-replica. SQLite is a single-writer store for quota tracking. At the target scale (500 apps, non-production workloads), a single replica is adequate — non-production blob access is infrequent.

For higher throughput, the quota tracking store could be migrated to a multi-reader cache (Redis or Postgres). This is not planned.

---

## Security

- Caller identity derived from Kubernetes service account token — the app cannot impersonate another app
- Key namespacing enforced server-side — cannot be overridden by the app
- Object storage credentials never exposed to app pods
- Blob service pod has no access to app namespaces — read-only TokenReview only
- All object storage traffic via the platform's internal network

---

## Performance

- `PUT` and `GET` latency dominated by object storage round-trip (typically 20–50ms within the same region)
- Quota check: SQLite read — sub-millisecond
- Key listing: object storage list API — proportional to result count; pagination limits response size

---

## Platform Feature Support

### Hibernation
The blob service is unaffected by app hibernation. Objects are retained in platform object storage while the app is hibernated. There is no connection to maintain — each HTTP request is stateless.

### Cost Controls
The blob service is the enforcement point for per-app blob storage quota. It tracks byte usage in SQLite and rejects writes that exceed the limit. Quota limits are pushed by the control plane on tier changes. See [platform-features/cost-controls.md](../platform-features/cost-controls.md).

### Persistence
Blob storage is one of the three managed persistence types. Object lifecycle (grace period, permanence) is managed by the control plane — the blob service stores and retrieves objects but does not implement deletion policies itself. See [platform-features/persistence.md](../platform-features/persistence.md).

---

Up: [Index](README.md) · Prev: [CLI](cli.md) · Next: [Queue Service](queue-service.md)
