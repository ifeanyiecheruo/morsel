Up: [Index](README.md) · Prev: [Hibernation](hibernation.md) · Next: [Approvals](approvals.md)

---

# Platform Feature — Cost Controls

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel bounds cost through four complementary mechanisms: quota tiers that cap resource consumption per repo, hibernation that eliminates compute cost for idle apps, budget enforcement that actively blocks and reverses compute spend when the monthly ceiling is approached, and a cost dashboard that gives the operator visibility without requiring them to read cloud billing directly. Cost enforcement is automatic — no manual intervention is required for routine use.

---

## Quota Tiers

Quota is tracked and enforced per repo. All apps belonging to a repo share the repo's quota allocation.

Tiers are fully operator-configurable. Morsel ships with three built-in tiers as a starting point:

| Attribute | small | medium | large |
|---|---|---|---|
| Max apps | 2 | 10 | 25 |
| CPU per app | 0.5 cores | 1 core | 4 cores |
| Memory per app | 512 MB | 1 GB | 4 GB |
| Blob storage per app | 1 GB | 10 GB | 50 GB |
| Database storage per app | 5 GB | 20 GB | 100 GB |
| Queue storage per app | 1 GB | 10 GB | 50 GB |
| Hibernate after (default) | 24 hrs | 48 hrs | 72 hrs |

The built-in `small` tier is the platform default — new repos are assigned to it automatically on first deploy. The operator can edit tier limits, create new tiers, set a different platform default, or delete tiers. The built-in tiers have no special status once created; they can be renamed, edited, or deleted like any operator-created tier.

Tier promotion is an operator action via the admin UI — under 30 seconds once the operator decides to act. Tier demotion is possible but rejected if the repo's current usage exceeds the lower tier's limits.

---

## Tier Management

### Creating and Editing Tiers

**Admin UI:** Platform settings → Tiers → New tier / edit existing.

**CLI:**

```text
morsel operator tier list

morsel operator tier create --name enterprise \
  --max-apps 50 --cpu 8 --memory 16GB \
  --blob 100GB --database 200GB --queues 100GB \
  --hibernate-after 168h

morsel operator tier edit --name medium --max-apps 15 --cpu 2

morsel operator tier set-default --name medium

morsel operator tier delete --name large
```

All tier fields are optional on `edit` — only specified fields are changed. Edits take effect immediately: the control plane updates `ResourceQuota` and `LimitRange` in all Kubernetes namespaces currently on that tier.

### Platform Default Tier

The platform default tier is the tier assigned to repos on their first deploy. It is set at bootstrap to `small` and is operator-configurable via `morsel tier set-default` or the admin UI.

### Deletion Constraints

A tier cannot be deleted if:

- Any repo is currently assigned to it. Repos must be reassigned to another tier first.
- It is currently set as the platform default. A different tier must be set as default first.

### Fallback Behaviour

**Invalid tier name in `morsel.json`:** If a developer declares `"tier": "enterprise"` but no tier named `enterprise` exists, the deploy proceeds using the platform default tier. A warning annotation is emitted in GitHub Actions:

```text
::warning title=Unknown tier: my-app::tier "enterprise" does not exist. Deploying at platform default tier "medium".
```

**Missing platform default tier:** If the platform default tier has been deleted without replacing it, the control plane falls back to its built-in hardcoded baseline (equivalent to the `small` built-in defaults) and emits a warning in the deploy output and the admin UI:

```text
::warning title=Platform default tier missing::configured default tier "medium" does not exist. Using built-in baseline tier. Set a valid default with: morsel tier set-default --name <tier>
```

The baseline fallback is non-configurable and exists solely to prevent the platform from becoming inoperable due to a misconfiguration.

---

## Quota Enforcement Mechanisms

Quota is enforced at multiple layers depending on the resource type:

### Compute (CPU and Memory)
Enforced by Kubernetes `ResourceQuota` and `LimitRange` at the namespace level. The control plane sets these when a namespace is created or when a tier changes. Kubernetes rejects any pod that would exceed the limits — no Morsel-level check required at deploy time.

### App Count
Enforced by the control plane at deploy time. When a repo attempts to deploy beyond its app limit, the API returns `quota_exceeded`. The developer is directed to contact the operator.

### Blob Storage
Enforced by the blob service at write time. `PUT` requests that would exceed the app's blob quota return `429 Too Many Requests` with a `blob_quota_exceeded` error. The blob service tracks per-app byte usage in SQLite. The control plane pushes updated quota limits to the blob service when an app's tier changes.

