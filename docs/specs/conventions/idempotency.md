Up: [Index](README.md) · Prev: [Identity & Ownership](identity-ownership.md) · Next: [Permanence](permanence.md)

---

# Convention — Idempotency

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel operations are safe to repeat. Deploying the same app twice produces the same result as deploying it once. Running `morsel service bootstrap` on an already-provisioned platform changes nothing. The sync endpoint is the declarative reconciliation primitive — callers state what should exist, Morsel makes it so.

---

## Upsert Everywhere

App creation and app update are the same operation. The caller always sends the full desired state; Morsel determines from its own records whether this is a first deploy or an update and acts accordingly. There is no separate "create app" endpoint.

```
POST /api/repos/:slug/apps   →  create if new, update if exists
```

Consequences:
- A deploy workflow does not need to know whether an app already exists
- Re-running a workflow after a partial failure is safe — already-deployed apps are updated in place, not duplicated
- There is no state the caller needs to maintain about whether an app is "registered"

---

## Check-Then-Act in Bootstrap

Every operation performed by `morsel service bootstrap` follows a check-then-act pattern:

1. Check whether the resource already exists in its desired state
2. If yes, skip and report `✓ (already up to date)`
3. If no, create or update, then verify

This means `morsel service bootstrap` is safe to run at any time without risk of duplicating resources, re-running the wizard, or resetting configuration. It is the correct response to any uncertainty about platform state — run it and it will converge to the desired state.

```
✓ State bucket (already up to date)
✓ VPC (already up to date)
  Creating Kubernetes cluster… (this takes 4–6 minutes)
✓ Kubernetes cluster ready
✓ Container registry (already up to date)
```

---

## Sync as Declarative Reconciliation

`POST /api/repos/:slug/sync` is called once per deploy workflow before individual app deploys. The caller passes the complete list of app names currently declared in `.morsel/`. Morsel diffs this against its own records:

- Apps in the declared list but not in Morsel's records: added (no action at sync time — deploy call follows)
- Apps in Morsel's records but not in the declared list: deleted (subject to permanence rules)
- Apps in both: unchanged

```
POST /api/repos/:slug/sync
{ "apps": ["api", "worker", "scheduler"] }

→ { "added": ["scheduler"], "unchanged": ["api", "worker"], "deleted": [] }
```

The sync model means removing an app from `.morsel/` is sufficient to delete it — no explicit delete call is required from the workflow. The workflow always sends the full current state; Morsel handles the diff.

---

## Idempotent Persistence Provisioning

If an app declares a database and that database already exists, no action is taken. Morsel tracks provisioned resources in its own database and skips provisioning for resources that are already in the desired state.

This means:
- Re-deploying an app with a database does not drop and recreate the database
- Re-deploying after a failed provisioning step retries only the failed step
- App data is never affected by a re-deploy

---

## Idempotent DNS and Certificate Operations

DNS records and TLS certificates are provisioned once and retained. Re-deploying an app does not trigger a new DNS record creation or certificate request if the existing record and certificate are valid. Morsel checks current state before acting.

Certificate renewal is handled by the control plane's background renewal process, not by deploys.

---

## Non-Idempotent Operations

A small number of operations are explicitly not idempotent and require deliberate action:

| Operation | Why not idempotent |
|---|---|
| `DELETE /api/repos/:slug/apps/:name` | Starts the grace period on persistence. Calling twice does not double the grace period, but the intent is destructive. |
| `POST /api/repos/:slug/apps/:name/hibernate` | Forces hibernation regardless of current state. |
| `POST /api/repos/:slug/apps/:name/wake` | Forces wake regardless of current state. |
| `POST /api/operator/approvals/batch` | Decisions are applied once. Re-submitting the same approval ID after it has been actioned returns an error. |

These operations are clearly named as commands (`hibernate`, `wake`, `delete`) rather than state declarations, which signals their non-idempotent nature.

---

Up: [Index](README.md) · Prev: [Identity & Ownership](identity-ownership.md) · Next: [Permanence](permanence.md)
