Up: [Index](README.md) · Prev: [Permanence](permanence.md) · Next: [Deployment](../platform-features/deployment.md)

---

# Convention — REST API

> **Status:** Draft · **Date:** May 2026

---

## Summary

The Morsel API follows two conventions that apply to all endpoints: async operations and the error model.

Long-running operations return `202 Accepted` immediately with a location to poll — callers never block waiting for deploys or provisioning. All errors return a consistent JSON structure with a stable machine-readable code, a human message, and a remedy that always tells the caller what to do next.

---

## Async Operations

Operations that take more than a moment — deploys, persistence provisioning, deletions, approval reconciliation — are asynchronous.

### The Pattern

Every async operation follows the same shape:

**1. Submit — returns immediately**
```
POST /api/repos/:slug/apps
→ 202 Accepted
  Location: /api/repos/my-repo/apps/my-demo-app/operations/op_abc123
  Retry-After: 5
{
  "operation_id": "op_abc123",
  ...
}
```

**2. Poll — until terminal state**
```
GET /api/repos/my-repo/apps/my-demo-app/operations/op_abc123
→ 200 OK
{
  "id": "op_abc123",
  "type": "deploy",
  "status": "pending",
  "progress": "waiting for pod ready",
  "created_at": "2026-05-26T10:00:00Z",
  "completed_at": null,
  "error": null
}
```

**3. Terminal — poll stops**
```
{
  "id": "op_abc123",
  "type": "deploy",
  "status": "complete",
  "progress": "deployed sha256:abc123",
  "created_at": "2026-05-26T10:00:00Z",
  "completed_at": "2026-05-26T10:00:42Z",
  "error": null
}
```

### Operation States

| Status | Meaning | Poll again? |
|---|---|---|
| `pending` | Operation in progress | Yes |
| `complete` | Operation succeeded | No |
| `failed` | Operation failed; `error` field populated | No |

### Operation Types

| Type | Triggered by |
|---|---|
| `deploy` | `POST /api/repos/:slug/apps` |
| `provision` | First deploy with persistence declared |
| `delete` | `DELETE /api/repos/:slug/apps/:name` |

### Retry-After

The `Retry-After` header on the initial `202` response gives the recommended poll interval in seconds. Callers should respect this value. The Morsel CLI and GitHub Actions workflow both poll at the recommended interval.

Typical values:
- Deploy: `5` seconds
- Provision (first deploy with database): `10` seconds
- Delete: `5` seconds

### Inline Results on 202

The `202` response body includes whatever is known at submission time — applied changes, pending approvals, ignored fields. Callers do not need to poll if they only care about submission outcome, not completion:

```json
{
  "operation_id": "op_abc123",
  "applied": {
    "image": "sha256:abc123",
    "private": true
  },
  "pending_approval": {
    "tier": {
      "id": "apr_abc123",
      "current_value": "small",
      "requested_value": "medium"
    }
  },
  "ignored": {}
}
```

This means a CI workflow can report approval warnings without waiting for the deploy to finish.

### Approval Reconciliation

When the operator approves a batch of changes, reconciliation for each affected app is also async. The batch response returns operation IDs for each app being reconciled:

```json
{
  "approved": ["apr_abc123"],
  "rejected": ["apr_abc124"],
  "ignored": ["apr_abc125"],
  "reconciling": ["apr_abc123"]
}
```

The reconciliation operation is polled via the same `GET /api/repos/:slug/apps/:name/operations/:id` endpoint.

### Error on Async Failure

When an operation fails, the poll response carries the structured error in the `error` field (see Error Model below):

```json
{
  "id": "op_abc123",
  "type": "deploy",
  "status": "failed",
  "completed_at": "2026-05-26T10:01:00Z",
  "error": {
    "code": "deploy_failed",
    "message": "rollout of my-demo-app did not complete within 60s",
    "remedy": "check app logs for startup errors. rolling back to last healthy image.",
    "context": {
      "failed_image": "sha256:abc123",
      "rollback_image": "sha256:def456",
      "rollback_operation": "op_xyz789"
    }
  }
}
```

### CLI Behaviour

The Morsel CLI polls automatically and streams progress to the terminal:

```
  Deploying my-demo-app…
    provisioning database
    waiting for pod ready
✓ my-demo-app  deployed sha256:abc123  (42s)
```

The GitHub Actions workflow polls and emits annotations on failure — see [platform-features/deployment.md](../platform-features/deployment.md).

---

## Error Model

All Morsel API errors return a consistent JSON structure with a stable machine-readable code, a human-readable message, and a remedy that always tells the caller what to do next. Error formatting is the client's responsibility — the API provides structured data. Pod logs are never included in error responses.

