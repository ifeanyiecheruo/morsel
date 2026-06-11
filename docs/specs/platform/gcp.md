Up: [Index](../README.md) · Prev: [Platform Interface](interface.md) · Next: [Local Platform](local.md)

---

# Platform — GCPPlatform

> **Status:** Draft · **Date:** May 2026

---

## Overview

`GCPPlatform` is the production platform implementation. It uses GCS for blob storage, Secret Manager for secrets, Workload Identity for service authentication, Cloud DNS or Cloudflare for DNS, and ACME/Let's Encrypt for TLS certificates. All GCP authentication uses Workload Identity — no service account key files are created.

---

## Resources Provisioned at Bootstrap

`Bootstrap()` provisions all GCP and Kubernetes resources in dependency order. Every step is idempotent — re-running bootstrap skips steps already in the desired state.

```
GCS state bucket
  └── VPC with Private Google Access subnet
        └── GKE Autopilot cluster (VPC-native)
              ├── Artifact Registry (staging + canonical repos)
              ├── Workload Identity Federation provider
              │     └── IAM binding: GitHub Actions SA → staging registry writer
              ├── Service accounts
              │     ├── morsel-api-sa  (artifact registry writer, GCS admin, Secret Manager accessor)
              │     └── gke-nodes-sa   (artifact registry reader)
              ├── Secret Manager secrets
              │     ├── morsel-signing-key
              │     ├── morsel-bootstrap-config
              │     ├── morsel-notification-config
              │     └── morsel-cloudflare-token  (if Cloudflare DNS selected)
              ├── Morsel API Deployment + PersistentVolumeClaim
              ├── Blob service Deployment + PersistentVolumeClaim
              ├── Queue service Deployment
              ├── Shared Postgres Deployment + PersistentVolumeClaim
              ├── Admin UI bundle → GCS bucket
              ├── GKE Gateway (external + internal classes)
              └── Identity-Aware Proxy
                    └── IAP OAuth client
                          └── Operator principal IAM binding
```

---

## Bootstrap Prompts

`BootstrapPrompts()` returns:

| Prompt | Default | Notes |
|---|---|---|
| GCP project ID | Detected from OAuth token | Operator confirms or overrides |
| GCP region | `us-central1` | |
| Base domain | None — required | Validated against DNS provider |
| GitHub org slug | None — required | Scopes OIDC token validation |
| Notification email | None — required | Stored in Secret Manager |
| Monthly budget ceiling | `$500` | Informational — cost dashboard only |
| DNS provider | `Cloud DNS` | `Cloud DNS` or `Cloudflare` |
| Cloudflare API token | None — required if Cloudflare | Validated for zone edit scope |
| Operator access | None — required | Google account or Group email |

---

## Sub-Interface Implementations

### BlobStore — GCS

Objects stored in a GCS bucket (`morsel-blobs-{project-id}`). The blob service authenticates to GCS via its ambient Workload Identity. The `BlobStore` interface methods map directly to GCS object operations.

Key namespacing (`{repo-slug}/{app-name}/{key}`) is applied by the blob service before calling `BlobStore.Put/Get/List`. The `BlobStore` implementation itself does not apply namespacing.

All GCS traffic uses Private Google Access — no public internet egress from the cluster.

### SecretStore — Secret Manager

Secrets are stored in GCP Secret Manager in the Morsel project. The Morsel API reads secrets at startup via its ambient Workload Identity.

Platform secrets:
| Secret name | Contents |
|---|---|
| `morsel-signing-key` | JWT signing key (HS256 symmetric key) |
| `morsel-bootstrap-config` | Wizard configuration (project, region, domain, org, etc.) |
| `morsel-notification-config` | Notification email address |
| `morsel-cloudflare-token` | Cloudflare API token (if Cloudflare DNS selected) |

No secret versions are managed by Morsel — the current version is always used. Rotation of the signing key requires a brief Morsel API restart.

### CredentialProvider — Workload Identity

The `CredentialProvider.AmbientToken()` method returns a short-lived GCP access token from the ambient GKE Workload Identity metadata server. No external calls or credential files required — the token is available at `http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token`.

### DNSProvider — Cloud DNS

DNS records managed via the GCP Cloud DNS API. The Morsel API authenticates via its ambient Workload Identity service account, which holds `dns.resourceRecordSets.*` roles on the Morsel project's DNS zone.

`DNSProvider.CreateRecord()` creates an A record pointing the app's subdomain to the GKE load balancer IP. `DeleteRecord()` removes it on app deletion.

### DNSProvider — Cloudflare

DNS records managed via the Cloudflare API using a token stored in Secret Manager. The token is scoped to edit DNS records for a single zone — no other Cloudflare permissions.

The bootstrap wizard generates the exact token scope instructions and validates the token's zone permissions before proceeding. Cloudflare DNS is selected when the operator's domain is managed in Cloudflare rather than Cloud DNS.

