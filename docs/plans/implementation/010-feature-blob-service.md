# Feature 10 — Blob Service

_Delivers: apps can call `blob.morsel.internal` to store and retrieve objects._

**Direct dependencies:** [F07](007-feature-bootstrap-local-platform.md)

> Can be developed in parallel with F08, F09, F11.

## Tasks

- [ ] `cmd/blob-service/main.go` — HTTP server with graceful shutdown
- [ ] `TokenReview` caller identity resolution — map pod service account to `{repo-slug}/{app-name}`
- [ ] Key namespacing — prepend `{repo-slug}/{app-name}/` before every storage operation
- [ ] `LocalPlatform.Blobs()` — filesystem implementation; root at `~/.morsel/local/blobs/`
- [ ] SQLite quota tracking database (separate file from Morsel API)
- [ ] `GET /objects/{key}`, `PUT /objects/{key}`, `DELETE /objects/{key}` endpoints
- [ ] `GET /objects?prefix=&cursor=` — paginated key listing
- [ ] `PUT` quota check — reject writes that would exceed app's byte limit; return `429` with `blob_quota_exceeded`
- [ ] Internal quota-push endpoint — receive updated limits from Morsel API on tier change
- [ ] Blob service registration in `morsel-services` namespace during bootstrap
