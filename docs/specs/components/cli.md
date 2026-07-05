Up: [Index](../README.md) · Prev: [Control Plane](control-plane.md) · Next: [Blob Service](blob-service.md)

---

# Component — CLI (`morsel`)

> **Status:** Draft · **Date:** May 2026

---

## Overview

The `morsel` binary is the operator's sole interface for installing, configuring, and upgrading the platform. It is also the developer's tool for local deployment and `morsel.json` validation. It is a static Go binary with no external dependencies — no Terraform, no gcloud, no kubectl required on the operator's machine.

Commands are grouped by audience. `morsel service` covers platform infrastructure lifecycle. `morsel operator` covers all management tasks performed exclusively by operators. `morsel app` and `morsel lint` are the developer surface.

All provisioning is implemented directly in Go using embedded platform and Kubernetes SDK libraries. The binary's behaviour is fully deterministic and debuggable.

---

## Component Diagram

```
Operator machine
┌──────────────────────────────────────────────────────────────────┐
│ morsel binary                                                    │
│                                                                  │
│  ┌──────────────┐  ┌─────────────────────┐  ┌────────────────┐  │
│  │ service      │  │ operator            │  │ app  │  lint   │  │
│  │ bootstrap    │  │ login / logout      │  │ deploy         │  │
│  │ status       │  │ principal add/rm/ls │  │      │ --staged│  │
│  │ delete       │  │ tier create/edit/.. │  │      │ --fix   │  │
│  │ upgrade retry│  │ app exempt add/rm   │  │                │  │
│  │              │  │ repo exempt add/rm  │  │                │  │
│  └──────┬───────┘  └──────────┬──────────┘  └───────┬────────┘  │
│         │                     │                      │           │
│  ┌──────▼─────────────────────▼──────────────────────▼──────┐   │
│  │ Platform interface  (GCPPlatform / LocalPlatform)        │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                  │
│  Profile: ~/.config/morsel/<name>.profile.json                  │
└──────────────────────────────────────────────────────────────────┘
         │                         │
         ▼                         ▼
   Cloud APIs                Control Plane
   (bootstrap only)          (operator, deploy)
```

---

## Personas

**Operators** use `morsel service *` for platform lifecycle (install, upgrade, health) and `morsel operator *` for management tasks (authentication, principals, tiers, exemptions).

**Developers** use `morsel app deploy` for local development deploys and `morsel lint` for validating `morsel.json` files before committing.

---

## CLI Structure

```
# Platform lifecycle (operator, run on operator machine)
morsel [--profile <name>] service deploy --platform <gcp|local> [--kubeconfig <path>]
                                          [--initial-username <name>] [--out-initial-passwd <file>]
                                          [--no-login] [--force] [-y]
morsel [--profile <name>] service status
morsel [--profile <name>] service delete
morsel [--profile <name>] service upgrade retry

# Operator management (operator, communicates with control plane)
morsel [--profile <name>] operator login [--api-url <url>] [--username <name>] [--password <pw>]
morsel [--profile <name>] operator logout

morsel [--profile <name>] operator principal add --principal <username>
morsel [--profile <name>] operator principal remove --principal <username>
morsel [--profile <name>] operator principal list
morsel [--profile <name>] operator principal password-reset --principal <username>

morsel [--profile <name>] operator tier list
morsel [--profile <name>] operator tier create --name <name> [--max-apps <n>] [--cpu <cores>] [--memory <MB>] [--blob <GB>] [--database <GB>] [--queues <GB>] [--hibernate-after <duration>]
morsel [--profile <name>] operator tier edit --name <name> [<same flags>]
morsel [--profile <name>] operator tier set-default --name <name>
morsel [--profile <name>] operator tier delete --name <name>

morsel [--profile <name>] operator app exempt add --repo <org/repo> --app <name>
morsel [--profile <name>] operator app exempt remove --repo <org/repo> --app <name>
morsel [--profile <name>] operator repo exempt add <org/repo>
morsel [--profile <name>] operator repo exempt remove <org/repo>

# Developer (runs in repo, locally or in CI)
morsel [--profile <name>] lint
morsel [--profile <name>] lint --staged
morsel [--profile <name>] lint --fix

morsel [--profile <name>] app deploy
```

`--profile` defaults to `default`. Each profile maps to `~/.config/morsel/<profile>.profile.json`.

---

## Commands

