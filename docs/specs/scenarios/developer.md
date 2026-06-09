Up: [Index](../README.md) · Prev: [Operator Scenarios](operator.md) · Next: [Resource Model](../conventions/resource-model.md)

---

# Developer Scenarios

> **Status:** Draft · **Date:** May 2026

---

## Persona

A member of the GitHub organisation. Deploys and operates apps on behalf of their team. No cloud or Kubernetes knowledge required. Their entire interface is a git repository and `*.morsel.json` files in `.morsel/`.

Developers care about: fast deploys, predictable URLs, working persistence, and clear error messages.

---

## Scenario 1 — First Deploy

**Context:** A developer has a containerised web application and wants to get it running with a public URL.

**Steps:**

1. Get the Morsel instance URL from the operator (e.g. `https://admin.apps.example.com`).

2. Create `.morsel/app.morsel.json` at the repo root:
   ```json
   {
     "type": "http",
     "dockerfile": "Dockerfile"
   }
   ```

3. Copy the morsel workflow file to `.github/workflows/morsel-deploy.yml` and set `MORSEL_API_URL` to the instance URL.

4. Push to `main`.

**What happens:**
- GitHub Actions triggers and runs `morsel app deploy`
- The CLI exchanges the GitHub OIDC token for a Morsel token
- The image is built and pushed to the staging registry
- Morsel validates the token, copies the image to the canonical registry, and applies the Kubernetes manifest
- The app passes health checks and is marked running
- Morsel auto-registers the repo with the default quota tier
- The operator receives a new repo digest notification (not in the critical path)

**Result:** The app is running at `https://my-repo.apps.example.com`. The developer did not interact with cloud infrastructure, Kubernetes, DNS, or certificates.

**Edge cases:**
- If health checks fail, the deploy is rolled back automatically and GitHub Actions marks the job failed with an annotation pointing to the logs endpoint
- If the repo has never deployed before, registration is automatic — no operator approval needed

---

## Scenario 2 — Iterative Development

**Context:** The developer wants to deploy a code change.

**Steps:**
1. Make code changes
2. Push to `main`

**What happens:** The same workflow runs. The image is rebuilt with the new code and Morsel performs a rolling update. The old image is retained as `last-healthy` for rollback.

**Result:** New version live, typically within 2–3 minutes of the push.

**Edge cases:**
- If the new image fails health checks within the configured timeout, Morsel automatically redeploys `last-healthy` and emits a failure annotation in GitHub Actions
- The developer sees: `✗ worker — deploy failed — rollout timed out. Rolling back to sha256:prev456`

---

## Scenario 3 — Multi-App Repo

**Context:** A developer has an HTTP API, a background worker, and a scheduler in the same repo.

**Configuration:**
```
.morsel/
  api.morsel.json
  worker.morsel.json
  scheduler.morsel.json
```

```json
// api.morsel.json
{ "name": "api", "type": "http", "dockerfile": "api/Dockerfile" }

// worker.morsel.json
{ "name": "worker", "type": "worker", "dockerfile": "worker/Dockerfile" }

// scheduler.morsel.json
{ "name": "scheduler", "type": "cronjob", "schedule": "0 9 * * 1-5", "timeout": "30m", "dockerfile": "scheduler/Dockerfile" }
```

**What happens on push:** The workflow discovers all three files and deploys them in parallel. Each app is independent — one failing deploy does not block the others.

**Result:** Three running apps:
- `https://api.my-repo.apps.example.com`
- Worker running continuously (no public URL)
- Scheduler running on weekday mornings

---

## Scenario 4 — Adding Persistence

**Context:** A developer wants to add a database to their API app.

**Steps:**
1. Update `api.morsel.json`:
   ```json
   {
     "name": "api",
     "type": "http",
     "persistence": {
       "database": { "permanent": true }
     }
   }
   ```
2. Push to `main`

**What happens:**
- On deploy, Morsel detects a new database declaration
- Morsel creates a Postgres database and user scoped to this app
- A PGBouncer sidecar is added to the pod
- The app can now connect using fixed constants: `host=database.morsel.internal port=5432 dbname=morsel user=morsel password=morsel`
- No credentials to store, no SDK to configure