### Error Response Shape

```json
{
  "error": {
    "code": "quota_exceeded",
    "message": "repo org/my-repo has reached its app limit (2/2) on the default tier",
    "remedy": "contact your platform operator to request a tier upgrade",
    "context": {
      "repo": "org/my-repo",
      "current": 2,
      "limit": 2,
      "tier": "default"
    }
  }
}
```

| Field | Description |
|---|---|
| `code` | Machine-readable stable identifier. Clients branch on this for formatting and handling. Never changes for a given error scenario. |
| `message` | Human-readable description of what went wrong. May change between versions. |
| `remedy` | What the caller should do next. Always present — no error is returned without a path forward. |
| `context` | Structured data specific to the error type. Present when it helps the client render a better message. May be omitted if no additional context is useful. |

### Error Codes

| Code | HTTP status | Scenario |
|---|---|---|
| `quota_exceeded` | 409 | App count, CPU, memory, blob, database, or queue storage limit reached |
| `permanent_resource` | 409 | Attempt to remove a resource marked permanent without the two-step process |
| `invalid_token` | 401 | OIDC or Morsel token invalid, expired, or malformed |
| `repo_mismatch` | 403 | Token repo claim does not match the app's registered repo |
| `insufficient_role` | 403 | Operation requires operator role; caller has developer role |
| `image_not_found` | 422 | Image digest not found in staging registry |
| `deploy_failed` | 422 | Kubernetes rollout did not complete — health checks did not pass within timeout |
| `immutable_field` | 422 | Attempt to change a field that cannot be changed after bootstrap |
| `tier_demotion_conflict` | 409 | Demotion would put the repo over the new tier's limits |
| `platform_unavailable` | 503 | Morsel API cannot reach a required platform service |
| `budget_soft_limit` | 503 | Wake blocked — estimated spend has reached the soft limit threshold (default 90% of monthly ceiling) |
| `budget_hard_limit` | 503 | Wake blocked — estimated spend has reached or exceeded the monthly ceiling |

`approval_required` is not an error. Protected config changes return `202 Accepted` with pending approval details in the response body. The app deploys at its currently approved config and the workflow succeeds.

### Deploy Failure and Rollback

When a deploy fails, Morsel automatically rolls back to the `last-healthy` image. The error response includes the rollback operation ID so the caller can poll progress:

```json
{
  "error": {
    "code": "deploy_failed",
    "message": "rollout of my-demo-app did not complete within 60s",
    "remedy": "check app logs for startup errors. rolling back to last healthy image.",
    "context": {
      "failed_image": "sha256:abc123",
      "rollback_image": "sha256:def456",
      "rollback_operation": "op_xyz789"
    }
  }
}
```

If no `last-healthy` image exists (first deploy failure), the app is left in `failed` state with no rollback. The remedy directs the developer to fix the image and redeploy.

### Pod Logs Are Never Included

Pod logs are never included in error responses. This prevents accidental leakage of sensitive data (secrets, credentials, PII) through the deploy pipeline into CI logs, Slack notifications, or other error aggregators.

Developers fetch logs separately:
```
morsel app logs my-demo-app
```

### GitHub Actions Error Surfacing

The workflow emits errors at two levels:

**Annotations** — critical errors that require immediate attention. Appear inline in the GitHub Actions UI and on the pull request:
```
::error title=Deploy failed: my-demo-app::Rollout did not complete within 60s. Check app logs for details.
::error title=Quota exceeded: my-demo-app::repo org/my-repo has reached its app limit (2/2). Contact your platform operator.
```

**Step output** — full structured error JSON written to the job log for debugging.

The workflow exits non-zero on any error — GitHub marks the job failed.

`approval_required` emits a warning annotation — the job still succeeds:
```
::warning title=Approval required: my-demo-app::tier change to 'medium' is pending operator approval. App running at current tier 'small'.
```

### CLI Error Formatting

The CLI formats structured errors for terminal display:

```
✗ Deploy failed: my-demo-app
  Rollout did not complete within 60s — health checks not passing
  Check app logs: morsel app logs my-demo-app
```

```
✗ Quota exceeded: my-demo-app
  repo org/my-repo has reached its app limit (2/2) on the default tier
  Contact your platform operator to request a tier upgrade
```

The CLI uses the `code` field to select the display template and the `context` fields to populate it. The `message` is used as a fallback for unknown error codes.

---

Up: [Index](README.md) · Prev: [Permanence](permanence.md) · Next: [Deployment](../platform-features/deployment.md)
