# Feature 12 — Queue Service

_Delivers: apps can enqueue and dequeue messages via `queue.morsel.internal`._

**Direct dependencies:** [F07](007-feature-bootstrap-local-platform.md), [F11](011-feature-database-service.md)

> Can be developed in parallel with F14, F15 once F11 is done. F13 (Hibernation) blocks on this for worker idle detection.

## Tasks

- [ ] `cmd/queue-service/main.go` — HTTP server
- [ ] `TokenReview` caller identity (same pattern as blob service)
- [ ] Postgres-backed queue tables with namespace prefix `{repo-slug}__{app-name}__{queue-name}`
- [ ] `PUT /queues/{name}` — create queue
- [ ] `DELETE /queues/{name}` — delete queue
- [ ] `GET /queues` — list caller's queues with idle status
- [ ] `POST /queues/{name}/messages` — enqueue; check storage quota
- [ ] `GET /queues/{name}/messages/next` — dequeue (at-most-once delivery)
- [ ] `DELETE /queues/{name}/messages/{id}` — explicit ack
- [ ] `GET /queues/{name}/depth` — message count
- [ ] Storage quota enforcement — track total bytes per app; reject enqueue at limit
- [ ] Internal quota-push endpoint — same pattern as blob service
- [ ] `GET /internal/queues/{namespace}/{app-name}` — return idle status for all queues owned by app; authenticated with `queue-internal-token`; used by control plane hibernation watcher
