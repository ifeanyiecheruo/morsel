Up: [Index](../README.md) · [Schemas](README.md)

---

# Schema — morsel.json

> **Status:** Draft · **Date:** May 2026

---

## Summary

Every app deployed through Morsel is declared in a `*.morsel.json` file committed to the repository. Morsel reads these files on deploy to determine what to build, how to route it, what resources to provision, and what quotas apply. A single repository can declare any number of apps by committing multiple files.

The formal JSON Schema is at [`morsel.schema.json`](morsel.schema.json).

---

## File Placement

All `*.morsel.json` files live in the `.morsel/` directory at the repository root. Files outside `.morsel/` are ignored by the CLI lint command and the deploy sync.

```
my-repo/
  .morsel/
    api.morsel.json
    worker.morsel.json
    scheduler.morsel.json
  api/
  worker/
  scheduler/
```

A repository with a single app typically uses `app.morsel.json` or a name matching the repo. A repository with multiple apps uses one file per app. File names are only for human organisation — the `name` field inside the file is the canonical identifier.

---

## Field Reference

| Field | Type | Required | Default | Protected |
|---|---|---|---|---|
| `name` | string | No | (repo slug) | No |
| `type` | string | Yes | — | No |
| `dockerfile` | string | Yes | — | No |
| `private` | boolean | No | `false` | No |
| `tier` | string | No | platform default | **Yes** |
| `idle_after` | duration | No | platform default | No |
| `health_check.path` | string | No | `"/healthz"` | No |
| `health_check.timeout` | duration | No | `"5m"` | No |
| `schedule` | cron string | Cronjob only | — | No |
| `timeout` | duration | Cronjob only | — | No |
| `retries` | integer | No | `0` | No |
| `persistence.database` | object | No | (absent) | Addition beyond quota |
| `persistence.storage` | object | No | (absent) | Addition beyond quota |
| `persistence.queues` | object | No | (absent) | Addition beyond quota |
| `persistence.*.permanent` | boolean | No | `false` | No |

**Protected fields** require operator approval before taking effect. The app deploys at its currently approved config until the operator acts. See [platform-features/approvals.md](../platform-features/approvals.md).

### `name`

Unique identifier for this app within the repository. Lowercase alphanumeric with hyphens. Must start and end with a letter or digit. Maximum 30 characters.

If omitted, the repository itself is treated as a single unnamed app. Named and unnamed apps cannot coexist in the same repository.

```json
{ "name": "api" }
```

### `type`

Execution model for the app. Required.

| Value | Description |
|---|---|
| `"http"` | Long-running server process. Gets a URL, a readiness probe, and hibernation on inactivity. |
| `"worker"` | Long-running background process. No URL. Hibernates when all declared queues are idle. |
| `"cronjob"` | Scheduled job. No URL. Runs on the schedule defined by `schedule`. |

### `dockerfile`

Path to the Dockerfile relative to the repository root. Required. The build context is always the repository root regardless of where the Dockerfile lives.

```json
{ "dockerfile": "services/api/Dockerfile" }
```

### `private`

Controls inbound routing. If `false` (default), the app is reachable from the public internet via the external platform gateway. If `true`, the app is reachable only from within the cluster VPC — visible to other Morsel apps but not external traffic. Applies to `http` type only; ignored for `worker` and `cronjob`.

### `tier`

Quota tier name. Determines the app's `ResourceQuota` and `LimitRange`. If unspecified, the platform default tier applies.

Changing `tier` to a value higher than the currently approved value creates a pending approval. The app runs at the current approved tier until the operator approves. See [platform-features/approvals.md](../platform-features/approvals.md).

### `idle_after`

Duration after which an idle HTTP app hibernates. Uses Go duration syntax: `"1h"`, `"30m"`, `"24h"`. Applies to `http` type only — workers hibernate based on queue idle state, not a timer. If omitted, the platform default applies (configurable by the operator).

### `health_check`

Deployment rollout validation settings.

**`health_check.path`** — HTTP path polled for the readiness probe. Applies to `http` type only. Must start with `/`. Default: `"/healthz"`.