| Command | Audience | Purpose |
|---|---|---|
| `service deploy --platform gcp\|local` | Operator | Install or upgrade the platform. On first run, creates the initial admin principal and prints the generated password. |
| `service status` | Operator | Report the health of all platform components without making changes. |
| `service delete` | Operator | Tear down all platform resources. Requires explicit confirmation. Designer use only. |
| `service upgrade retry` | Operator | Retry app redeployments that failed during the most recent platform upgrade. |
| `operator login` | Operator | Authenticate to the Morsel instance. Prompts for username and password. Writes profile on success. If `password_reset_required` is set, prompts for a new password inline. |
| `operator logout` | Operator | Revoke refresh token server-side. Delete profile file. |
| `operator principal add` | Operator | Add a new principal to the operator principals list (no password set initially). |
| `operator principal remove` | Operator | Remove a principal and revoke all their refresh tokens. |
| `operator principal list` | Operator | List all principals with their username and password-reset-required flag. |
| `operator principal password-reset` | Operator | Mark a principal as requiring a password reset on next login. |
| `operator tier list` | Operator | List all configured quota tiers. |
| `operator tier create` | Operator | Create a new quota tier. |
| `operator tier edit` | Operator | Edit limits on an existing tier. Changes apply immediately to all repos on that tier. |
| `operator tier set-default` | Operator | Set the platform default tier for new repos. |
| `operator tier delete` | Operator | Delete a tier. Fails if any repos are assigned to it or it is the current default. |
| `operator app exempt add/remove` | Operator | Add or remove a budget-control exemption for a specific app. |
| `operator repo exempt add/remove` | Operator | Add or remove a budget-control exemption for all apps in a repo. |
| `lint` | Developer | Validate all `*.morsel.json` files in `.morsel/`. |
| `lint --staged` | Developer | Validate only git-staged `*.morsel.json` files. Pre-commit hook use. |
| `lint --fix` | Developer | Auto-remediate safe issues (schema errors, formatting). |
| `app deploy` | Developer | Deploy all apps in `.morsel/`. Works locally and in GitHub Actions CI. |

---

## Deploy Phases

`morsel service deploy` runs four phases in sequence. Each phase is fully idempotent.

### Phase 1 — Authentication

Platform-specific.

**GCPPlatform:** Browser opens to the GCP OAuth consent screen. Operator authenticates. OAuth token is returned to a localhost callback listener in the binary. Token is held in memory only — never persisted.

**LocalPlatform:** No OAuth flow. Deploy verifies cluster access using the kubeconfig provided via `--kubeconfig` (or detected from `$KUBECONFIG` / `~/.kube/config` with confirmation). The cluster server URL is written to the profile and all future commands for this profile verify it has not changed.

### Phase 2 — Preflight Checks

Before provisioning, the binary validates prerequisites. All checks must pass before proceeding.

| Check | Failure message |
|---|---|
| Billing active | "Billing is not enabled on project {id}. Enable it in the cloud console." |
| Required APIs enabled | "The following APIs must be enabled: {list}." |
| Operator IAM permissions | "Your account is missing the following roles: {list}." |
| Compute quota sufficient | "Insufficient CPU/node quota in region {region}." |
| DNS zone exists | "No DNS zone found for {domain}." |

### Phase 3 — Wizard (first run only)

Presents prompts returned by `Platform.BootstrapPrompts()`. Configuration is collected, summarised, and confirmed before any resources are created.

Wizard configuration is stored in the platform secret store (`morsel-bootstrap-config`) after provisioning. On subsequent runs, the wizard is skipped — config is read from the platform secret store.

### Phase 4 — Provisioning

Resources are created in dependency order with friendly progress output. Raw API responses are written to a log file for debugging.

```
✓ Object storage bucket created
✓ VPC and internal networking configured
  Creating Kubernetes cluster… (this takes 4–6 minutes)
✓ Kubernetes cluster ready
✓ Container registry configured
✓ Platform identity federation configured
✓ IAM bindings applied
✓ Platform secret store provisioned
✓ Control plane installed
✓ Admin UI installed
  Waiting for morsel-api to become healthy…
✓ morsel-api ready
Initial operator "admin" password: <generated-password>
Logged in as "admin".
```

After provisioning, `morsel service deploy` calls `POST /bootstrap` with a randomly generated password and the one-time bootstrap token from the cluster. On success (201) the password is printed once and the CLI auto-logs in (unless `--no-login` is given). On subsequent runs the 409 Conflict response is silently ignored — no new principal is created.

Flags:

- `--initial-username` (default: `admin`) — username for the first principal
- `--out-initial-passwd <file>` — write the password to a file instead of printing
- `--no-login` — skip automatic login after first-time bootstrap

Note: the exact step names are platform-specific. See [platform/gcp.md](../platform/gcp.md) for the GCP output.

---

## Profile Management

Each profile is stored at `~/.config/morsel/<profile>.profile.json` with `0600` permissions. The file is only written on successful authentication — never created for failed or incomplete flows.

GCPPlatform profile:

```json
{
  "platform": "gcp",
  "project": "morsel-prod",
  "region": "us-central1",
  "api_url": "https://api.example.com",
  "access_token": "eyJ...",
  "access_token_expires_at": "2026-05-26T10:15:00Z",
  "refresh_token": "mrl_...",
  "refresh_token_expires_at": "2026-08-26T10:00:00Z"
}
```

LocalPlatform profile:

```json
{
  "platform": "local",
  "kubeconfig": "/Users/alice/.kube/config",
  "kubecontext": "k3d-morsel-local",
  "cluster_server": "https://127.0.0.1:6443",
  "api_url": "http://localhost:18080",
  "access_token": "eyJ...",
  "access_token_expires_at": "2026-06-08T10:15:00Z",
  "refresh_token": "mrl_...",
  "refresh_token_expires_at": "2026-09-08T10:00:00Z"
}
```

