# Feature 12 — Queue Service

_Delivers: apps can enqueue and dequeue messages via `queue.morsel.internal`._

**Direct dependencies:** [F07](007-feature-bootstrap-local-platform.md)

> No dependency on F11 (Database Service). The queue service uses SQLite (one file per queue) instead of a shared Postgres instance, making it independent of and parallel with F10 and F11.
> Can be developed in parallel with F10, F11, F14, F15 once F07 is done. F13 (Hibernation) blocks on this for worker idle detection.

## Tasks

- [x] Add `Queue` interface and types to `cmd/morsel-ctrl-plane/internal/queue/queue.go` (named `Queue`/`LocalQueue` rather than `QueueStore`/`LocalQueueStore`; sqlc-generated queries in `internal/queue/queries/`)
- [x] `Queues(repoSlug, appName string) queue.Queue` on `platform.Platform` — `LocalQueue` backed by SQLite files under `~/.morsel/local/queues/`; namespace derived via `kube.AppNamespace`
- [x] `svc queue` subcommand in `cmd/morsel-ctrl-plane/svc_queue.go` — HTTP server with graceful shutdown (integrated into control-plane binary rather than a separate `cmd/queue-service`)
- [x] `TokenReview` caller identity (same pattern as blob service)
- [x] SQLite schema: one `.db` file per queue at `{data-dir}/{k8s-namespace}/{queue-name}.db`; per-app `quota.db`
- [x] `PUT /queues/{name}` — create queue (idempotent)
- [x] `DELETE /queues/{name}` — delete queue and all messages
- [x] `GET /queues` — list caller's queues with depth and idle status
- [x] `POST /queues/{name}/messages` — enqueue; check storage quota
- [x] `GET /queues/{name}/messages/next` — dequeue with 5-second long poll (in-process channel); visibility timeout 30s
- [x] `DELETE /queues/{name}/messages/{id}` — explicit ack (hard delete)
- [x] `GET /queues/{name}/depth` — pending message count
- [x] Storage quota enforcement — track total bytes in `quota.db`; reject enqueue at limit with `429`
- [x] Internal quota-push endpoint — `POST /internal/quota/{namespace}/{app-name}`; authenticated with `QUEUE_INTERNAL_TOKEN`
- [x] `GET /internal/queues/{namespace}/{app-name}` — idle status for all queues owned by app; authenticated with `QUEUE_INTERNAL_TOKEN`; used by control plane hibernation watcher
