Up: [Index](../README.md) · Prev: [GCP Platform](gcp.md)

---

# Platform — LocalPlatform

> **Status:** Draft · **Date:** May 2026

---

## Overview

`LocalPlatform` is a complete implementation of the `Platform` interface with no cloud dependencies. It runs a full Morsel replica on a developer's machine for local development and testing. All platform behaviour — hibernation, quotas, approvals, permanent resources — is enforced identically to production.

The goal is that any behaviour observable locally will behave the same way in production. There are no local-only shortcuts in business logic.

---

## Prerequisites

Local mode requires a local Kubernetes cluster. Morsel does not install or manage one.

Supported local Kubernetes distributions:
- Docker Desktop (Kubernetes enabled)
- Rancher Desktop
- k3s
- minikube

### Cluster Selection at Bootstrap

Bootstrap requires specifying the kubeconfig to use via `--kubeconfig`:

```
morsel --profile local service bootstrap --platform local --kubeconfig ~/.kube/config
```

If `--kubeconfig` is omitted, bootstrap falls back to `$KUBECONFIG` then `~/.kube/config`, displays the detected cluster server URL, and requires explicit confirmation before proceeding:

```
Detected cluster: https://127.0.0.1:6443 (context: docker-desktop)
Proceed with this cluster? [y/N]:
```

The kubeconfig path and cluster server URL are written to the profile at bootstrap time. All subsequent CLI commands for this profile verify the current cluster server URL against the saved value and exit with an error if they do not match:

```
✗ Cluster mismatch.
  Profile expects: https://127.0.0.1:6443
  Found:           https://192.168.1.5:6443

  Your kubeconfig may have changed since bootstrap. To re-bootstrap
  against the new cluster, re-run: morsel service bootstrap --platform local
```

This prevents accidental operations against a different cluster after a kubeconfig rotation or context switch.

If no cluster is found at all, bootstrap prints instructions and exits:

```
✗ No local Kubernetes found.

  Install one of the following and re-run:
    • Docker Desktop   https://docs.docker.com/desktop/
    • Rancher Desktop  https://rancherdesktop.io
    • k3s              https://k3s.io
    • minikube         https://minikube.sigs.k8s.io
```

---

## Bootstrap Prompts

`LocalPlatform.BootstrapPrompts()` returns a minimal set — no cloud credentials, no region, no budget:

| Prompt | Default | Notes |
|---|---|---|
| GitHub org slug | None — required | Scopes OIDC token validation |
| Local domain | `morsel.localhost` | Base domain for app URLs |

---

## Resources Provisioned at Bootstrap

Local bootstrap provisions only what is needed within the local Kubernetes cluster:

```
Local container registry (Deployment in morsel namespace)
  └── control plane Deployment + PersistentVolumeClaim (SQLite)
        ├── Blob service Deployment + PersistentVolumeClaim (quota tracking)
        ├── Queue service Deployment
        ├── Shared Postgres Deployment + PersistentVolumeClaim
        ├── Admin UI (static files embedded in control plane binary via Go embed)
        └── Envoy Gateway (GatewayClass: morsel-external / morsel-internal)
              └── Self-signed wildcard certificate for *.morsel.localhost
```

No GCP resources are created. No cloud account is required.

---

## Sub-Interface Implementations

### BlobStore — Filesystem

Objects stored in `~/.morsel/local/blobs/{namespace}/{key}` on the local filesystem. No GCS, no credentials.

Namespace isolation and quota enforcement are identical to production — the `LocalPlatform` `BlobStore` calls the same blob service HTTP API. The blob service itself is unaware it is running locally; only the backing store changes.

### SecretStore — Local JSON File

Secrets stored in `~/.morsel/local/secrets.json` on the local filesystem with `0600` permissions.

```json
{
  "morsel-signing-key": "base64-encoded-key",
  "morsel-bootstrap-config": "base64-encoded-config"
}
```

No encryption at rest — local development only.

### CredentialProvider — Local deploy token

`CredentialProvider.DeployToken()` generates a minimal signed JWT:

```json
{
  "repository": "localhost/{git-root-dirname}",
  "ref":        "refs/heads/{current-branch}",
  "sha":        "{current-git-sha}"
}
```

The JWT is signed with `local-deploy-signing-key`, generated at bootstrap and stored in the platform SecretStore.

`CredentialProvider.ValidateDeployToken(token)` validates the incoming JWT signature against `local-deploy-signing-key` and returns `localhost/{dirname}` as the repo slug. The control plane's `POST /api/token/deploy` handler calls this method — it contains no GitHub-specific logic. See [platform-features/authentication.md — Deploy Auth Flow](../platform-features/authentication.md).

`CredentialProvider.ValidateOperatorToken(ctx, r)` reads the operator's email address from the JSON request body and checks it against the `operator-principals` list in the platform SecretStore. Returns the email as the operator subject on success.

`CredentialProvider.AmbientToken()` (ambient service identity, used by control plane itself) returns an empty string — no cloud identity is required locally.

### DNSProvider — No-op (`*.morsel.localhost`)

