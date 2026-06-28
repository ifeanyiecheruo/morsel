Up: [Index](README.md) · Prev: [Networking](networking.md) · Next: [Control Plane](../components/control-plane.md)

---

# Platform Feature — Persistence

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel provides three types of managed persistence: blob storage, a relational database, and a message queue. Apps declare which types they need in `morsel.json`. The platform provisions them automatically, enforces quota limits, and manages the lifecycle of the data through app deletion and hibernation. Apps access all persistence via fixed internal hostnames with no credentials to manage.

See [conventions/resource-model.md](../conventions/resource-model.md) for the zero-config access model and namespace isolation convention.

---

## Declaring Persistence

```json
{
  "persistence": {
    "database": { "permanent": true },
    "storage":  { "permanent": false },
    "queues":   { "permanent": false }
  }
}
```

Each resource type is declared independently. Omitting a type means the app does not get that resource. The `permanent` flag controls what happens to the data when the app is deleted — see [conventions/permanence.md](../conventions/permanence.md).

---

## Blob Storage

Apps that declare `storage` get a blob storage quota and can store and retrieve objects via a simple HTTP API at `blob.morsel.internal`.

**Access:**
```
GET  http://blob.morsel.internal/objects/{key}
PUT  http://blob.morsel.internal/objects/{key}
GET  http://blob.morsel.internal/objects?prefix={prefix}&cursor={cursor}
```

**Quota:** `PUT` requests that would exceed the app's quota return `429 Too Many Requests`. Quota limits are set by the repo's tier. See [platform-features/cost-controls.md](cost-controls.md).

**Isolation:** The app uses bare keys. The blob service prepends `{repo-slug}/{app-name}/` transparently. The app cannot read or write another app's objects.

**Backing store:** Platform object storage. The app never interacts with the underlying storage directly — no SDK, no credentials, no bucket names. See [platform/gcp.md](../platform/gcp.md) for GCP-specific details.

**Durability:** Platform object storage provides high durability (platform-dependent). Blob data survives platform failures and pod rescheduling.

See [components/blob-service.md](../components/blob-service.md) for the full component spec.

---

## Database

Apps that declare `database` get a dedicated Postgres database on the shared in-cluster Postgres instance. A PGBouncer sidecar runs alongside the app pod and manages the real credentials transparently.

**Access** — fixed constants, never change:
```
Host:     database.morsel.internal
Port:     5432
Database: morsel
Username: morsel
Password: morsel
```

These are PGBouncer conventions, not real Postgres credentials. The real per-app credentials are stored in a Kubernetes Secret in the app's namespace and are never visible to the developer.

**Isolation:** Each app gets its own Postgres database and Postgres user. The user is granted access only to that app's database. Cross-app database access is structurally impossible — Kubernetes NetworkPolicy prevents direct sidecar access, and the Postgres user grants prevent access even if the policy were bypassed.

**Provisioning:** On first deploy with a database declaration, Morsel creates the Postgres database, user, and a `GRANT ALL` scoped to that database. A PGBouncer sidecar configuration is added to the app pod.

**Storage quota:** Advisory — the control plane tracks declared allocations and blocks tier-exceeding requests at deploy time, but per-database enforcement at the Postgres level is not possible on a shared instance.

**Durability:** Database data lives on a Kubernetes PersistentVolume. Data survives pod rescheduling but is lost if the PV is deleted. There is no automated backup. App owners are responsible for data they cannot afford to lose.

See [components/database-service.md](../components/database-service.md) for the full component spec.

---

## Queues

Apps that declare `queues` can create and manage message queues at runtime via `queue.morsel.internal`. Queue names are not declared in advance — the app creates them on demand.

**Access:**
```
PUT    http://queue.morsel.internal/queues/{name}
DELETE http://queue.morsel.internal/queues/{name}
GET    http://queue.morsel.internal/queues
POST   http://queue.morsel.internal/queues/{name}/messages
GET    http://queue.morsel.internal/queues/{name}/messages/next
DELETE http://queue.morsel.internal/queues/{name}/messages/{id}
GET    http://queue.morsel.internal/queues/{name}/depth
```

**Semantics:** Point-to-point delivery only. Each message is delivered once to one consumer. Topic/fan-out semantics are not supported — apps requiring pub/sub should deploy a broker as a Morsel app.

**Isolation:** Queue names are scoped to the calling app's pod identity. Two apps can use the same queue name without collision.

**Quota:** Total message storage across all of the app's queues is capped by the repo's tier. Enqueue requests that would exceed quota return `429 Too Many Requests`.

**Backing store:** Postgres tables on the shared in-cluster Postgres instance. Each queue is a table managed by the queue service.

**Durability:** Same as the database — data lives on a Kubernetes PersistentVolume and is not backed up automatically.

See [components/queue-service.md](../components/queue-service.md) for the full component spec.

---

## Persistence Lifecycle

### On First Deploy with Persistence

1. Morsel detects newly declared persistence resources
2. Provisions them (creates Postgres database/user, allocates blob quota namespace, registers queue service access)
3. Updates the app pod spec (adds PGBouncer sidecar for database, mounts service account token for blob/queue)
4. Deploy proceeds with the fully provisioned resources available

### On App Update

Re-deploying with the same persistence declarations does nothing — resources are already provisioned. Adding a new resource type provisions it. Removing a resource type triggers permanence checks.

### On App Deletion

- Non-permanent resources enter the grace period (default 30 days) and are purged after expiry
- Permanent resources are retained until the operator explicitly removes them with `?force=true`

### During Hibernation

All persistence is fully retained during hibernation. The app's data is not affected by scaling to zero or by how long the app has been hibernated. PGBouncer sidecar is removed when the pod scales to zero and recreated on wake — no database reconnection is required from the app.

---

## What Morsel Does Not Provide

| Concern | Expectation |
|---|---|
| Caching (Redis, Memcached) | Developer provisions and manages their own |
| App secrets | Developer manages their own — environment variables or a secrets manager of their choice |
| Automated backups | Not provided. App owners responsible for data they cannot afford to lose. |
| Topic/pub-sub semantics | Deploy a broker as a Morsel app |
| Multi-database engines | Postgres only |

---

## Component Contributions

### Control Plane
Owns persistence provisioning, resource lifecycle, and permanence enforcement at the API layer. See [components/control-plane.md — Persistence](../components/control-plane.md).

### Blob Service
Implements the blob storage API, enforces quota, manages object storage interactions. See [components/blob-service.md](../components/blob-service.md).

### Queue Service
Implements the queue API, manages Postgres tables, enforces queue storage quota. See [components/queue-service.md](../components/queue-service.md).

### Database Service
Manages shared Postgres instance, per-app database and user provisioning, PGBouncer sidecar configuration. See [components/database-service.md](../components/database-service.md).

---

Up: [Index](README.md) · Prev: [Networking](networking.md) · Next: [Control Plane](../components/control-plane.md)
