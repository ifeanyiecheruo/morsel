# Feature 05 — App Lifecycle: API Layer

_Delivers: apps can be created, listed, and deleted through the API (SQLite only — no Kubernetes yet)._

**Direct dependencies:** [F03](003-feature-authentication.md)

## Tasks

- [x] `POST /api/repos/:slug/sync` — upsert declared app list; mark apps absent from list as `deletion_pending`
- [x] `POST /api/repos/:slug/apps` — upsert app record; validate fields; write SQLite row; return `202 Accepted` with operation location
- [x] Async operation model — `operations` table; status polling at `GET /api/repos/:slug/apps/:name/operations/:id`
- [x] `GET /api/repos/:slug/apps` — list apps with status
- [x] `GET /api/repos/:slug/apps/:name` — app detail
- [x] `GET /api/repos/:slug/apps/:name/status` — current runtime state (stub: `unknown` until K8s integration)
- [x] `GET /api/repos/:slug/apps/:name/history` — deploy history from operations log
- [x] `DELETE /api/repos/:slug/apps/:name` — soft-delete; begin 30-day grace period
- [x] `GET /api/repos` — list repos (developer: own only; operator: all with `?all=true`)
- [x] `GET /api/repos/:slug` — repo detail
- [x] Namespace naming function: `{org-slug}-{repo-slug}--{app-name}` (slash → hyphen, preserve existing hyphens)