`*.morsel.localhost` resolves to `127.0.0.1` natively in all modern browsers via the `.localhost` special-use domain (RFC 6761). No DNS configuration, no hosts file edits, no DNS server required.

`DNSProvider.CreateRecord()` and `DeleteRecord()` are no-ops — records do not need to be created because wildcard resolution handles all subdomains.

### CertProvider — Self-Signed

A wildcard self-signed certificate for `*.morsel.localhost` is generated at bootstrap time using the Go `crypto/tls` package. `CertProvider.Provision()` returns the `*tls.Certificate`; the control plane writes it to a Kubernetes Secret (type `kubernetes.io/tls`) in the `morsel` namespace via its existing `client-go` connection. Envoy Gateway references that Secret in its listener TLS configuration for termination.

Browsers will show a certificate warning on first visit. Developers can add the certificate to their local trust store to suppress the warning:
```
morsel --profile local service bootstrap --trust-cert
```
(This flag adds the self-signed CA to the OS trust store.)

Renewal is not required — the certificate is regenerated on each bootstrap run.

---

## App URLs

```
Named app:    https://{app-name}.{repo-slug}.morsel.localhost
Unnamed app:  https://{repo-slug}.morsel.localhost
```

No DNS configuration required. `*.morsel.localhost` resolves natively.

---

## Authentication in Local Mode

Local mode has no IAP. Operator identity is managed as a list of email addresses stored in the platform SecretStore under the key `operator-principals`. Any email in the list is treated as an authorised operator.

### Managing Principals

```
morsel --profile local operator principal add    --principal alice@example.com
morsel --profile local operator principal remove --principal alice@example.com
morsel --profile local operator principal list
```

The first principal is added automatically at bootstrap time using the email address provided in the bootstrap wizard.

### Operator Login

`morsel operator login` prompts for an email address and posts it to `POST /api/token/oidc`. The control plane handler calls `LocalPlatform.ValidateOperatorToken(ctx, r)`, which reads the email from the request body and checks it against the principals list. On success, it returns the email as the operator subject; the handler issues a 15-minute access token plus a 90-day refresh token.

No password — authentication relies on local network trust (only someone who can reach the control plane endpoint can obtain a token).

```
morsel --profile local operator login
Email: alice@example.com
✓ Logged in as alice@example.com. Token written to ~/.config/morsel/local.profile.json.
```

Login is rejected with `401 Unauthorized` if the email is not in the principals list.

---

## Deploying Apps Locally

```
morsel --profile local app deploy
```

No GitHub remote is required. The command derives the repo slug from the git root directory name, prefixed with `localhost`:

```
/home/alice/projects/my-app  →  slug: localhost/my-app
```

The deploy flow is identical to GitHub Actions — only the identity source differs:

1. `LocalPlatform.DeployToken()` generates a signed JWT with `{ "repository": "localhost/my-app" }` and exchanges it at `POST /api/token/deploy` for a short-lived developer token.
2. Discovers all `*.morsel.json` files in `.morsel/`
3. Calls `POST /api/repos/localhost/my-app/sync` with the full app list and current git SHA
4. Builds each app's container image in parallel using the local Docker or Podman daemon
5. Pushes each image directly to the in-cluster registry (no staging handshake)
6. Calls `POST /api/repos/localhost/my-app/apps` for each app with the image digest and developer token

**Requirements:**
- Docker or Podman running locally
- Dockerfiles at the paths declared in each app's `dockerfile` field
- A bootstrapped local Morsel instance (`morsel --profile local service bootstrap --platform local`)

---

## Full Replica Behaviour

Local mode enforces all production platform behaviour:

| Feature | Local behaviour |
|---|---|
| Hibernation | Fully enforced — apps scale to zero after idle threshold |
| Quotas | Fully enforced — deploy API rejects apps over tier limits |
| Approvals | Fully enforced — developer is also operator, so they approve their own requests |
| Permanent resources | Fully enforced — lint and API both enforce two-step removal |
| Lint | Identical to production |
| Sync | Identical to production |
| Error model | Identical to production |

The developer approving their own approval requests is intentional — it exercises the full approval workflow locally without requiring a separate operator account.

---

## Dollar Cost

Zero. `LocalPlatform` runs entirely on the developer's machine with no cloud services.

---

## Limitations vs. GCPPlatform

| Concern | LocalPlatform | GCPPlatform |
|---|---|---|
| Object durability | Local filesystem — not durable across machine loss | GCS — 99.999999999% durability |
| Multi-developer access | Single developer only | All org members |
| Public URLs | `*.morsel.localhost` — local only | `*.apps.example.com` — publicly accessible |
| Certificate trust | Self-signed — browser warning | Let's Encrypt — trusted by default |
| Persistent data across reinstall | Lost on cluster reset | Survives platform reinstall (GCS + Secret Manager) |
| Admin UI auth | No IAP — local operator token | GCP IAP + Google account |

These limitations are acceptable for local development and testing. Production deployment uses `GCPPlatform`.

---

Up: [Index](../README.md) · Prev: [GCP Platform](gcp.md)
