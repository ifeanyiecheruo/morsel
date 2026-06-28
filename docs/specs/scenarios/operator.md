Up: [Index](../README.md) · Prev: [Architecture](../architecture.md) · Next: [Developer Scenarios](developer.md)

---

# Operator Scenarios

> **Status:** Draft · **Date:** May 2026

---

## Persona

Manages the running platform on a part-time basis. The role may rotate. No Kubernetes or cloud infrastructure knowledge required. Their entire workflow is the `morsel` CLI (for installation, upgrades, and access management) and the admin UI (for day-to-day management). Expected time investment: a few hours per month.

---

## Scenario 1 — Initial Platform Setup

**Context:** Setting up Morsel for the first time in a GCP project.

**Prerequisites:**
- A GCP project with billing enabled
- Owner or equivalent IAM role on the project
- A domain with an existing Cloud DNS zone (or Cloudflare account)
- A GitHub organisation slug

**Steps:**

1. Download the `morsel` binary:
   ```
   curl -L https://github.com/org/morsel/releases/latest/download/morsel-Linux-x86_64 \
     -o morsel && chmod +x morsel
   ```

2. Run bootstrap:
   ```
   ./morsel service bootstrap --platform gcp
   ```

3. A browser opens for GCP OAuth login. Sign in with the Google account that has Owner on the project.

4. Preflight checks run automatically. If any fail, the binary prints a specific actionable error and exits — no partial state is created.

5. The wizard prompts for configuration (first run only):
   - GCP project ID (detected from OAuth, confirm or override)
   - GCP region (default: `us-central1`)
   - Base domain (e.g. `apps.example.com`)
   - GitHub org slug
   - Notification email
   - Monthly budget ceiling (default: $500)
   - DNS provider (Cloud DNS or Cloudflare)
   - Operator access (Google account or Group email)

6. The binary prints a full summary of resources to be created and estimated monthly cost, then asks for confirmation.

7. Provisioning runs with friendly progress output (4–8 minutes total):
   ```
   ✓ State bucket created
   ✓ VPC and Private Google Access configured
     Creating GKE cluster… (this takes 4–6 minutes)
   ✓ GKE cluster ready
   ✓ Artifact Registry configured
   ✓ Workload Identity Federation configured
   ✓ IAM bindings applied
   ✓ Secret Manager provisioned
   ✓ control plane installed
   ✓ Admin UI installed
   ✓ Identity-Aware Proxy configured
   ✓ Operator access granted
     Waiting for TLS certificate…
   ✓ TLS certificate issued
   ✓ Smoke test passed

   Morsel is ready.
   Admin UI: https://admin.apps.example.com
   ```

8. Share the Morsel instance URL with developers so they can set it in their deploy workflow.

**Result:** Platform running. Developers can deploy immediately. Operator accesses the admin UI at `https://admin.apps.example.com` using their Google account — no separate password.

---

## Scenario 2 — Weekly Health Check

**Context:** Routine weekly review. Expected time: 5–10 minutes.

**Steps:**

1. Open the admin UI.

2. Check the cost dashboard:
   - Is total estimated spend within budget?
   - Are any repos consuming significantly more than expected?
   - Are there repos near their quota ceiling that may need a tier upgrade?

3. Glance at new repos digest:
   - Any new repos auto-registered this week?
   - Do they look legitimate (recognisable team names, reasonable app counts)?

4. Check platform status:
   - Any failed deploys in the last 24 hours?
   - Any certificate alerts?
   - Any pending approvals requiring attention?

**Result:** 5-minute visibility into platform health. No action required if everything is green.

---

## Scenario 3 — Promoting a Repo Tier

**Context:** A developer has hit the default tier app limit and requests an upgrade to standard tier.

**Steps:**

1. In the admin UI → Repo management, find the repo by name.

2. Review: current app count, current spend, last deploy date. Confirm the request is reasonable.

3. Click "Promote to Standard" and confirm.

**What happens:** The control plane updates the repo's quota tier. The `ResourceQuota` in the repo's Kubernetes namespaces is updated immediately. The developer can deploy additional apps on their next push.

**Time:** Under 2 minutes including review.

---

## Scenario 4 — Batch Approving Pending Changes

**Context:** Several developers have requested tier changes for their apps. Weekly batch review.

**Steps:**

1. Admin UI → Approvals. View all pending approvals with repo, app, field, current value, and requested value.

2. For each approval:
   - **Approve** — change applied immediately
   - **Reject** — with an optional reason (visible to the developer in their next deploy output)
   - **Ignore** — defer without rejecting; approval stays pending