### Queue Storage
Enforced by the queue service at enqueue time. Enqueue requests that would exceed total queue storage across all of the app's queues return `429 Too Many Requests`. The queue service tracks total storage per app.

### Database Storage
Advisory only. The control plane tracks declared storage allocations and blocks tier-exceeding requests at deploy time. Per-database enforcement at the Postgres level is not possible on a shared instance. App owners who exceed their database quota will not be automatically throttled — the operator is alerted and can act.

---

## Budget Ceiling

The operator sets a monthly budget ceiling at bootstrap (default $500). The billing period is the calendar month. Morsel tracks estimated spend against the ceiling in real time and enforces two thresholds:

| Threshold | Default | Behaviour |
|---|---|---|
| Soft limit | 90% of ceiling | Wake blocked for all non-exempt hibernated apps |
| Hard limit | 100% of ceiling | All running non-exempt apps force-hibernated; wake blocked |

Both thresholds are operator-configurable via `PATCH /api/operator/config`. The budget ceiling itself is also operator-configurable after bootstrap.

The budget ceiling is separate from cloud provider billing alerts. Operators who want hard infrastructure-level alerts should configure them independently in the cloud console — Morsel's enforcement acts on estimated spend and cannot prevent the cloud provider from charging for resources already consumed.

---

## Budget Enforcement

### Soft Limit — Wake Blocked

When estimated spend reaches the soft limit (default 90%):

- **Wake-on-request** — the wake-on-request proxy returns a `503 Service Unavailable` response with a user-facing message ("Platform is over budget for this period") and a `Retry-After` header set to the start of the next billing period. The hibernated app is not woken.
- **Explicit wake** — `POST /api/repos/:slug/apps/:name/wake` returns `budget_soft_limit`. The app remains hibernated.
- **Running apps** — unaffected. Apps that are already running continue to run.
- **Deploys** — unaffected. New images can be pushed and manifests updated; the app runs through its health check, then follows normal hibernation rules.

### Hard Limit — Force Hibernate

When estimated spend reaches the hard limit (100%):

- All running non-exempt apps are force-hibernated immediately. The control plane issues `scale to 0` for each.
- Wake is blocked under the same rules as the soft limit.
- Deploys are allowed so developers can push fixes. The app pod runs long enough to pass health checks, then is immediately hibernated again.
- At the start of the next billing period the hard limit enforcement lifts automatically. Apps remain hibernated but can wake normally once budget resets.

### Operator Wake Override

When an operator explicitly wakes an app via `POST /api/repos/:slug/apps/:name/wake` or the admin UI during a period where the soft or hard limit is active:

- The wake proceeds regardless of the current budget threshold.
- The app is granted a **period exemption** — it is excluded from budget enforcement for the remainder of the current billing period.
- The period exemption means: the app can receive wake-on-request calls normally, and will not be force-hibernated if the hard limit is hit or remains active.
- Period exemptions expire automatically at the end of the billing period and are not renewed unless the operator wakes the app again in the new period.
- The admin UI shows all apps with active period exemptions and their expiry time.

### Explicit Exemptions

The operator can permanently exempt specific apps or entire repos from all budget enforcement. Explicit exemptions persist across billing periods and are not affected by threshold changes.

**From the admin UI:** App management → select app or repo → toggle "Exempt from cost controls".

**From the CLI:**

```text
morsel operator app exempt add    --repo org/my-repo --app api
morsel operator app exempt remove --repo org/my-repo --app api

morsel operator repo exempt add    org/my-repo
morsel operator repo exempt remove org/my-repo
```

Explicit exemptions are intended for apps that must remain available regardless of budget state — for example, a shared internal tool that a team depends on. They should be used sparingly; every exempted app is excluded from the cost controls that protect the platform's overall budget health.

The operator can list all exemptions via `GET /api/operator/exemptions` or in the admin UI.

---

## Cost Estimation

Morsel estimates monthly cost by combining resource requests, actual running time, and current platform list prices fetched from the platform pricing API. Prices are fetched at bootstrap and refreshed daily by the control plane in the background. The stored prices are used for all cost calculations — no live API call is made per request. See [platform/gcp.md](../platform/gcp.md) for the GCP-specific pricing API details.

### Scale Event Tracking

Every scale-to-1 and scale-to-0 event is recorded in the `scale_events` SQLite table with an app identifier and a UTC timestamp. The control plane derives each app's running intervals from this log. No running intervals means zero compute cost.

### Formula

