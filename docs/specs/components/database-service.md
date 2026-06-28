Up: [Index](README.md) · Prev: [Queue Service](queue-service.md) · Next: [Admin UI](admin-ui.md)

---

# Component — Database Service

> **Status:** Draft · **Date:** May 2026

---

## Overview

The database service provides each app with a dedicated Postgres database on a shared in-cluster Postgres instance. Access is proxied through a PGBouncer sidecar running alongside the app pod. The app connects using fixed constants — no credentials to manage, no SDK to configure. Real per-app credentials are held in a Kubernetes Secret and are never visible to the developer.

---

## Component Diagram

```
App pod (app namespace)
  │
  │  Postgres wire protocol
  │  host=database.morsel.internal port=5432
  │  dbname=morsel user=morsel password=morsel
  ▼
PGBouncer sidecar (same pod, localhost)
  │  reads K8s Secret: {real-credentials}
  │  maps morsel/morsel/morsel → real per-app credentials
  ▼
database.morsel.internal (ClusterIP Service, morsel namespace)
  │
  ┌─────────────────────────────────────────────────┐
  │ Shared Postgres instance (morsel namespace)     │
  │                                                 │
  │  Database: {repo-slug}-{app-name}               │
  │  User:     {repo-slug}-{app-name}               │
  │  GRANT ALL on own database only                 │
  │                                                 │
  │  PersistentVolume (platform SSD)                │
  └─────────────────────────────────────────────────┘
```

---

## Database Naming

Each app gets its own Postgres database and user, named from its repo slug and app name:

```
Database:  {repo-slug}-{app-name}
User:      {repo-slug}-{app-name}
```

The real credentials are held in a Kubernetes Secret in the app namespace and are only ever read by the PGBouncer sidecar. The app always connects with the fixed constants `morsel`/`morsel`/`morsel` — the real names are never visible to the developer.

---

## Personas

**Developers** connect to their app's database using fixed constants in their application code. No configuration, no secrets management.

**Operators** observe database storage usage via the cost dashboard. No routine management of the Postgres instance is expected.

---

## Access Constants

These constants are the same for every app on the platform. They never change.

| Constant | Value |
|---|---|
| Host | `database.morsel.internal` |
| Port | `5432` |
| Database | `morsel` |
| Username | `morsel` |
| Password | `morsel` |

These are PGBouncer conventions mapped to real per-app credentials by the sidecar. The developer uses these constants in their application config verbatim.

---

## Functionality

### Per-App Provisioning

When an app first declares `persistence.database`, the control plane provisions:

1. A Postgres database named `{repo-slug}-{app-name}`
2. A Postgres user named `{repo-slug}-{app-name}` with a generated password
3. A `GRANT ALL` scoped to that database only
4. A Kubernetes Secret in the app's namespace containing the real credentials
5. A PGBouncer sidecar configuration in the app's pod spec that maps `morsel/morsel/morsel` → the real credentials

On re-deploy, if the database already exists, nothing changes. Provisioning is idempotent.

### PGBouncer Sidecar

PGBouncer runs as a sidecar container in the app pod — same pod, same network namespace. The app's `database.morsel.internal` connection resolves to the cluster-wide Postgres service, but the control plane also injects `127.0.0.1 database.morsel.internal` into the pod's `/etc/hosts` (or uses a loopback alias) so that the app actually hits the local PGBouncer first.

PGBouncer:
- Listens on port 5432
- Accepts `morsel/morsel/morsel` as credentials
- Maps to the real per-app credentials from the Kubernetes Secret
- Forwards to the shared Postgres instance at `postgres.morsel.svc.cluster.local`

Connection pool mode: transaction pooling (default) — balances connection count against per-request overhead.

### Isolation

Each app's Postgres user is granted access only to its own database. Cross-app database access is impossible at the Postgres level regardless of what the app attempts.

Additionally, Kubernetes NetworkPolicy prevents pod-to-pod sidecar access. A developer who `exec`s into a pod and connects directly to `database.morsel.internal:5432` hits their own pod's PGBouncer — not another app's sidecar.

