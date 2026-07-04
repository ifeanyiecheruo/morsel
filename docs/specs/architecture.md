Up: [Index](README.md) · Prev: [Principles](principles.md) · Next: [Operator Scenarios](scenarios/operator.md)

---

# Morsel — Architecture

> **Status:** Draft · **Date:** May 2026

---

## Overview

Morsel has one central service and two external actors.

The **control plane** (`morsel-ctrl-plane`) is the center of the system. All operations flow through it. It validates tokens, manages app lifecycle, and applies Kubernetes manifests on behalf of callers. No other actor touches the cluster directly. It also runs background processes: a hibernation watcher that scales idle apps to zero, a wake-on-request proxy that holds inbound requests until a hibernated app comes back up, and a cost enforcement watcher that evaluates estimated spend against the budget ceiling — blocking wakes at the soft limit, force-hibernating apps at the hard limit, and taking daily price snapshots from the platform pricing API (see [platform/gcp.md](platform/gcp.md)).

Developer code runs in **per-app namespaces** inside the cluster, each managed exclusively by the control plane. 

Apps have access to batteries-included building blocks in the **morsel-services namespace**. eg. Blob Service for object storage, Queue Service for async messaging, and SQL Service for a database. Apps depend on these services at runtime; the services themselves have no knowledge of individual apps.

The **operator machine** runs the `morsel` CLI to provision the platform. Day-to-day management is done via CLI commands to the contol plan or the Admin UI.

**GitHub** is where developers work. A git push triggers a CI runner that calls the control plane to stage Docker images and deploy new app configurations

## System Architecture

```text
+----------------------------------------------------------+  +----------------------------+
| GitHub                                                   |  | Operator machine           |
|                                                          |  |  morsel service bootstrap  |
|  +--------------------------------------------------+   |  |  morsel service status     |
|  | Developer Repo                                   |   |  |  browser -> admin UI       |
|  +------------------+-------------------------------+   |  +-------------+--------------+
|                     | git push -> GitHub Actions        |                |
|                     |                                   |                | HTTPS
|  +------------------v-------------------------------+   |                |
|  | CI-Runner                                        |   |                |
|  |  1. Exchange OIDC token -> Morsel token          |   |                |
|  |  2. Build container image                        |   |                |
|  |  3. Push to staging container registry           |   |                |
|  |  4. POST app description to /api/repos/:slug/apps|   |                |
|  +--------------------------------------------------+   |                |
+--------------------------+-------------------------------+                |
                           | HTTPS (outbound from CI)                       |
                           +---------------------------------------------+--+
                                                                         |
- - - - - - - - - - - - - - - - - - - internet boundary - - - - - - - - - - - - - - - - -
                                                                         |
                                                                         v
+----------------------------------------------------------------------------------------+
| Morsel instance                                                                        |
|                                                                                        |
|  Internet-facing:  external LB -> public apps (private: false)                        |
|                    auth gateway -> Admin UI (operators only)                          |
|  VPC-internal:     internal LB -> private apps (private: true, no internet exposure)  |
|                                                                                        |
|  +----------------------------------------------------------------------------------+ |
|  | Kubernetes Cluster                                                               | |
|  |                                                                                  | |
|  |  +----------------------------------------------------------------------------+ | |
|  |  | morsel control plane namespace                                             | | |
|  |  |  morsel-ctrl-plane                                                                | | |
|  |  |  Hibernation watcher                                                       | | |
|  |  |  Wake-on-request proxy                                                     | | |
|  |  |  Cost enforcement watcher                                                  | | |
|  |  |  Admin UI                                                                  | | |
|  |  +----------------------------------------------------------------------------+ | |
|  |                      | deploys / manages                                        | |
|  |                      v                                                          | |
|  |  +--------------------+  +--------------------+                                | |
|  |  | app namespace:     |  | app namespace:     |  ...                           | |
|  |  | org-my-repo        |  | org-other-repo     |                                | |
|  |  |                    |  |                    |                                | |
|  |  |  Deployment/       |  |  Deployment/       |                                | |
|  |  |  CronJob/Worker    |  |  CronJob/Worker    |                                | |
|  |  +--------------------+  +--------------------+                                | |
|  |                      | calls services                                           | |
|  |                      v                                                          | |
|  |  +----------------------------------------------------------------------------+ | |
|  |  | morsel-services namespace                                                  | | |
|  |  |  Blob Service                                                              | | |
|  |  |  Queue Service                                                             | | |
|  |  |  SQL service                                                               | | |
|  |  +----------------------------------------------------------------------------+ | |
|  +----------------------------------------------------------------------------------+ |
|                                                                                        |
|  +------------------+  +-------------------+                                          |
|  | Image Registry   |  |  Object Storage   |                                          |
|  |  staging/        |  +-------------------+                                          |
|  |  canonical/      |                                                                  |
|  +------------------+                                                                  |
|                                                                                        |
|  +------------------------------------------+                                         |
|  | DNS                                      |                                         |
|  |  *.apps.example.com -> Load Balancer     |                                         |
|  +------------------------------------------+                                         |
+----------------------------------------------------------------------------------------+
```