```text
compute_cost  = running_hours_this_period
                × (cpu_cores × compute_cpu_per_core_hour
                   + mem_gb  × compute_mem_per_gb_hour)

storage_cost  = blob_gb       × storage_per_gb_month
              + database_gb   × storage_per_gb_month
              + registry_gb   × registry_per_gb_month

estimated_cost_month = compute_cost + storage_cost
```

`running_hours_this_period` is the sum of all running intervals for the app since the start of the current billing period, expressed in hours.

Estimates reflect resource *requests*, not actual CPU or memory consumption — an app using 10% of its requested CPU is still estimated using 100% of its declared request. This is intentional: requests are what the cloud provider charges for (reserved capacity), not utilisation.

Prices are platform list prices, not negotiated or committed-use prices. Operators with platform discounts will see actual bills lower than Morsel's estimates.

Cost estimates are available at:

- Per-app: `GET /api/repos/:slug/apps/:name/utilisation` → `estimated_cost_month`
- Per-repo: `GET /api/repos/:slug` → `estimated_cost_month`
- Platform: `GET /api/operator/cost` → `estimated_total_month`, `prices_fetched_at`

`prices_fetched_at` in the platform response indicates when prices were last refreshed from the Catalog API. The control plane emits an admin UI warning if prices are more than 48 hours stale (e.g., Catalog API unreachable).

### Price History

Each daily price fetch is stored as an immutable timestamped row in the control plane's SQLite database — snapshots are never overwritten. This builds a history of platform list price changes over time, which makes it possible to audit why a cost estimate changed on a specific date.

The full snapshot history is available at:

```text
GET /api/operator/prices/history
```

Response:

```json
{
  "snapshots": [
    {
      "fetched_at": "2026-06-07T04:00:00Z",
      "compute_cpu_per_core_hour": 0.0535,
      "compute_memory_per_gb_hour": 0.0072,
      "storage_per_gb_month": 0.023,
      "registry_per_gb_month": 0.10
    }
  ]
}
```

Price history is intended for debugging only — for example, to explain why an app's estimated monthly cost increased between two billing periods.

---

## Hibernation as Cost Control

Hibernation is covered in detail in [platform-features/hibernation.md](hibernation.md). From a cost perspective:

- A hibernated app consumes zero compute (CPU and memory)
- Storage costs continue regardless of hibernation state
- At scale, hibernation is the dominant cost control — a cluster of 50 apps where 40 are typically idle costs roughly the same as 10 always-on apps

The default tier's 24-hour idle threshold is deliberately aggressive to maximise cost efficiency for the typical demo/experiment use case.

---

## Operator Cost Dashboard

The admin UI cost dashboard shows:

- Total estimated monthly spend vs. budget ceiling
- Per-repo breakdown sorted by spend
- Apps approaching quota ceiling (flagged for operator awareness)
- Hibernate candidates — running apps with no recent activity that have not yet hit their idle threshold

The operator does not need to open the cloud billing console for routine cost oversight. All actionable cost signals are surfaced in the Morsel dashboard.

---

## Tier Promotion Flow

When a developer hits a quota limit:

1. Developer sees: `✗ Quota exceeded — contact your platform operator to request a tier upgrade`
2. Developer contacts operator (out of band — email, Slack, etc.)
3. Operator opens admin UI → Repo management → finds repo → clicks "Promote to Standard"
4. Operator confirms
5. Kubernetes ResourceQuota updated immediately
6. Developer re-deploys

The operator is the deliberate gatekeeper for tier promotions. This is intentional — unconstrained self-service tier upgrades would undermine the cost model.

---

## Component Contributions

### Control Plane
Enforces app count limits, manages Kubernetes ResourceQuota and LimitRange, provides cost estimation endpoints, and handles tier promotion. See [components/control-plane.md — Cost Controls](../components/control-plane.md).

### Blob Service
Enforces per-app blob storage quota at write time. Tracks byte usage per app. Receives updated quota limits from control plane on tier changes. See [components/blob-service.md — Cost Controls](../components/blob-service.md).

### Queue Service
Enforces per-app total queue storage quota at enqueue time. See [components/queue-service.md — Cost Controls](../components/queue-service.md).

### Database Service
Advisory database storage limits only — no hard enforcement at the Postgres layer. See [components/database-service.md — Cost Controls](../components/database-service.md).

### Admin UI
Surfaces cost dashboard, tier promotion controls, quota ceiling alerts, and hibernate candidates. See [components/admin-ui.md — Cost Controls](../components/admin-ui.md).

---

Up: [Index](README.md) · Prev: [Hibernation](hibernation.md) · Next: [Approvals](approvals.md)
