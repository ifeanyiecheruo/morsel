Up: [Index](README.md) · Prev: [Blob Service](blob-service.md) · Next: [Database Service](database-service.md)

---

# Component — Queue Service

> **Status:** Draft · **Date:** May 2026

---

## Overview

The queue service is a lightweight HTTP message queue backed by Postgres tables, running in the `morsel` namespace. Apps create queues at runtime and enqueue/dequeue messages via a fixed internal hostname. The service enforces per-app namespace isolation, storage quota, and self-consuming queue detection for worker hibernation.

---

## Component Diagram

```
App pod (app namespace)
  │
  │  HTTP  PUT    /queues/{name}
  │        POST   /queues/{name}/messages
  │        GET    /queues/{name}/messages/next
  │        DELETE /queues/{name}/messages/{id}
  │        GET    /queues/{name}/depth
  │        GET    /queues
  ▼
queue.morsel.internal (ClusterIP Service)
  │
  ┌──────────────────────────────────────────────┐
  │ Queue Service (morsel namespace)             │
  │                                              │
  │  HTTP handler                                │
  │    ├── Identify caller (pod SA token)        │
  │    ├── Namespace queue: repo__app__name      │
  │    ├── Check / update quota (Postgres)       │
  │    ├── Enqueue / dequeue (Postgres tables)   │
  │    └── Track self-consume window             │
  │                                              │
  │  Shared Postgres instance                    │
  │    queue_{namespace}: messages table         │
  │    queue_usage: {namespace → bytes}          │
  └──────────────────────────────────────────────┘
```

---

## Queue Naming

Queue names in Postgres are prefixed with the caller's repo and app:

```
Postgres table name:  {repo-slug}__{app-name}__{queue-name}
```

Double underscores are used as separators to avoid collisions with hyphenated repo or app names. The queue service derives the prefix from the caller's pod service account token. Apps address queues by their short name only — the full prefixed name is never visible to the app.

---

## Personas

**Developers** access queues from worker and HTTP app code via plain HTTP to `queue.morsel.internal`. Queue names are chosen freely at runtime — no declaration required.

**Operators** observe queue depths and idle status via the admin UI. Hibernation decisions for worker apps are based on queue idle status.

---

## API

All requests are authenticated by the pod's projected Kubernetes service account token.

```
PUT /queues/{name}
```
Create a queue (idempotent). Returns `200 OK` if already exists.

```
DELETE /queues/{name}
```
Delete a queue and all its messages.

```
GET /queues
```
List all queues for the calling app, with depth and idle status.

```json
{
  "queues": [
    { "name": "jobs",          "depth": 14, "idle": false },
    { "name": "internal-loop", "depth": 47, "idle": true  }
  ]
}
```

```
POST /queues/{name}/messages
Content-Type: application/octet-stream
{body}
```
Enqueue a message. Returns `429 Too Many Requests` if total queue storage would exceed quota.

```
GET /queues/{name}/messages/next
```
Dequeue the next message. Briefly polls if the queue is empty (up to 5 seconds). Returns `204 No Content` if still empty after polling. The message is not removed until acknowledged.

```json
{
  "id": "msg_abc123",
  "body": "base64-encoded-payload",
  "enqueued_at": "2026-05-26T10:00:00Z"
}
```

```
DELETE /queues/{name}/messages/{id}
```
Acknowledge and remove a message. Idempotent — safe to call multiple times.

```
GET /queues/{name}/depth
```
Returns the count of pending (unacknowledged) messages.

```json
{ "depth": 14 }
```

---

## Functionality

### Caller Identification and Namespacing

Identical to the blob service — the pod's projected service account token is resolved via Kubernetes TokenReview API. Queue names are namespaced as `{repo-slug}__{app-name}__{queue-name}` internally. The app uses bare names.

### At-Least-Once Delivery

Messages remain in the queue until explicitly acknowledged with `DELETE /queues/{name}/messages/{id}`. If a consumer crashes after dequeuing but before acknowledging, the message becomes eligible for redelivery after a visibility timeout (platform-configurable, default 30 seconds).

This gives at-least-once delivery semantics. Apps that require exactly-once processing must implement their own idempotency.

### Quota Enforcement

Total message storage across all of the app's queues is tracked per app. Enqueue requests that would push total storage over the limit return `429 Too Many Requests`.

Storage is calculated as the byte size of the message body. Message metadata (ID, timestamp) is not counted toward quota.

### Self-Consuming Queue Detection

A queue tracks `last_external_enqueue_at` — the timestamp of the most recent enqueue call whose sender identity (resolved via Kubernetes `TokenReview`) differs from the queue-owning app. A queue is marked `idle: true` when no external enqueue has occurred within the app's `idle_after` window. `last_external_enqueue_at = NULL` (never received external work) is treated as idle.

This prevents a worker from keeping itself alive by processing only its own messages. Queue depth is irrelevant to the idle flag — a deep queue filled entirely by the owning app is still idle.

```json
{ "name": "internal-loop", "depth": 47, "idle": true }
```

