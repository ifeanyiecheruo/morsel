# Feature 02 — Morsel API: HTTP Server Skeleton

_Delivers: running API binary; `curl /healthz` returns 200._

**Direct dependencies:** [F01](001-feature-repository-foundation.md)

## Tasks

- [x] `cmd/morsel-api/main.go` — HTTP server with graceful shutdown on SIGTERM
- [x] Structured error middleware — all responses follow `{"error": "code", "message": "...", "details": {...}}` shape per `conventions/rest.md`
- [x] `GET /healthz` — returns `{"status": "ok"}`
- [x] SQLite connection pool with WAL mode enabled
- [x] Migration runner — versioned SQL files applied at startup; idempotent
- [x] Initial schema: `repos`, `apps`, `operations` tables
- [x] Structured request logging (method, path, status, latency)
