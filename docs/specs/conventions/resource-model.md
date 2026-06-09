Up: [Index](README.md) · Prev: [Operator Scenarios](../scenarios/operator.md) · Next: [Identity & Ownership](identity-ownership.md)

---

# Convention — Resource Model

> **Status:** Draft · **Date:** May 2026

---

## Summary

All platform-managed resources (database, blob storage, queues) belong exclusively to one app. Apps access those resources via fixed internal hostnames with fixed credentials — no configuration, no SDK, no credential management required. Isolation between apps is enforced by the platform using pod identity, not by credentials the app supplies.

---

## All Resources Belong to One App

A resource is provisioned for a specific app and is not shared across apps. One app declares a database, that app gets a database. Another app declaring a database gets a separate, isolated database. There is no concept of a shared resource that two apps co-own.

This is enforced at every layer:
- Kubernetes: each app runs in its own namespace with its own service account
- Blob service: objects are namespaced by `repo-slug/app-name/` — derived from pod identity, invisible to the app
- Queue service: queue names are scoped to the calling app's pod identity — two apps can use the same queue name without collision
- Database: each app gets its own Postgres database and Postgres user, with access granted only to that database

Resources are declared in the app's `morsel.json`. They are created when the app is first deployed and persist according to the `permanent` flag and grace period rules (see [conventions/permanence.md](permanence.md)).

---

## Fixed Hostnames — Zero Configuration

Apps access platform services via fixed internal hostnames that never change:

| Service | Hostname | Protocol |
|---|---|---|
| Blob storage | `blob.morsel.internal` | HTTP |
| Queue | `queue.morsel.internal` | HTTP |
| Database | `database.morsel.internal` | Postgres wire protocol |

These hostnames are stable across deploys, upgrades, and platform migrations. An app does not need to discover where its services are — they are always at the same address.

The hostnames are resolvable only within the cluster VPC. Apps running outside the cluster (local development, external services) cannot reach them directly.

---

## Fixed Credentials — Zero Credential Management

Database connections use fixed constants that never change and require no configuration:

| Constant | Value |
|---|---|
| Host | `database.morsel.internal` |
| Port | `5432` |
| Database | `morsel` |
| Username | `morsel` |
| Password | `morsel` |

These are not real Postgres credentials. They are PGBouncer conventions. Under the hood, the PGBouncer sidecar running alongside the app pod holds the real per-app generated credentials in a Kubernetes Secret and uses them to authenticate to the shared Postgres instance. The app is not aware of this indirection.

The blob and queue services require no credentials at all. The caller's identity is determined by their pod's Kubernetes service account — the app passes nothing.

---

## Namespace Isolation via Pod Identity

Apps do not prove their identity by presenting a credential. The platform identifies callers by the Kubernetes service account of the calling pod. This happens automatically — the app does nothing special.

Each app runs with a dedicated Kubernetes service account in its own namespace. The platform services (blob, queue) use the pod's projected service account token to determine which app is calling and apply the correct namespace prefix and quota limits.

Consequences:
- An app cannot access another app's blobs or queues regardless of what paths or queue names it uses
- An app cannot access another app's database regardless of what credentials it presents — Postgres user grants are scoped to that app's own database
- A developer who `exec`s into a pod and connects to `database.morsel.internal` is still routed through their app's PGBouncer and cannot reach another app's sidecar — Kubernetes NetworkPolicy prevents it

---

## What the App Sees vs. What the Platform Does

| What the app sees | What the platform does |
|---|---|
| `GET blob.morsel.internal/objects/my-file` | Prefixes key: `org-my-repo/my-app/my-file`, checks quota, reads from object storage |
| `POST queue.morsel.internal/queues/jobs/messages` | Namespaces queue as `org-my-repo__my-app__jobs`, checks quota, inserts into Postgres |
| `psql -h database.morsel.internal -U morsel -d morsel` | PGBouncer maps `morsel/morsel/morsel` → real per-app credentials → `org-my-repo-my-app` database |

The app never sees prefixes, namespaces, real credentials, or backing store details. Platform complexity is invisible to the app.

---

## Implications for App Design

- Apps should treat `blob.morsel.internal`, `queue.morsel.internal`, and `database.morsel.internal` as always-available internal services
- Apps should not attempt to share resources with other apps through the platform — the platform prevents it
- Apps that need to share data with another app should do so through their public HTTPS URLs or by putting data in a format accessible to both (e.g., a shared external store the apps own independently)
- Apps should not attempt to access platform backing stores directly using cloud SDK credentials — this would bypass quota enforcement and namespace isolation

---

Up: [Index](README.md) · Prev: [Operator Scenarios](../scenarios/operator.md) · Next: [Identity & Ownership](identity-ownership.md)
