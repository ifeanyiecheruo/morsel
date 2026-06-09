Up: [Index](README.md) · Prev: [Idempotency](idempotency.md) · Next: [REST API](rest.md)

---

# Convention — Permanence

> **Status:** Draft · **Date:** May 2026

---

## Summary

Platform-managed persistence resources (database, blob storage, queues) can be marked permanent. A permanent resource is protected from accidental deletion — removing it requires a deliberate two-step process. Non-permanent resources enter a grace period on app or repo deletion and are purged automatically.

---

## The Permanent Flag

Each persistence type in `morsel.json` carries an independent `permanent` flag:

```json
{
  "persistence": {
    "database": { "permanent": true },
    "storage":  { "permanent": false },
    "queues":   { "permanent": false }
  }
}
```

Default is `false` for all resource types. Setting `permanent: true` is a deliberate declaration that this resource holds data that must not be deleted automatically.

The flags are independent — a database can be permanent while blob storage is not.

---

## What Happens on App Deletion

When an app is deleted (via `DELETE /api/repos/:slug/apps/:name`, via sync, or via admin UI):

| Resource type | `permanent: false` | `permanent: true` |
|---|---|---|
| Database | Enters grace period; purged after grace period expires | Kubernetes resources removed; database and data retained until operator explicitly removes with `?force=true` |
| Blob storage | Enters grace period; purged after grace period expires | Kubernetes resources removed; objects retained in platform storage until operator explicitly removes with `?force=true` |
| Queues | Enters grace period; purged after grace period expires | Kubernetes resources removed; queue tables and messages retained until operator explicitly removes with `?force=true` |

The grace period is platform-configurable (default 30 days). During the grace period the app is gone but its data can be recovered by redeploying the app.

---

## The Two-Step Removal Process for Permanent Resources

Removing a permanent resource requires two separate deploys:

**Step 1 — Mark as non-permanent and deploy:**
```json
{ "persistence": { "database": { "permanent": false } } }
```
The app deploys normally. The database is still provisioned. This deploy signals intent to remove.

**Step 2 — Remove the declaration and deploy:**
```json
{ "persistence": {} }
```
On this deploy, the database enters the grace period and will be purged after it expires.

This two-step process is enforced at two layers:
- `morsel lint` catches a single-step removal locally before it reaches CI
- The Morsel API rejects a single-step removal with a `409 Conflict` response

---

## Lint Enforcement

Attempting to remove a permanent resource in a single step is a lint error:

```
✗ api.morsel.json — removing 'database' which is marked permanent
  Remedy:
    1. Set persistence.database.permanent to false
    2. Deploy
    3. Remove the database declaration in a subsequent commit
```

`morsel lint --staged` catches this in a pre-commit hook before the change is committed.

---

## API Enforcement

If lint is bypassed and a single-step removal reaches the API:

```
→ 409 Conflict
{
  "error": "permanent_resource",
  "detail": "persistence.database is marked permanent and cannot be removed by a developer",
  "remedy": "set permanent to false, deploy, then remove the declaration"
}
```

The `?force=true` flag overrides permanent resource protection. It is available to operators only — developers receive 403 if they attempt it.

---

## Grace Period

Non-permanent resources enter the grace period when:
- The app is deleted
- The repo is deleted
- The resource is removed from `morsel.json` after being marked non-permanent

The grace period starts at the moment of deletion, not at the end of any existing TTL. The expiry timestamp is returned in delete responses:

```json
{
  "deleted_at": "2026-05-26T10:00:00Z",
  "persistence_purge_after": "2026-06-25T10:00:00Z"
}
```

During the grace period, the resource is retained and visible in the admin UI as pending purge. The operator can cancel the purge before expiry if needed.

The grace period is set platform-wide by the operator via `PATCH /api/operator/config`. It cannot be set per-app.

---

## Operator Override

The operator can override permanence protection via the `?force=true` query parameter on delete endpoints. This is an escape hatch for cleanup of orphaned apps or repos where the developer is no longer available to perform the two-step process.

```
DELETE /api/repos/:slug/apps/:name?force=true
Authorization: Bearer <operator-morsel-token>
```

Developers receive 403 on any request with `?force=true`.

---

Up: [Index](README.md) · Prev: [Idempotency](idempotency.md) · Next: [REST API](rest.md)