---

## Components

### Control Plane

The control plane. A Go HTTP service running in the `morsel` namespace. All platform operations flow through it.

Responsibilities:
- OIDC token validation and Morsel token issuance
- App and repo lifecycle (create, update, delete, sync)
- Image staging handshake and registry copy
- Kubernetes manifest apply via `client-go`
- Quota enforcement and approval workflow
- Hibernation watcher and HTTP proxy for wake-on-request
- Cost enforcement watcher (soft/hard limit evaluation, daily price snapshots)
- Certificate provisioning and renewal via ACME/Let's Encrypt
- DNS record management via the configured DNS provider
- Platform state persistence in SQLite on a Kubernetes PersistentVolume

See [components/control-plane.md](components/control-plane.md).

### Admin UI

A server-rendered multipage app served by the control plane and protected by the platform's operator authentication gateway. The operator's web interface for day-to-day platform management.

See [components/admin-ui.md](components/admin-ui.md).

### CLI (`morsel`)

A static Go binary. The operator's sole interface for installing, configuring, and upgrading the platform. No Terraform, kubectl, or gcloud required.

Responsibilities:

- Platform OAuth authentication
- Preflight checks
- Wizard-driven configuration collection
- Cloud resource provisioning (Kubernetes cluster, container registry, secret store, identity, admin auth gateway)
- Platform upgrades (rolling)
- Operator access management
- `morsel lint` for developer `morsel.json` validation
- `morsel app deploy` for local and CI deploys

See [components/cli.md](components/cli.md).

### Blob Service (`blob.morsel.internal`)

A lightweight HTTP object storage service running in the `morsel-services` namespace. Apps access it via a fixed internal hostname with no SDK or credentials.

See [components/blob-service.md](components/blob-service.md).

### Queue Service (`queue.morsel.internal`)

A lightweight HTTP queue service running in the `morsel-services` namespace, backed by SQL service tables. Apps create queues at runtime and enqueue/dequeue via a fixed internal hostname.

See [components/queue-service.md](components/queue-service.md).

### SQL Service (`database.morsel.internal`)

A shared in-cluster SQL instance running in the `morsel-services` namespace, with a PGBouncer sidecar per app. Apps connect using fixed constants — no credentials visible to the developer.

See [components/database-service.md](components/database-service.md).

---

## Networking

Every app in Morsel has a `private` flag (`private: true/false` in `morsel.json`). Private apps being unreachable from the public internet is a first-class design goal — enforced by network topology, not by application-level access control. Even if an app has a bug in its auth layer, a private app cannot be reached from outside the network.

Morsel achieves this by routing the two app types through separate load balancers:

- **Public apps** (`private: false`) are reachable from the internet via an external load balancer at `*.apps.example.com`. TLS is terminated here.
- **Private apps** (`private: true`) are routed only through an internal load balancer scoped to the VPC — no public IP, no internet path, unreachable by construction.

The Admin UI is internet-facing but gated by the platform's operator authentication gateway, restricting access to authenticated operators. The control plane itself has no public port; it only accepts connections from outbound HTTPS calls by CI runners and the operator CLI — it cannot be reached from a browser.

See [platform-features/networking.md](platform-features/networking.md).

---

Up: [Index](README.md) · Prev: [Principles](principles.md) · Next: [Operator Scenarios](scenarios/operator.md)
