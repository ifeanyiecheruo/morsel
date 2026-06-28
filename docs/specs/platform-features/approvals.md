Up: [Index](README.md) · Prev: [Cost Controls](cost-controls.md) · Next: [Networking](networking.md)

---

# Platform Feature — Approvals

> **Status:** Draft · **Date:** May 2026

---

## Summary

A small set of protected configuration fields require operator approval before taking effect. Developers declare desired state freely in `morsel.json`. When a protected field changes, the current approved value stays in effect and a pending approval is created. The app deploys successfully at its current config — it is never blocked by a pending approval. The operator reviews and acts on approvals in batch.

---

## Why Approvals Exist

Protected fields affect resource consumption or cost in ways that should not be unilaterally self-served. Without an approval gate, a developer could freely escalate their app's tier or exceed quota limits, undermining the cost model.

The approval design is deliberately non-blocking: the developer is never prevented from deploying. The app runs at its best currently-approved configuration while the change is pending. This keeps deployment velocity high while giving the operator oversight over resource escalations.

---

## Protected Fields

| Field | Reason |
| --- | --- |
| `tier` | Affects compute resource allocation |
| Persistence additions beyond current quota | Would exceed approved resource limits |
| App count beyond repo limit | Would exceed approved app limit |

All other fields (`type`, `private`, `idle_after`, `health_check`, `schedule`, `timeout`, `retries`) are applied immediately on deploy without approval.

---

## Approval Lifecycle

### 1. Developer Declares Change

Developer updates `morsel.json` and pushes:

```json
{ "tier": "medium" }
```

### 2. Deploy Succeeds at Current Config

The deploy API returns `202 Accepted` with the applied changes and the pending approval in the response body:

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
      "requested_value": "medium",
      "expires_at": "2026-06-26T10:00:00Z"
    }
  }
}
```

The app deploys and runs at `tier: small`. The tier change to `medium` is pending.

### 3. Warning Annotation in CI

GitHub Actions emits a warning annotation (job still succeeds):

```text
⚠ my-app — tier change to 'medium' pending operator approval — running at 'small'
```

### 4. Operator Reviews

The operator sees the pending approval in the admin UI → Approvals list:

```text
org/my-repo  /  my-app  |  tier  |  small → medium  |  requested 2026-05-26
```

### 5. Operator Acts

The operator approves, rejects, or ignores:

- **Approve** — tier change applied immediately via reconciliation. Approval cleared.
- **Reject** — with optional reason. Developer sees rejection reason in next deploy output. Approval cleared.
- **Ignore** — decision deferred. Approval stays pending. Expiry clock continues.

### 6. Reconciliation on Approval

When approved, Morsel applies the tier change in a single reconciliation pass. The app is updated in place — no new image build required. The operation is async; the operator can poll progress.

---

## Batch Actions

The operator actions multiple approvals in one call — the normal workflow is a weekly batch review, not per-request intervention:

```json
POST /api/operator/approvals/batch
{
  "decisions": [
    { "id": "apr_abc123", "action": "approve" },
    { "id": "apr_abc124", "action": "reject", "reason": "use small tier for demo apps" },
    { "id": "apr_abc125", "action": "ignore" }
  ]
}
```

Approved changes per app are applied in a single reconciliation pass — multiple approved changes for the same app are consolidated.

---

## Approval Granularity

Approvals are per field, not per deploy. A developer can request a tier change and a persistence addition in the same deploy. The operator can approve the tier change and reject the persistence addition independently. The app applies each approved change as it is actioned.

### Coalescing

There is at most one pending approval per protected field per app. If the same field is changed again before the operator acts, the existing approval's `requested_value` is updated in place — no second approval is created and no history of intermediate values is kept. The operator always sees the current approved value versus the latest requested value.

For example: a developer pushes `tier: medium`, then pushes `tier: large` before the operator reviews. The operator sees `small → large`. The intermediate request for `medium` is gone.

This means the expiry clock does not reset on each update — it was set when the first change to that field was requested. A developer who repeatedly updates a pending field does not get additional time before expiry.

---

## Expiry

Approvals expire after a configurable period (default 30 days, set via `PATCH /api/operator/config`). Expired approvals are automatically rejected and cleared. The developer resubmits if still needed — the next deploy that includes the protected field change creates a new approval.

Expiry prevents stale approvals from accumulating indefinitely and ensures the operator's queue stays manageable.

---

## Developer Visibility

Developers can view their own repo's pending approvals:

```text
GET /api/repos/:slug/approvals
```

This lets developers check the status of pending approvals without contacting the operator.

---

## Approval State Machine

```text
pending ──→ approved  (operator approves → reconciliation runs)
        ──→ rejected  (operator rejects or approval expires)
        ──→ pending   (operator ignores → stays pending)
```

There is no "cancelled" state. If a developer changes their mind and removes the protected field from `morsel.json`, the pending approval becomes irrelevant but is not automatically cancelled — it expires after 30 days or the operator rejects it.

---

## Component Contributions

### Control Plane

Owns the approval data model, approval creation on protected field changes, reconciliation on approval, and all approval API endpoints. See [components/control-plane.md — Approvals](../components/control-plane.md).

### Admin UI

Provides the approvals list view, batch action UI, and per-approval detail. See [components/admin-ui.md — Approvals](../components/admin-ui.md).

### CLI

Surfaces approval warnings in terminal output and polls reconciliation operations. See [components/cli.md — Approvals](../components/cli.md).

---

Up: [Index](README.md) · Prev: [Cost Controls](cost-controls.md) · Next: [Networking](networking.md)