**`health_check.timeout`** — Maximum time to wait for a rollout to reach the ready state before triggering an automatic rollback to the previous healthy image. Applies to all types. Default: `"5m"`.

### `schedule`

Cron expression defining when the job runs. Required when `type` is `"cronjob"`. Uses standard five-field cron format: `minute hour day-of-month month day-of-week`.

```json
{ "schedule": "0 9 * * 1-5" }
```

### `timeout`

Maximum wall-clock time for a single job run. Required when `type` is `"cronjob"`. Uses Go duration syntax. If the job exceeds this duration, Kubernetes terminates it.

### `retries`

Number of times to retry a failed job run before marking the run as failed. Applies to `cronjob` type only; ignored for `http` and `worker`. Integer from 0 to 10, default `0`.

Non-production jobs often have external side effects — retry only if your job is idempotent.

### `persistence`

Declares platform-managed resources for this app. Each resource is optional and independent. Resources are provisioned on the first deploy that declares them and persist according to the `permanent` flag.

See [conventions/resource-model.md](resource-model.md) and [platform-features/persistence.md](../platform-features/persistence.md).

**`persistence.database`** — Provisions a dedicated Postgres database accessible at `database.morsel.internal` via a PGBouncer sidecar. Connection string never changes: `postgres://morsel:morsel@database.morsel.internal:5432/morsel`.

**`persistence.storage`** — Grants blob storage access via `blob.morsel.internal`. Objects are namespaced automatically by the platform — the app uses plain keys.

**`persistence.queues`** — Grants queue access via `queue.morsel.internal`. Queue names are scoped to the calling app automatically.

**`persistence.*.permanent`** — If `true`, the resource is retained when the app is deleted (with a 30-day grace period before actual deletion). If `false` (default), the resource is deleted with the app. See [conventions/permanence.md](permanence.md).

Adding any persistence resource beyond the repo's current approved quota creates a pending approval. The resource is not provisioned until approved.

---

## Type-Specific Requirements

| Type | Required extra fields | Forbidden fields |
|---|---|---|
| `http` | (none beyond `type`, `dockerfile`) | (none) |
| `worker` | (none beyond `type`, `dockerfile`) | `private`, `health_check.path` |
| `cronjob` | `schedule`, `timeout` | `private`, `health_check.path`, `idle_after` |

Fields listed as "forbidden" are silently ignored if present, but `morsel lint` emits a warning.

---

## Examples

### Single HTTP app

```json
{
  "type": "http",
  "dockerfile": "Dockerfile"
}
```

### Named HTTP app with all options

```json
{
  "name": "api",
  "type": "http",
  "dockerfile": "api/Dockerfile",
  "private": false,
  "tier": "small",
  "idle_after": "24h",
  "health_check": {
    "path": "/healthz",
    "timeout": "5m"
  },
  "persistence": {
    "database": { "permanent": true },
    "storage": { "permanent": false }
  }
}
```

### Background worker

```json
{
  "name": "worker",
  "type": "worker",
  "dockerfile": "worker/Dockerfile",
  "persistence": {
    "queues": { "permanent": false }
  }
}
```

### Scheduled job

```json
{
  "name": "nightly-report",
  "type": "cronjob",
  "dockerfile": "jobs/report/Dockerfile",
  "schedule": "0 2 * * *",
  "timeout": "30m",
  "retries": 1
}
```

### Private internal API

```json
{
  "name": "internal-api",
  "type": "http",
  "dockerfile": "internal/Dockerfile",
  "private": true,
  "persistence": {
    "database": { "permanent": true }
  }
}
```

### Multi-app repository layout

```
.morsel/
  api.morsel.json      { "name": "api",       "type": "http",    "dockerfile": "api/Dockerfile" }
  worker.morsel.json   { "name": "worker",    "type": "worker",  "dockerfile": "worker/Dockerfile" }
  cron.morsel.json     { "name": "cron",      "type": "cronjob", "dockerfile": "cron/Dockerfile", "schedule": "0 9 * * 1-5", "timeout": "15m" }
```

---

Up: [Index](../README.md) · [Schemas](README.md)