### CertProvider — ACME/Let's Encrypt

TLS certificates provisioned via the ACME protocol using the Go ACME library against Let's Encrypt. Uses DNS-01 challenges via the configured `DNSProvider`.

On certificate provisioning:
1. Request certificate from Let's Encrypt ACME endpoint
2. Receive DNS-01 challenge: create `_acme-challenge.{domain}` TXT record via `DNSProvider`
3. Signal challenge completion to Let's Encrypt
4. Let's Encrypt validates and issues certificate
5. Store certificate and private key in Kubernetes Secret in app namespace
6. Remove `_acme-challenge` TXT record

Renewal runs 30 days before expiry via a background goroutine in the Morsel API.

---

## Workload Identity Federation

WIF allows the Morsel API pod to access GCP services without storing any long-lived secret. Developer CI runners do not use WIF directly.

**Configuration provisioned at bootstrap:**
- WIF Identity Pool: `morsel-github-pool`
- WIF Provider: `morsel-github-provider` (bound to the GKE cluster's Kubernetes service account)
- Service account: `morsel-api-sa` — granted `artifactregistry.writer` on staging and canonical repos

**Flow (Morsel API → GCP):**
```
Morsel API pod
  → presents Kubernetes service account token (ambient, no config required)
  → GKE Workload Identity exchanges it for a short-lived GCP access token for morsel-api-sa
  → uses token to access Artifact Registry, GCS, Secret Manager, Cloud DNS
```

**Developer CI registry access** is brokered by the Morsel API. When a CI runner exchanges its GitHub OIDC token at `POST /api/token/deploy`, the Morsel API uses its own `morsel-api-sa` credentials to generate short-lived staging registry push credentials scoped to that caller's path, and returns them alongside the Morsel token. The CI runner never holds GCP credentials.

`GCPPlatform.DeployToken()` reads the GitHub OIDC token from the GitHub Actions environment. `GCPPlatform.ValidateDeployToken(token)` fetches GitHub's JWKS (public endpoint, cached), validates the JWT signature, and extracts the `repository` claim. See [platform-features/authentication.md — Deploy Auth Flow](../platform-features/authentication.md).

---

## IAM Least-Privilege Policy

| Service account | Role | Scope |
|---|---|---|
| `morsel-api-sa` | `artifactregistry.writer` | Morsel project (staging + canonical repos) |
| `morsel-api-sa` | `storage.admin` | `morsel-blobs-*` bucket only |
| `morsel-api-sa` | `secretmanager.secretAccessor` | Morsel project secrets only |
| `morsel-api-sa` | `dns.resourceRecordSets.*` | Morsel project DNS zone only |
| `gke-nodes-sa` | `artifactregistry.reader` | Canonical repo only |

No service account holds `Editor`, `Owner`, or any project-level primitive role. No cross-project IAM bindings exist.

---

## Private Google Access

The GKE cluster subnet is configured with Private Google Access enabled. All traffic from GKE to GCP APIs (Artifact Registry, GCS, Cloud DNS, Secret Manager) routes through Google's internal network. GCP API endpoints are not reachable from the public internet.

GitHub Actions workflows run on GitHub-hosted runners and make outbound HTTPS connections to the Morsel API only. The Morsel API makes all GCP API calls on their behalf from within the cluster — no GCP traffic originates from CI runners.

---

## Admin UI Authentication

The admin UI is protected by GCP IAP. IAP is provisioned at bootstrap with an OAuth client and the operator's principal list. Operators authenticate with their Google account — no separate password.

When an authenticated request reaches the Morsel API, IAP injects a signed `X-Goog-IAP-JWT-Assertion` header. The `POST /api/token/oidc` handler calls `GCPPlatform.ValidateOperatorToken(ctx, r)`, which reads and verifies that header using Google's public JWKS, then returns the operator's email as the subject. The handler issues a Morsel access token and refresh token from there — no GCP-specific logic in the handler itself.

IAP is the most GCP-specific concern in the platform that is not covered by the `Platform` interface. If portability to another cloud is needed, Cloudflare Access is the recommended replacement — it is cloud-agnostic and supports the same Google identity provider.

---

## GCP Project Isolation

Morsel runs in a dedicated GCP project. The one exception to strict GCP-project-boundary isolation is the Cloudflare API token — if Cloudflare DNS is selected, this token stored in Secret Manager can modify DNS records in a Cloudflare zone, which is outside the GCP project. The token is scoped as tightly as Cloudflare allows (single zone, edit only) to minimise blast radius.

No VPC peering, no Shared VPC, no org-level service account bindings connect the Morsel project to any other GCP project.

---

Up: [Index](../README.md) · Prev: [Platform Interface](interface.md) · Next: [Local Platform](local.md)