On LocalPlatform the API is also reachable through the Gateway at `https://api.morsel.localhost` after bootstrap. The profile stores `http://localhost:18080` (the NodePort) for direct CLI access without requiring DNS resolution.

`kubeconfig`, `kubecontext`, and `cluster_server` are locked at bootstrap. On every subsequent command, the CLI resolves `kubecontext` from the saved `kubeconfig` path, reads the current server URL, and exits with an error if it does not match `cluster_server`.

On every command invocation:
1. Load profile file
2. If access token valid, proceed
3. If expired, attempt silent refresh via `POST /api/token/refresh`
4. If refresh token expired or absent, trigger interactive platform OAuth flow
5. Execute command

---

## `morsel lint`

Validates `*.morsel.json` files in `.morsel/`. Designed to run as a pre-commit hook.

| Check | Severity |
|---|---|
| Schema validity — all required fields present, correct types | Error |
| Valid `type` value (`http`, `worker`, `cronjob`) | Error |
| `schedule` present when `type` is `cronjob` | Error |
| `timeout` present when `type` is `cronjob` | Error |
| `name` unique within `.morsel/` folder | Error |
| Removing a persistence resource marked `permanent: true` | Error |
| Removing a persistence resource marked `permanent: false` | Warning |
| `tier` value within repo's current quota tier | Warning |
| `idle_after` value is a valid duration string | Error |

Pre-commit hook setup:
```bash
# .git/hooks/pre-commit
morsel lint --staged
```

---

## `morsel app deploy`

Deploys all apps declared in `.morsel/`. Works identically locally and in CI — the `Platform` interface abstracts all credential and registry differences.

In CI (`GITHUB_ACTIONS=true`):
- Calls `Platform.DeployToken()` to exchange GitHub OIDC token for Morsel token and registry credentials
- Pushes images to staging container registry (staging handshake)
- Emits GitHub Actions annotations for errors and approval warnings

Locally:
- Uses stored profile token
- Pushes images directly to in-cluster registry (no staging handshake on `LocalPlatform`)
- Streams progress to terminal

See [platform-features/deployment.md](../platform-features/deployment.md) for the full deploy flow.

---

## Distribution

Released as a static Go binary via GitHub Releases for Linux, macOS, and Windows:

```
curl -L https://github.com/org/morsel/releases/latest/download/morsel-Linux-x86_64 \
  -o morsel && chmod +x morsel
./morsel service bootstrap --platform gcp
```

No package manager, no installer, no dependencies beyond the binary.

---

## Dollar Cost

The `morsel` binary runs on the operator's machine — no hosted infrastructure cost. Bootstrap-time cloud API calls are negligible.

---

## Operational Cost

- **Distribution** — binary published to GitHub Releases on each platform version. No infrastructure to maintain.
- **Upgrade** — operator downloads new binary, runs `morsel service bootstrap --platform <name>`. Bootstrap reads existing config from the platform secret store.
- **Multi-operator** — each operator downloads the binary and runs `morsel operator login`. Profiles are per-machine, per-operator. Bootstrap config is shared in the platform secret store.

---

## Scalability

The CLI is a local tool. There is no scalability concern — it runs on one machine at a time and makes sequential API calls. Bootstrap is not designed for concurrent execution.

---

## Security

- Platform OAuth token held in memory only during bootstrap — never written to disk
- Profile file written with `0600` permissions — no group or world read
- Refresh token rotated on every use
- `morsel service delete` requires explicit confirmation and is intended for designer use only
- Bootstrap log file (raw API responses) is written to a local temp file — operators should treat it as potentially sensitive

---

## Performance

Bootstrap provisioning takes 8–15 minutes on first run (dominated by Kubernetes cluster creation). Subsequent bootstrap runs for upgrades are faster — most resources are already in the desired state.

`morsel lint` completes in under 1 second for typical repo sizes.

`morsel app deploy` performance is dominated by image build and push time — typically 1–3 minutes per app.

---

## Platform Feature Support

### Authentication
Owns the platform OAuth browser flow, profile file lifecycle, silent token refresh, `operator login`, and `operator logout`. See [platform-features/authentication.md](../platform-features/authentication.md).

### Deployment
Owns `morsel app deploy` — the reference deploy implementation for both local and CI contexts. Calls `Platform.DeployToken()` to abstract credential differences. See [platform-features/deployment.md](../platform-features/deployment.md).

### Networking (bootstrap-time)
During `service bootstrap`, provisions the platform gateway classes and configures the DNS provider connection, then waits for the initial TLS certificate. Post-bootstrap, networking is managed entirely by the control plane.

### Approvals
Surfaces approval warnings in `morsel app deploy` terminal output. Polls reconciliation operations on approval. No approval management — that is the operator's domain via the admin UI.

---

Up: [Index](../README.md) · Prev: [Control Plane](control-plane.md) · Next: [Blob Service](blob-service.md)
