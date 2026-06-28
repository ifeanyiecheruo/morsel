Up: [Index](README.md) · Prev: [REST API](../conventions/rest.md) · Next: [Authentication](authentication.md)

---

# Platform Feature — Deployment

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel deploys containerised apps from any system that can build a container image, push it to the staging container registry, and call the Morsel deploy API with a valid OIDC token. The supported and documented flow is GitHub Actions using `morsel app deploy`. Other deployment models are possible but unsupported.

---

## App Declarations

Apps are declared in `*.morsel.json` files inside the `.morsel/` directory at the repository root. Each file declares one app:

```
.morsel/
  api.morsel.json
  worker.morsel.json
  scheduler.morsel.json
```

The filename is cosmetic — the app's identity within Morsel comes from the `name` field inside the file, not the filename. The filename is conventionally the same as the app name. For a single-app repo with an unnamed app the conventional name is `.morsel/app.morsel.json`.

App names must be lowercase alphanumeric with hyphens, no leading or trailing hyphens, and unique within the repo. An app with no `name` field is the unnamed app for that repo; the repo itself is treated as the app name. A repo can have at most one unnamed app.

---

## Deployment Model

A deployment integration is responsible for three things:

1. **Identity** — prove to Morsel which repo is deploying, using a mechanism the platform trusts
2. **Image** — build a container image and place it in the staging registry
3. **Dispatch** — call the Morsel sync and deploy APIs with the image digest and a valid token

Everything else — image copy to canonical registry, Kubernetes manifest apply, health check monitoring, rollback on failure, DNS, TLS — is handled by the control plane.

---

## The Staging Handshake

Morsel enforces an image staging handshake on all cloud platform deploys. The deployer never writes directly to the canonical image store.

```
Deployer
  → builds image
  → pushes to staging container registry  (deployer has write access to staging only)
  → calls POST /api/repos/:slug/apps with image digest + Morsel token

control plane
  → validates token
  → validates image exists in staging repo at claimed digest
  → copies image: staging → canonical (registry-side metadata operation, no network transfer)
  → deletes staging image
  → applies Kubernetes manifest
  → watches rollout
```

Registry path structure:

```
Staging:   {region}-docker.pkg.dev/{project}/morsel-staging/{repo-slug}/{app-name}:{build-id}
Canonical: {region}-docker.pkg.dev/{project}/morsel-canonical/{repo-slug}/{app-name}
```

Morsel retains two digests per app in the canonical repo: `current` (the image currently running) and `last-healthy` (the image before the most recent successful deploy). Rollback redeploys `last-healthy`.

This ensures:
- No deployer ever touches the canonical image store directly
- Cross-repo image overwrites are impossible regardless of registry ACLs
- The control plane is the sole writer to the canonical registry
- Abandoned staging images are cleaned up by a 1-hour TTL policy on the staging repo

On `LocalPlatform`, the staging handshake is skipped — the deployer pushes directly to the in-cluster registry. Developer and platform share the same trust boundary locally.

---

## Supported Flow: GitHub Actions with `morsel app deploy`

This is the reference implementation. Developers copy a single workflow file and set the Morsel instance URL directly in it — no secrets or repository variables required.

### Workflow File

Save as `.github/workflows/morsel-deploy.yml`:

```yaml
name: Deploy to Morsel

on:
  push:
    branches: [main]

permissions:
  id-token: write   # required for GitHub OIDC token
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Install morsel CLI
        run: |
          curl -L https://github.com/org/morsel/releases/latest/download/morsel-Linux-x86_64 \
            -o morsel && chmod +x morsel && sudo mv morsel /usr/local/bin/morsel

      - name: Deploy
        env:
          MORSEL_API_URL: https://admin.apps.example.com
        run: morsel app deploy
```

`MORSEL_API_URL` is not a secret — it is the public URL of the Morsel instance and can be committed directly to the repository. The workflow is intentionally thin — authentication and dispatch only. All deploy logic lives in `morsel app deploy`.

### What `morsel app deploy` Does in CI

When `GITHUB_ACTIONS=true`:

1. Calls `Platform.DeployToken()` — posts the GitHub OIDC token directly to `POST /api/token/github-oidc`; control plane validates it against GitHub's public JWKS and returns a Morsel access token plus short-lived staging registry push credentials
2. Discovers all `*.morsel.json` files in `.morsel/`
3. Calls `POST /api/repos/:slug/sync` with the full declared app list and current git SHA
4. For each app in parallel:
   - Builds the container image using Docker (`ubuntu-latest` has Docker pre-installed)
   - Pushes the image to the staging container registry
   - Calls `POST /api/repos/:slug/apps` with the image digest, git ref, git SHA, and Morsel token
   - Polls the operation until complete or failed
5. Emits GitHub Actions annotations for errors and approval warnings
6. Exits non-zero on any deploy failure — GitHub marks the job failed