**Result:** App has a working database. The developer never sees the real credentials — PGBouncer manages them transparently.

---

## Scenario 5 — Private App

**Context:** A developer wants an internal API accessible only to other Morsel apps, not the public internet.

**Configuration:**
```json
{
  "name": "internal-api",
  "type": "http",
  "private": true
}
```

**Result:** The app gets a URL (`https://internal-api.my-repo.apps.example.com`) that resolves only from within the VPC. Other Morsel apps can call it. External traffic is rejected.

---

## Scenario 6 — Hitting a Quota Limit

**Context:** A developer tries to deploy a third app but their repo is on the default tier (2 app limit).

**What happens:**
- The sync call succeeds (it just records the declared list)
- The deploy call for the third app returns an error:
  ```
  ✗ Quota exceeded: new-app
    repo org/my-repo has reached its app limit (2/2) on the default tier
    Contact your platform operator to request a tier upgrade
  ```
- GitHub Actions marks the job failed and adds an annotation

**Developer action:** Contact the operator and request a tier upgrade. The operator promotes the repo to standard tier in the admin UI in under 30 seconds. The developer re-pushes and the deploy succeeds.

---

## Scenario 7 — Requesting a Tier Change

**Context:** A developer wants their app to use more CPU and memory (move from `small` to `medium` tier).

**Steps:**
1. Update `morsel.json`:
   ```json
   { "tier": "medium" }
   ```
2. Push to `main`

**What happens:**
- The deploy succeeds at the current approved tier (`small`)
- A pending approval is created for the tier change
- GitHub Actions emits a warning annotation:
  ```
  ⚠ my-app — tier change to 'medium' pending operator approval — running at 'small'
  ```

**Developer action:** Notify the operator. When the operator approves, the tier change is applied immediately.

**Result:** App continues running without interruption while the approval is pending.

---

## Scenario 8 — Removing an App

**Context:** A developer has finished with an experiment and wants to remove it.

**Steps:**
1. Delete `experiment.morsel.json` from `.morsel/`
2. Push to `main`

**What happens:**
- The sync call compares the declared app list against Morsel's records
- Morsel detects `experiment` is no longer declared and deletes it
- If persistence was declared with `permanent: false`, it enters the grace period (default 30 days) before purge
- If persistence was declared with `permanent: true`, the API returns an error and the developer must follow the two-step removal process

**Result:** App stops immediately. Data retained for the grace period.

---

## Scenario 9 — Local Development

**Context:** A developer wants to test their app locally with the full Morsel platform including hibernation and quotas.

**Prerequisites:** Docker Desktop (or equivalent) with Kubernetes enabled.

**Steps:**
1. Bootstrap a local Morsel instance:
   ```
   morsel --profile local service bootstrap --platform local
   ```
2. Deploy from the working directory (including uncommitted changes):
   ```
   morsel --profile local app deploy
   ```

**Result:** App running at `https://my-repo.morsel.localhost`. Full platform behaviour including hibernation, quotas, and approvals enforced locally. The developer approves their own approval requests since they are also the local operator.

**No staging handshake:** Locally, images push directly to the in-cluster registry — no staging copy step.

---

## Scenario 10 — Wake from Hibernation

**Context:** A developer visits their app's URL and the app is hibernated (scaled to zero due to inactivity).

**What happens:**
1. Browser sends request to `https://my-app.my-repo.apps.example.com`
2. Platform gateway routes to the wake-on-request proxy
3. Proxy holds the request and triggers scale-to-1 via the Morsel API
4. Pod starts (typically 5–15 seconds)
5. Proxy forwards the held request to the now-running pod
6. Browser receives the response — no special handling required

**Result:** App responds normally. The developer may notice a cold-start delay on the first request after a period of inactivity.

---

Up: [Index](../README.md) · Prev: [Operator Scenarios](operator.md) · Next: [Resource Model](../conventions/resource-model.md)
