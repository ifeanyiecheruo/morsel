Up: [Index](../README.md) · Prev: [Persistence](../platform-features/persistence.md) · Next: [CLI](cli.md)

---

# Component — Control Plane

> **Status:** Draft · **Date:** May 2026

---

## Overview

The control plane (`morsel-ctrl-plane`) is the center of the entire platform. It is the only component that speaks to Kubernetes, manages DNS records, provisions TLS certificates, and coordinates the deploy lifecycle. All other components — the CLI, admin UI, blob service, and queue service — interact with it via HTTP.

The control plane is a single Go binary running as a Kubernetes Deployment in the `morsel` namespace. It holds its own state in SQLite on a Kubernetes PersistentVolume.

---

## Component Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│ Control Plane / morsel-ctrl-plane (morsel namespace)                                    │
│                                                                  │
│  ┌──────────────────┐   ┌───────────────────┐                   │
│  │  HTTP server     │   │  Background tasks │                   │
│  │  /api/token/*    │   │  Hibernation      │                   │
│  │  /api/repos/*    │   │  watcher          │                   │
│  │  /api/operator/* │   │  Cert renewal     │                   │
│  └────────┬─────────┘   │  Approval expiry  │                   │
│           │             └────────┬──────────┘                   │
│  ┌────────▼─────────────────────▼──────────┐                   │
│  │  Core services                          │                   │
│  │  Auth  │  Quota  │  Approval  │  Deploy │                   │
│  └────────┬─────────────────────────────────┘                   │
│           │                                                      │
│  ┌────────▼─────────────────────────────────┐                   │
│  │  SQLite (PersistentVolume)               │                   │
│  │  repos · apps · tokens · approvals ·    │                   │
│  │  image digests · operations              │                   │
│  └──────────────────────────────────────────┘                   │
└──────────────────────────┬───────────────────────────────────────┘
                           │
          ┌────────────────┼────────────────────────┐
          ▼                ▼                        ▼
   Kubernetes API    Container registry      DNS provider
   (client-go)       (image copy/delete)    (platform-configured)
          │
   ┌──────┴───────────────────────────────┐
   │  App namespaces                      │
   │  Deployment / CronJob                │
   │  ResourceQuota / LimitRange          │
   │  NetworkPolicy                       │
   │  ServiceAccount                      │
   │  HTTPRoute (platform gateway)         │
   └──────────────────────────────────────┘
```

---

## Personas

**Developers** interact with the control plane indirectly through `morsel app deploy` and the GitHub Actions workflow. They never call the API directly in normal use.

**Operators** interact with the control plane through the admin UI (which calls operator endpoints on their behalf) and the `morsel` CLI.

**Internal components** (blob service, queue service) call the control plane to verify caller identity and receive updated quota limits.

---

## Functionality

### Token Exchange

Issues Morsel access tokens from trusted identity sources. The access token is a signed JWT — no database lookup is required per request.

- `POST /api/token/github-oidc` — validates GitHub OIDC JWT, issues 10-min developer access token
- `POST /api/token/platform-oidc` — validates platform identity token, checks operator principal, issues 15-min operator access token + 90-day refresh token (endpoint path is platform-specific; see [platform/gcp.md](../platform/gcp.md))
- `POST /api/token/refresh` — validates refresh token, issues new access token + rotated refresh token

### App Lifecycle

- `POST /api/repos/:slug/sync` — declarative app list reconciliation; detects deleted apps
- `POST /api/repos/:slug/apps` — upsert app; runs staging handshake, applies Kubernetes manifest
- `DELETE /api/repos/:slug/apps/:name` — delete app; starts persistence grace period
- `DELETE /api/repos/:slug` — delete all apps in repo
- `GET /api/repos/:slug/apps` — list apps
- `GET /api/repos/:slug/apps/:name` — app detail
- `GET /api/repos/:slug/apps/:name/status` — current runtime state
- `GET /api/repos/:slug/apps/:name/history` — deploy history
- `GET /api/repos/:slug/apps/:name/utilisation` — current resource usage and cost estimate
- `GET /api/repos/:slug/apps/:name/operations/:id` — poll async operation
- `POST /api/repos/:slug/apps/:name/hibernate` — force hibernate
- `POST /api/repos/:slug/apps/:name/wake` — force wake

### Repo Operations

- `GET /api/repos` — list repos (developer: own repo only; operator: all with `?all=true`)
- `GET /api/repos/:slug` — repo detail including tier and quota
- `GET /api/repos/:slug/approvals` — list pending approvals for repo

### Operator Operations

- `GET /api/operator/config` — read platform config
- `PATCH /api/operator/config` — update mutable platform config
- `PATCH /api/operator/repos/:slug` — promote or demote repo tier
- `GET /api/operator/approvals` — list all pending approvals
- `GET /api/operator/approvals/:id` — single approval detail
- `POST /api/operator/approvals/batch` — action multiple approvals
- `GET /api/operator/cost` — platform-wide cost estimate
- `GET /api/operator/status` — platform health summary

### Staging Handshake

On each deploy:
1. Validates the Morsel token and repo claim
2. Confirms the image digest exists in the staging container registry
3. Performs a registry-side copy: staging → canonical (metadata operation, no image data transferred)
4. Deletes the staging image
5. Records the new digest as `current` and the previous as `last-healthy`

### Namespace Naming

Each app gets its own Kubernetes namespace:

```
Named app:    {org-slug}-{repo-slug}--{app-name}
Unnamed app:  {org-slug}-{repo-slug}
```

Examples: `org/my-repo` + `name: "api"` → `org-my-repo--api`; `org/my-repo` + unnamed → `org-my-repo`. Hyphens within org or repo names are preserved; slashes and underscores are replaced with hyphens.

The namespace is the isolation boundary for `ResourceQuota`, `LimitRange`, `NetworkPolicy`, and Kubernetes service accounts.

### Manifest Apply

The control plane applies Kubernetes resources directly via `client-go`. There is no Helm, no Flux, no GitOps operator. On each deploy:
- Creates or updates the app namespace if needed
- Applies `ResourceQuota` and `LimitRange` for the repo's current tier
- Applies `NetworkPolicy`
- Applies `Deployment`, `CronJob`, or headless `Service` depending on app type
- Applies `HTTPRoute` for HTTP apps
- Applies `ServiceAccount`
- Adds PGBouncer sidecar if database is declared
- Watches the rollout via `client-go`; triggers rollback on failure

### Certificate Management

The control plane manages TLS certificates directly using the Go ACME library against Let's Encrypt:
- Provisions certificates for new HTTP app domains
- Renews certificates 30 days before expiry via a background goroutine
- Uses DNS-01 challenge via the configured DNS provider
- Stores certificates in Kubernetes Secrets in each app's namespace

### DNS Management

The control plane creates and removes DNS records via the configured DNS provider:
- Creates A record on app deploy
- Updates record if load balancer IP changes
- Removes record on app delete

### Hibernation Watcher

A background goroutine tracks activity and manages hibernation:
- Reads HTTP request metrics from the platform gateway for HTTP apps
- Polls queue service for worker apps
- Issues `scale to 0` via `client-go` when idle threshold exceeded
- Updates `HTTPRoute` to route to wake-on-request proxy
- Manages wake-on-request proxy lifecycle
- Suspends/unsuspends CronJob specs

### Cost Enforcement Watcher

A background goroutine that evaluates estimated spend against the budget ceiling and enforces active cost controls. Runs on a configurable tick interval (default 5 minutes).

**Budget evaluation:**

- Computes the current estimated monthly spend by summing per-app cost estimates across all running and hibernated apps
- Compares the estimate against the configured soft and hard limit thresholds
- When the soft limit is crossed, sets a platform-wide `budget_soft_limit_active` flag; the wake-on-request proxy and explicit wake handler check this flag before waking an app
- When the hard limit is crossed, issues `scale to 0` for all running non-exempt apps via `client-go`
- When a new billing period begins (first tick after calendar month rollover), clears the soft/hard limit flags and expires all period exemptions granted by operator wake overrides

**Price snapshots:**

- Once per day, fetches current compute, object storage, and container registry list prices from the platform pricing API (see [platform/gcp.md](../platform/gcp.md))
- Stores each fetch as a timestamped row in the `price_snapshots` SQLite table — one row per fetch, never overwritten
- The stored snapshot is the basis for all cost estimates until the next successful fetch
- Emits an admin UI warning if the most recent snapshot is more than 48 hours old (e.g., Catalog API unreachable)

Price history is available at `GET /api/operator/prices/history` for debugging cost estimate changes over time. See [platform-features/cost-controls.md — Cost Estimation](../platform-features/cost-controls.md).

---

## APIs

See the full REST API reference above under Functionality. All requests require `Authorization: Bearer <morsel-token>` except token exchange endpoints.

**Common response conventions:**
- Async operations return `202 Accepted` with `Location` and `Retry-After` headers
- All errors follow the structured error model — see [conventions/rest.md](../conventions/rest.md)
- Developers are scoped to their own repo; cross-repo requests return `403`

---

## Dollar Cost

The control plane runs as a single-replica Deployment in the `morsel` namespace.

| Resource | Allocation | Monthly estimate |
|---|---|---|
| CPU request | 0.25 cores | ~$5 |
| Memory request | 256 MB | ~$1 |
| PersistentVolume (SQLite) | 10 GB SSD | ~$2 |

Total control plane compute: approximately **$8/month**.

Cost is fixed regardless of app count — the API serves all apps from a single process.

---

## Operational Cost

- **Upgrades** — rolling pod replacement via `morsel service bootstrap`. Brief API unavailability during switchover (seconds). No operator action required beyond running the binary.
- **Monitoring** — platform status endpoint exposes health signal. Admin UI surfaces failed deploys, cert alerts, and pending approvals.
- **Debugging** — structured error responses and operation logs in SQLite. Raw API responses written to a log file during bootstrap for debugging.
- **On pod rescheduling** — SQLite PV is reattached automatically. Brief unavailability while pod restarts (typically < 30 seconds).

---

## Scalability

The control plane is single-replica by design. SQLite is a single-writer database and does not support concurrent writers. This is an accepted constraint for a non-production platform with the following scale target:

- ~50 active developers
- ~500 apps across all repos
- Deploy throughput: tens of concurrent deploys during a busy push window

At this scale, a single Go process with SQLite is adequate. The API is not in the critical path for app request serving — running apps communicate directly with each other via their URLs.

Scaling beyond this would require migrating from SQLite to Postgres and running multiple replicas. This is not planned.

---

## Database Schema

The control plane stores all state in a single SQLite file. WAL mode is enabled so reads do not block writes. The connection pool is capped to one connection — SQLite serialises writes regardless, but a pool cap prevents lock contention between goroutines.

### Migrations

Schema changes are applied by a migration runner at startup. Migrations are versioned SQL files named `NNN_description.sql` under `internal/db/migrations/`, applied in lexicographic order. Applied versions are recorded in a `schema_migrations` table so re-runs skip already-applied files. Each migration runs in its own transaction — a failure rolls back without touching earlier migrations.

To add a schema change: create the next numbered file (e.g. `002_add_refresh_tokens.sql`) and the runner picks it up on next startup.

### Tables

| Table | Purpose |
| --- | --- |
| `schema_migrations` | Tracks which migration files have been applied |
| `repos` | One row per repository; holds tier assignment and timestamps |
| `apps` | One row per declared app; tracks type, status, image digests, namespace, and deletion state |
| `operations` | Async operation log; each deploy, delete, hibernate, or wake creates a row polled by the client |

Additional tables are added in later features:

| Table | Added in | Purpose |
| --- | --- | --- |
| `refresh_tokens` | Feature 3 | Refresh token store for token rotation |
| `platform_config` | Feature 17 | Budget ceiling and platform-wide settings |
| `tiers` | Feature 14 | Quota tier definitions |
| `approvals` | Feature 15 | Pending protected-field change approvals |
| `scale_events` | Feature 16 | Hibernation/wake transitions for cost estimation |
| `price_snapshots` | Feature 16 | Immutable per-fetch price records |
| `exemptions` | Feature 17 | App and repo budget exemptions |

### Key Column Conventions

- Timestamps are stored as ISO 8601 UTC strings (`strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`), not Unix integers
- Boolean flags are `INTEGER NOT NULL DEFAULT 0` (0/1); SQLite has no native boolean type
- UUIDs are stored as `TEXT`; the operations `id` is a UUID assigned by the API at creation time
- Soft deletes use `deletion_pending INTEGER` + `deleted_at DATETIME` rather than physical row removal, to preserve operation history

---

## Security

- JWT signing key stored in the platform secret store — the API loads it at startup and holds it in memory
- All platform API calls via ambient cloud identity — no credential files
- `client-go` uses the pod's ambient service account — no kubeconfig on disk
- RBAC enforced at the middleware layer — `repo` claim verified on every developer request
- Permanent resource protection — `?force=true` restricted to operator role
- Refresh token rotation on every use — stolen token usable at most once
- No pod logs in error responses — prevents sensitive data leakage

---

## Performance

- Access token verification: signature check only, no database lookup — sub-millisecond
- Deploy critical path: image copy (registry metadata, fast) + manifest apply + rollout watch. Total: typically 30–90 seconds depending on container image size
- Hibernation watcher: polls every 60 seconds; scale-to-zero is a single Kubernetes API call
- Certificate renewal: background goroutine; no impact on API latency

---

## Platform Feature Support

### Hibernation
The API owns the watcher goroutine, scale-to-zero commands, and wake-on-request proxy management. The watcher goroutine runs continuously in the background, polling platform gateway metrics (HTTP apps) and the queue service (worker apps). Scale operations are issued via `client-go` against the app's Deployment spec.

### Deployment
The API owns the staging handshake, image copy to canonical registry, manifest apply, rollout watching, and automatic rollback on failure. Two image digests per app are retained: `current` and `last-healthy`. Rollback redeploys `last-healthy` without any registry interaction.

### Authentication
The API owns all token exchange endpoints, JWT signing, refresh token storage in SQLite, and RBAC middleware. The signing key is loaded from the platform secret store at startup.

### Cost Controls
The API enforces app count limits, manages Kubernetes `ResourceQuota` and `LimitRange`, and provides cost estimation endpoints. On tier promotion, the API updates quota resources in all of the repo's app namespaces.

### Approvals
The API creates pending approvals for protected field changes, stores them in SQLite, and reconciles approved changes. Approval expiry is run by a background goroutine that rejects and clears expired approvals daily.

### Networking
The API provisions `HTTPRoute` resources in the platform gateway, manages DNS records via the configured DNS provider, and runs the ACME certificate lifecycle (provisioning and renewal).

### Persistence
The API provisions Postgres databases and users, manages PGBouncer sidecar injection into app pod specs, and enforces permanence rules on resource removal.

---

Up: [Index](../README.md) · Prev: [Persistence](../platform-features/persistence.md) · Next: [CLI](cli.md)