### Output

Successful deploy:
```
✓ Synced 3 apps (0 deleted)
✓ my-demo-app       deployed sha256:abc123  (38s)
✓ worker            deployed sha256:def456  (41s)
⚠ scheduler         tier change to 'medium' pending operator approval — running at 'small'
```

Failed deploy:
```
✓ Synced 3 apps (0 deleted)
✓ my-demo-app       deployed sha256:abc123  (38s)
✗ worker            deploy failed — rollout timed out. Rolling back to sha256:prev456
✓ scheduler         deployed sha256:ghi789  (44s)
```

---

## Local Deployment with `morsel app deploy`

Without `GITHUB_ACTIONS=true`, `morsel app deploy` deploys against a local Morsel instance. The code path is identical to CI except for how repo identity and registry access are established.

### Repo slug

On LocalPlatform every local repo is identified under the `localhost` org. The slug is derived from the git root directory name:

```
/home/alice/projects/my-app  →  localhost/my-app
/Users/bob/code/payments     →  localhost/payments
```

The slug is stable as long as the directory name does not change. No configuration is required.

### Auth and image push

1. `LocalPlatform.DeployToken()` generates a signed JWT with `{ "repository": "localhost/{dirname}" }` and exchanges it at `POST /api/token/github-oidc` for a short-lived Morsel developer token. The control plane on LocalPlatform skips GitHub JWKS validation and trusts the submitted `repository` claim directly. See [platform-features/authentication.md — Local Deploy Auth](authentication.md).
2. Discovers all `*.morsel.json` files in `.morsel/`
3. Calls `POST /api/repos/localhost/{dirname}/sync` with the full declared app list and current git SHA
4. For each app in parallel:
   - Builds using local Docker or Podman daemon
   - Pushes directly to the in-cluster registry — no staging handshake on LocalPlatform
   - Calls `POST /api/repos/localhost/{dirname}/apps` with the image digest and Morsel developer token
5. Polls and streams progress to terminal

Builds from the working directory including uncommitted changes — intentional for local development. No GitHub remote is required.

---

## Deployment Concerns

### Authentication

The deployer must present a Morsel token issued from a trusted identity source. The deployer never holds a long-lived credential — the OIDC token is short-lived (5 min) and exchange produces a Morsel token (10 min, no refresh for CI). See [platform-features/authentication.md](authentication.md) and [platform/gcp.md](../platform/gcp.md) for platform-specific identity federation details.

### Parallelism

Apps within a repo are deployed in parallel. One failing deploy does not block others (`fail-fast: false` equivalent). The sync call gives Morsel the full declared list before any deploy starts, so deletions are detected even if a deploy fails.

### Health Checks and Rollback

Morsel watches the Kubernetes rollout for each app. A deploy is considered successful when the desired number of pods are ready (readiness probe passing). If the rollout does not complete within the configured `health_check.timeout`, Morsel automatically redeploys the `last-healthy` image. See [conventions/rest.md](../conventions/rest.md) for the rollback error shape.

### Protected Config

Fields that require operator approval (e.g., `tier`) are not blocked at deploy time. The app deploys at its currently approved config. A pending approval is created and the workflow emits a warning annotation. The app is never blocked from deploying because of a pending approval. See [platform-features/approvals.md](approvals.md).

### Dockerfile Placement

Each app declares its Dockerfile via the `dockerfile` field in `morsel.json`. The value is a path relative to the repository root. The build context is always the repository root.

```json
{ "name": "api", "type": "http", "dockerfile": "services/api/Dockerfile" }
```

A repository can contain any number of Dockerfiles in any layout. There is no required location. Each app in `.morsel/` names its own Dockerfile independently.

### Sync and Deletions

`POST /api/repos/:slug/sync` is called once per deploy run with the complete list of declared app names. Apps present in Morsel's records but absent from the declared list are deleted, subject to permanence rules. This means removing a `*.morsel.json` file and pushing is sufficient to delete an app — no explicit delete call is needed. See [conventions/idempotency.md](../conventions/idempotency.md).

---

## Other Deployment Models

Any system that can satisfy the three deployment model requirements (identity, image, dispatch) can deploy to Morsel. The control plane does not care how the deployer built the image or where it runs, as long as the OIDC token is valid and the image digest exists in the staging registry.

Examples of what is possible but unsupported:
- GitLab CI / Jenkins / CircleCI — would need to obtain a GitHub-compatible OIDC token or a different trusted identity mechanism
- Manual deploy script — `morsel app deploy` works outside of GitHub Actions against any profile
- CD system calling the API directly — possible if the system can produce a valid Morsel token

Morsel does not document or test these flows. The GitHub Actions path is the only supported integration.

---

Up: [Index](README.md) · Prev: [REST API](../conventions/rest.md) · Next: [Authentication](authentication.md)