The queue has 47 messages but is marked idle because all recent enqueues came from the owning app itself.

### No Topic Semantics

Each message is delivered to exactly one consumer. Fan-out (one message delivered to multiple consumers) is not supported. Apps that require pub/sub should deploy a message broker as a Morsel app.

---

## Dollar Cost

| Resource | Allocation | Monthly estimate |
|---|---|---|
| CPU request | 0.1 cores | ~$2 |
| Memory request | 128 MB | ~$0.50 |
| Postgres storage (queue tables) | Shared with database service PV | Included in database service cost |

Queue messages are stored in Postgres tables on the shared in-cluster Postgres instance. There is no separate storage cost for queues — they share the database PV.

---

## Operational Cost

- **Upgrades** — rolling pod replacement during platform upgrade. Brief queue service unavailability (seconds). Messages in transit are not lost — they remain in Postgres.
- **Queue growth monitoring** — unusually deep queues (worker not keeping up) are visible in the admin UI.
- **Table maintenance** — acknowledged messages are hard-deleted immediately. No separate vacuum job required for queue tables.

---

## Scalability

The queue service is single-replica, backed by Postgres. At the target scale (non-production workloads, tens of apps with queues), this is adequate. Queue throughput is bounded by Postgres write throughput — typically thousands of messages per second on in-cluster Postgres.

Scaling beyond this would require a purpose-built queue store. Not planned.

---

## Security

- Caller identity from Kubernetes service account token — app cannot impersonate another app
- Queue namespacing enforced server-side
- No queue names or message contents leak between apps
- Queue service has minimal Kubernetes RBAC — TokenReview only
- Message bodies are stored in Postgres — subject to the same PV security as the database service

---

## Performance

- Enqueue: single Postgres `INSERT` — sub-millisecond
- Dequeue (`messages/next`): Postgres `SELECT FOR UPDATE SKIP LOCKED` — sub-millisecond if queue has messages; up to 5-second long poll if empty
- Depth: single Postgres `COUNT` — sub-millisecond for typical queue sizes
- Idle flag check: single Postgres read of `last_external_enqueue_at` per queue — sub-millisecond

---

## Platform Feature Support

### Hibernation

The queue service is the data source for worker hibernation decisions. The control plane watcher polls an internal endpoint at each tick to check idle status per worker app:

```http
GET /internal/queues/{namespace}/{app-name}
Authorization: Bearer <queue-internal-token>
```

Response:

```json
{
  "queues": [
    { "name": "jobs",          "depth": 14, "idle": false },
    { "name": "internal-loop", "depth": 47, "idle": true  }
  ]
}
```

A worker hibernates when all of its queues report `idle: true`. This endpoint uses the same `queue-internal-token` bearer token as the quota endpoint — the control plane never uses app pod service account tokens.

A queue's `idle` flag is driven by the `last_external_enqueue_at` timestamp stored per queue. An enqueue is "external" when the enqueueing pod's service account identity (resolved via `TokenReview`) differs from the queue-owning app. A queue is `idle: true` when no external enqueue has occurred within the app's idle window. Queue depth is irrelevant to the idle flag — a deep queue filled entirely by the owning app is still idle.

This prevents a worker from keeping itself alive indefinitely by processing its own messages. `last_external_enqueue_at = NULL` (never received external work) is treated as idle.

```json
{
  "queues": [
    { "name": "jobs",          "depth": 14, "idle": false },
    { "name": "internal-loop", "depth": 47, "idle": true  }
  ]
}
```

In this example the worker stays alive because `jobs` has external messages pending, even though `internal-loop` is self-consuming.

Workers wake when an external message is enqueued. The queue service notifies the control plane watcher, which scales the worker deployment to 1. The first message may wait in the queue for the cold-start duration before being processed.

See [platform-features/hibernation.md](../platform-features/hibernation.md) for the full hibernation lifecycle.

### Cost Controls

The queue service enforces per-app total queue storage quota at enqueue time. Quota limits are pushed by the control plane on tier changes:

```http
POST /internal/quota/{namespace}/{app-name}
Authorization: Bearer <queue-internal-token>

{ "queue_bytes": 1073741824 }
```

The queue service rejects calls without a valid token with `401 Unauthorized`. The token is a shared secret provisioned at bootstrap, stored in the platform SecretStore under the key `queue-internal-token`, and mounted as the `QUEUE_INTERNAL_TOKEN` environment variable in both the control plane pod and the queue service pod. It is independent of the blob service token.

See [platform-features/cost-controls.md](../platform-features/cost-controls.md).

### Persistence
Queues are one of the three managed persistence types. Queue table creation, deletion, and lifecycle (grace period on app deletion) are managed by the control plane. The queue service creates and drops Postgres tables on `PUT /queues/{name}` and `DELETE /queues/{name}`. See [platform-features/persistence.md](../platform-features/persistence.md).

---

Up: [Index](README.md) · Prev: [Blob Service](blob-service.md) · Next: [Database Service](database-service.md)