3. Submit the batch in one action.

**What happens:** Approved changes are applied in a single reconciliation pass per app. Rejected approvals are cleared. Ignored approvals remain pending until they expire (default 30 days) or are actioned.

**Time:** 10–15 minutes for a moderate backlog.

---

## Scenario 5 — Responding to a Failed Deploy Alert

**Context:** The operator receives an alert that a deploy has failed repeatedly.

**Steps:**

1. Admin UI → Platform status → Failed deploys. Find the affected app.

2. Review the deploy history for the app — status, timestamps, git SHAs.

3. Options:
   - If the issue is platform-side (e.g., image pull failure due to registry issue): investigate via CLI — `morsel service status`
   - If the issue is app-side: notify the developer with context from the deploy history

4. If the app is stuck in a bad state, the operator can force-hibernate it from the admin UI to stop resource consumption while the developer investigates.

**Time:** 10–20 minutes depending on root cause.

---

## Scenario 6 — Cleaning Up Stale Apps

**Context:** Periodic cleanup of apps from repos that may have been deleted or archived. Quarterly or as needed.

**Steps:**

1. Admin UI → Stale apps. List is sorted by last deploy date, oldest first.

2. For each app with no recent deploy activity:
   - Check whether the source repo still exists in GitHub (manual verification — Morsel cannot do this automatically)
   - If repo is deleted or archived: delete the app and its non-permanent resources from the admin UI
   - If repo still exists but is dormant: leave it (the developer may return) or notify the team

3. For apps with permanent resources from deleted repos, use the operator override (`?force=true` via admin UI) to purge retained data.

**Time:** 30–60 minutes quarterly.

---

## Scenario 7 — Transferring App Ownership

**Context:** A repo has been reorganised and an app needs to move to a different repo.

**Steps:**

1. Admin UI → App management. Find the app by repo and name.

2. Select "Transfer ownership". Enter the new repo slug (e.g., `org/new-repo`).

3. Confirm.

**What happens:** Morsel updates the app's registered repo. The app continues running without interruption. On the next deploy from the new repo, ownership is confirmed via the OIDC token.

**Note:** The new repo must exist and must have at least one successful deploy to Morsel (or the operator must manually register it). The old repo can no longer deploy this app after transfer.

---

## Scenario 8 — Platform Upgrade

**Context:** A new `morsel` binary version is available with platform improvements.

**Steps:**

1. Download the new binary.

2. Run:
   ```
   morsel service bootstrap --platform gcp
   ```

3. The binary reads existing configuration from Secret Manager, prints a summary of what will change, and asks for confirmation.

4. Rolling upgrade proceeds:
   ```
   ✓ control plane upgraded
   ✓ Blob service upgraded
   ✓ Queue service upgraded
   ✓ Database service upgraded
     Redeploying apps…
   ✓ 47 apps redeployed
   ✓ 12 hibernated apps updated in place
   ✗ 3 apps failed — retry with: morsel service upgrade retry
   ```

5. For any failed app redeployments:
   ```
   morsel service upgrade retry
   ```

**What happens during upgrade:** The platform remains operational throughout. Apps keep running. Hibernated apps are updated in place without waking. GKE node upgrades are handled independently by GKE Autopilot.

**Time:** 15–30 minutes including app redeployments.

---

## Scenario 9 — Adding a New Operator

**Context:** The operator role is rotating to a new team member.

**Steps:**

```
morsel operator principal add --principal new-operator@example.com
```

Or for a team:
```
morsel operator principal add --principal morsel-operators@example.com
```

**What happens:** The new principal is granted access to the IAP-protected admin UI. On their first visit they authenticate with their Google account — no password, no separate account.

To remove a departing operator:
```
morsel operator principal remove --principal old-operator@example.com
```

Their Morsel refresh token remains valid for up to one TTL period (15 minutes) after removal, then expires naturally.

---

## Scenario 10 — Checking Platform Status

**Context:** An operator wants a quick health snapshot without opening a browser.

**Steps:**
```
morsel service status
```

**Output:**
```
Cluster:        healthy
control plane:     healthy
Failed deploys: 2 in last 24h
Certificates:   all valid
Pending approvals: 14
Stale apps:     3 (last deploy > 90 days ago)
Est. cost/month: $287.40 / $500 budget
```

**Time:** Seconds.

---

Up: [Index](../README.md) · Prev: [Architecture](../architecture.md) · Next: [Developer Scenarios](developer.md)