### Storage Quota

Database storage limits are advisory. The control plane tracks declared allocations per tier and blocks deploy requests that would exceed the tier's database storage limit. However, per-database enforcement at the Postgres level is not possible on a shared instance — Postgres does not support tablespace-level quotas per database in a practical way.

Apps that exceed their database storage limit continue to operate until the operator takes action. The admin UI surfaces databases approaching their advisory limit.

---

## Dollar Cost

| Resource | Allocation | Monthly estimate |
|---|---|---|
| Postgres CPU request | 0.5 cores | ~$10 |
| Postgres memory request | 1 GB | ~$4 |
| PersistentVolume | 100 GB SSD | ~$20 |
| PGBouncer sidecar (per app) | 0.05 cores / 64 MB | ~$1/app |

Shared Postgres instance: approximately **$34/month** base, plus ~$1/month per app with a database declared.

The shared instance is more cost-effective than a managed Cloud SQL instance (minimum ~$50/month) and avoids the operational overhead of a managed service.

---

## Operational Cost

- **Upgrades** — rolling pod replacement for PGBouncer sidecars during platform upgrade. Brief database proxy unavailability per app during switchover (seconds). Postgres itself is not restarted during upgrades.
- **Postgres version upgrades** — requires a Postgres pod replacement. Apps briefly lose database connectivity. Planned during low-traffic windows.
- **PV management** — the Postgres PV must be large enough for all apps' combined data. The operator monitors usage via the cost dashboard and can resize the PV via standard Kubernetes PV expansion.
- **No backups** — consistent with the platform's non-production posture. App owners are responsible for data they cannot afford to lose.

---

## Scalability

A single shared Postgres instance supports hundreds of databases on modest hardware. For the target scale (500 apps, non-production workloads with infrequent writes), a single instance is more than adequate.

PGBouncer transaction pooling means active connections to Postgres scale with concurrent transactions, not with app count. An app that is hibernated has no sidecar and holds no Postgres connections.

Scaling the Postgres instance vertically (larger Kubernetes node) is the primary scaling path. Horizontal sharding would require significant redesign and is not planned.

---

## Security

- Real credentials stored only in Kubernetes Secrets in the app's own namespace
- App never has access to its own Kubernetes Secret (no `get secret` RBAC)
- PGBouncer is the only component that reads the Secret — via projected volume mount
- Postgres user grants scoped to one database only
- NetworkPolicy prevents cross-app sidecar access
- No Postgres superuser credentials accessible outside the `morsel` namespace

---

## Performance

- Connection overhead: PGBouncer transaction pooling reduces Postgres connection count proportionally to the pool size
- Query latency: in-cluster Postgres on SSD — sub-millisecond for simple queries
- Cold start: PGBouncer sidecar starts in < 1 second — no meaningful add to app cold start time
- Connection re-establishment on wake: PGBouncer re-connects to Postgres when the app wakes from hibernation; first query may take an additional 10–50ms for connection setup

---

## Platform Feature Support

### Hibernation
When an app is hibernated (scaled to 0), the PGBouncer sidecar is removed along with the app pod. The Postgres database and all data are retained. On wake, the sidecar is recreated with the same credentials from the Kubernetes Secret. No database reconnection logic is required from the app. See [platform-features/hibernation.md](../platform-features/hibernation.md).

### Cost Controls
Advisory database storage limits are enforced at the control plane tier-change layer. The database service component (Postgres + PGBouncer) has no active role in quota enforcement — it is a passive data store from the quota perspective. See [platform-features/cost-controls.md](../platform-features/cost-controls.md).

### Persistence
The database service is the backing store for app databases and queue service tables. Provisioning, permanence, grace periods, and deletion are all orchestrated by the control plane. See [platform-features/persistence.md](../platform-features/persistence.md).

---

Up: [Index](README.md) · Prev: [Queue Service](queue-service.md) · Next: [Admin UI](admin-ui.md)
