Up: [Index](README.md) · Prev: [Architecture](architecture.md) · Next: [Developer Scenarios](scenarios/developer.md)

---

# Security Model

> **Status:** Draft · **Date:** May 2026

---

## Core Principles

These constraints are non-negotiable. They exist to ensure Morsel cannot be used as a liability for the operator's wider infrastructure. Any proposed change that would expand the blast radius requires explicit designer sign-off and a documented rationale.

---

## Principle 1: GitHub Calls Morsel, Not the Other Way

Morsel never holds a GitHub credential. GitHub Actions calls the control plane and presents a short-lived OIDC token as proof of identity. Morsel validates the token signature against GitHub's public JWKS endpoint — a read-only, unauthenticated operation.

**Why:** Morsel has no ability to enumerate the GitHub organisation, read source code, or impersonate developers. If Morsel is compromised, the attacker has no way to reach beyond the platform into GitHub.

---

## Principle 2: Cloud Project Isolation

Morsel runs in a dedicated cloud project (GCP) or account (AWS, Azure). This is a hard isolation boundary, not a convention.

- All Morsel resources live in this project and nowhere else
- Morsel's service accounts are granted no roles outside this project
- No service account key files are created — all authentication uses ambient platform identity
- No cross-project IAM grants, no Shared VPC peering, no org-level service account bindings

**Why:** Even with full control-plane access to the Morsel project, an attacker cannot affect the operator's other projects or move laterally to other infrastructure.

---

## Principle 3: Blast Radius Boundary

An attacker who fully compromises Morsel can:
- Deploy or delete apps on the platform
- Exhaust quotas and incur cost (bounded by monthly budget ceiling)
- Read all app code and data stored in platform-managed persistence

An attacker cannot:
- Enumerate the GitHub organisation or team membership
- Read source code from GitHub
- Impersonate users outside the platform
- Access any cloud project other than the dedicated Morsel project
- Modify DNS or certificates outside the Morsel domain
- Access app secrets that the developers manage independently (environment variables from external sources)

This boundary is preserved by following principles 1, 2, and 4.

---

## Principle 4: No Long-Lived Credentials

No GitHub PAT, no service account key files, no stored cloud credentials. Every authentication relationship in the system uses one of:

- **Short-lived cryptographic tokens** (GitHub OIDC JWT, platform identity tokens) — valid for minutes, signed by the issuer, verified by signature only
- **Ambient cloud identity** — credentials injected by the cloud platform, not stored on disk
- **Short-lived access tokens** (issued by control plane) — signed JWT, verified by signature, no database lookup required

**What Morsel holds:**
- GitHub's public JWKS URL (read-only, no auth required)
- Platform service credentials for Morsel itself (via ambient cloud identity — not stored on disk)
- Its own JWT signing key (stored in the platform secret store, read at startup)
- Cloudflare API token if Cloudflare DNS is selected (stored in the platform secret store, minimal scope: single zone)

**What Morsel never holds:**
- GitHub PAT or OAuth token
- Service account key files
- Any stored credential that can be stolen and replayed

---

## GitHub OIDC Token Flow

```
GitHub Actions runner (GitHub-hosted)
  → GitHub generates OIDC JWT (5-min TTL)
  → Claims: { repository, ref, sha, workflow, ... }
  → POST /api/token/github-oidc  { token: "jwt" }

control plane
  → Fetch GitHub JWKS (public, cached, no auth)
  → Validate JWT signature
  → Extract repository claim
  → Issue Morsel access token (10 min):
      { sub: "repo:org/my-repo", repo: "org/my-repo", role: "developer", exp: ... }

GitHub Actions
  → Deploy with Morsel access token
  → Token expires after workflow completes
```

Morsel never stores the GitHub OIDC token. The exchange is stateless — no database lookup required.

---

## Cloud Project Isolation Details

### No Cross-Project IAM

Morsel's service identities are granted roles **only** in the Morsel project. No service identity holds any role in any other cloud project. See [platform/gcp.md](platform/gcp.md) for the specific GCP service account names and IAM bindings.

The one exception is the Cloudflare API token when Cloudflare DNS is selected — it is stored in the platform secret store and can modify DNS records in a Cloudflare zone outside the cloud project boundary. The token is scoped to the minimum: edit access to a single zone, no other permissions.

### Platform Internal Networking

All traffic from the Kubernetes cluster to platform APIs (container registry, object storage, DNS, secret store) is routed via the platform's internal network — not the public internet. No platform API endpoint is reachable from the internet. See [platform/gcp.md](platform/gcp.md) for GCP-specific details.

GitHub Actions workflows run on GitHub-hosted runners and make outbound connections to the platform (OIDC exchange and staging registry push). This is accepted: the connections are outbound-only, TLS-secured, and authenticated by short-lived cryptographically signed tokens. No inbound internet access to Morsel is required.

### Self-Hosted Runners Explicitly Out of Scope

Running self-hosted GitHub Actions runners inside the Morsel VPC would eliminate the internet leg of the GitHub Actions → cloud platform connection. However, it introduces persistent VMs requiring patching, scaling, and lifecycle management — operational overhead unjustified for a non-production convenience platform. This decision is recorded here so it is not revisited without deliberate consideration.

---

## Platform Identity Federation

GitHub Actions obtains short-lived platform credentials via identity federation without storing any long-lived secret. The exact mechanism is platform-specific — see [platform/gcp.md](platform/gcp.md) for the GCP implementation (Workload Identity Federation).

**Configuration (generic):**
- Identity federation maps GitHub OIDC → short-lived platform credential
- Service identity bound to the federation provider
- Condition: restricts to the operator's GitHub org
- Permissions: write access to staging container registry **only**

**Flow:**
```
GitHub Actions
  → exchange GitHub OIDC for short-lived platform credential via identity federation
  → credential grants write access to staging container registry only
  → push image to staging repo
  → call control plane with image digest (via GitHub OIDC, not platform credential)
```

GitHub Actions never touches the canonical image repository. The control plane is the sole writer to the canonical registry.

---

## Container Registry: Staging Handshake

The staging handshake prevents deployers from directly overwriting production images.

```
Deployer (GitHub Actions)
  ├─ push image → staging repo (has write access)
  └─ call control plane with image digest + OIDC token

control plane
  ├─ validate token
  ├─ confirm image exists in staging at claimed digest
  ├─ copy image: staging → canonical (metadata operation, no data transfer)
  ├─ delete staging image
  └─ apply Kubernetes manifest
```

**Why:** Cross-repo image overwrites are impossible regardless of registry ACLs. One repo's deployer can never overwrite another repo's production image — the control plane is the gatekeeper.

---

## Container Image Security

Two digests per app are retained in the canonical registry:
- `current` — the image currently running
- `last-healthy` — the image before the most recent successful deploy

On deploy failure, Morsel redeploys `last-healthy` without any registry interaction — no deployer can reverse a rollback by overwriting the image.

Staging images are deleted by Morsel after the canonical copy completes. A 1-hour TTL policy on the staging repository removes any images abandoned by failed workflows as a safety net.

---

## Network Security

### Internal Apps vs. Public Apps

Apps with `private: true` are reachable only from within the cluster VPC. Public apps are reachable from the internet. Both use the same URL convention and platform infrastructure — the difference is which load balancer (internal vs. external) serves the traffic.

### Kubernetes NetworkPolicy

Each app namespace has a `NetworkPolicy` that:
- Allows inbound traffic from the load balancer (public or internal)
- Allows inbound from other app pods (for private inter-app calls)
- Denies pod-to-pod direct sidecar access (prevents database sidecar sniffing)

---

## Database Isolation

Each app gets its own Postgres database and Postgres user. The user is granted `GRANT ALL` scoped to that database only. Cross-app database access is impossible at the Postgres level regardless of what the app attempts.

Additionally, Kubernetes NetworkPolicy prevents direct pod-to-pod access to another app's PGBouncer sidecar. An app that connects to `database.morsel.internal:5432` is routed through its own pod's PGBouncer (via `127.0.0.1` inject or localhost alias), not another app's sidecar.

---

## Blob Storage Isolation

The blob service identifies callers by their Kubernetes service account token (resolved via TokenReview API). Key namespacing (`{repo-slug}/{app-name}/{key}`) is applied server-side before any object storage operation. An app that passes a key starting with another app's namespace is still namespaced under its own prefix — collision is structurally impossible.

---

## Queue Isolation

Queue names are namespaced by the calling pod's service account identity. Two apps can use the same queue name without collision — they hit different Postgres tables namespaced internally as `{repo-slug}__{app-name}__{queue-name}`.

---

## Secret Management

### Morsel's Secrets

Stored in the platform secret store (read by control plane via ambient cloud identity; see [platform/gcp.md](platform/gcp.md)):
- JWT signing key — used to sign all Morsel tokens
- Bootstrap config — platform configuration (immutable after bootstrap)
- Notification config — operator email address
- Cloudflare API token — (if Cloudflare DNS selected; minimal scope: single zone)

No secret is rotated at runtime — the signing key persists for the lifetime of the control plane instance. Rotation requires a control plane pod restart.

### App Secrets

Apps do not receive credentials from Morsel (except the fixed database constants, which are PGBouncer conventions mapped to real per-app credentials held in Kubernetes Secrets). Apps must manage their own secrets via:
- Environment variables injected by the app's CI/CD system
- External secrets manager (HashiCorp Vault, 1Password, etc.)
- Kubernetes Secrets created and managed independently

Morsel has no knowledge of or access to app secrets.

---

## RBAC: Two Roles

| Role | How acquired | Can do |
|---|---|---|
| `developer` | GitHub OIDC exchange | Deploy, manage, delete own repo's apps |
| `operator` | Platform OIDC exchange + principal check | All developer actions + approve tier changes + promote repos + view all apps |

Role is encoded in the Morsel token at exchange time. No per-request directory lookup. Role changes take effect within one access token TTL (default 15 minutes).

---

## Access Token Security

Access tokens are signed JWTs. Verification requires only the public JWT signing key (embedded in the binary at compile time or read from the platform secret store). No database lookup per request.

If an access token is stolen, it can be used until expiry (default 15 minutes). After expiry, it is invalid — no revocation mechanism. Refresh tokens can be revoked immediately via SQLite delete.

---

## Operator Principal List

Operators are authenticated via the platform's operator authentication gateway. The operator's principal identity is checked against the operator principal list stored in the platform secret store. The list is immutable after bootstrap and can only be modified via `morsel operator principal add/remove`. See [platform/gcp.md](platform/gcp.md) for GCP-specific details (IAP, Google account/group format).

---

## What Compromised Morsel Cannot Do

- Read source code from GitHub
- Enumerate GitHub organisation or team membership
- Create GitHub credentials or tokens
- Access any cloud project other than the dedicated Morsel project
- Modify DNS records outside the Morsel domain (unless Cloudflare DNS is selected, in which case only the configured zone is at risk)
- Access cloud resources outside the Morsel project boundary
- Steal long-lived credentials (none exist in the system)
- Steal GitHub tokens (Morsel never holds them)

---

## Threat Model: Common Attacks

### Compromised control plane Pod

An attacker gains shell access to the control plane container.

**Can do:**
- Access all Morsel data in SQLite (repos, apps, tokens, approvals)
- Access the JWT signing key in memory
- Access Morsel's secrets from the platform secret store (via ambient cloud identity)
- Deploy or delete apps
- Access all platform-managed persistence (object storage, Postgres, queues)
- Issue Morsel tokens

**Cannot do:**
- Reach beyond the Morsel cloud project (no cross-project IAM)
- Access GitHub (no stored credentials)
- Compromise the operator's other infrastructure

**Mitigation:**
- Regular platform updates (rolling pod replacement)
- Pod security policies (RunAsNonRoot, no privileged containers)
- RBAC limiting what Morsel service account can do in the cloud project
- Audit logging of API calls

### Stolen GitHub OIDC Token

An attacker intercepts a GitHub OIDC token during the workflow.

**Can do:**
- Call control plane on behalf of the compromised repo
- Deploy, update, or delete apps for that repo
- Exceed quota (bounded by tier limit and budget ceiling)

**Cannot do:**
- Access other repos (token is scoped to one repository)
- Deploy from repos other than the one that generated the token
- Call control plane after the token expires (5-minute TTL)

**Mitigation:**
- OIDC tokens are short-lived (5 min)
- GitHub Actions logs are auditable
- control plane validates token signature (cannot be forged)
- Repo access controls in GitHub prevent unauthorized workflows

### Stolen Operator Refresh Token

An attacker obtains an operator's refresh token from the profile file.

**Can do:**
- Call control plane as an operator
- Approve tier changes, transfer apps, delete apps, etc.

**Cannot do:**
- Use the token after the legitimate operator performs their next refresh (token is rotated on use)
- Compromise infrastructure outside the Morsel project

**Mitigation:**
- Profile files created with `0600` permissions (owner read/write only)
- Refresh tokens are rotated on every use — stolen token is valid at most once
- Operators are encouraged not to share machines or to use machine encryption
- Token expiry (90 days) — expired tokens are invalid even if stolen

### Compromised App Pod

An attacker gains shell access to an app container.

**Can do:**
- Access the app's own database (as the app's Postgres user)
- Access the app's own blob storage objects
- Access the app's own queues
- Read the app's own Kubernetes Secrets and environment variables

**Cannot do:**
- Access another app's data — database isolation, blob namespacing, and queue namespacing prevent it
- Access Kubernetes Secrets outside the app's namespace (RBAC)
- Access Morsel's infrastructure (different namespace, different service account)
- Access the control plane (different authentication)

**Mitigation:**
- Pod security policies (RunAsNonRoot, etc.)
- Read-only root filesystems where possible
- Regular scanning of container images for vulnerabilities
- NetworkPolicy prevents direct pod-to-pod access
- Developers responsible for app-level security (secure coding, dependency management, etc.)

### DNS Hijacking (Cloudflare)

An attacker obtains the Cloudflare API token from the platform secret store.

**Can do:**
- Modify DNS records for the configured Cloudflare zone
- Redirect traffic for `*.apps.example.com` to attacker infrastructure

**Cannot do:**
- Modify records outside the configured zone (token scope limited)
- Access Cloudflare account settings, modify API tokens, etc.

**Mitigation:**
- Cloudflare API token scoped to single zone, edit-only, no other permissions
- Token is rotated (new token generated during bootstrap, old token revoked)
- DNS propagation is logged in Cloudflare audit trail
- Operator monitors cert alerts for mis-issued certificates

---

## Security by Default

Morsel enforces secure-by-default practices:

- Apps are private (`private: false` is opt-in for public exposure)
- Database and queue access requires app declaration (not automatically provisioned)
- Permanent resources are protected from accidental deletion (two-step removal)
- Credentials are never logged or returned in error responses
- All API traffic uses HTTPS (enforced by the platform load balancer and Kubernetes Ingress)
- All platform API traffic uses the platform's internal network (not the public internet)

---

## Security Audit and Monitoring

Morsel does not provide application-level logging (app stdout/stderr) or request tracing. App owners manage their own observability. Morsel provides:

- Structured error responses with machine-readable codes (no sensitive data in error messages)
- Operation audit trail in SQLite (deploy history, approval actions, etc.)
- Platform audit logs for all platform API calls (see [platform/gcp.md](platform/gcp.md) for GCP Cloud Audit Logs)
- Admin UI visibility into failed deploys, cert issues, and pending approvals

App logs are never included in error responses — developers fetch logs separately to prevent accidental sensitive data leakage.

---

## Design Decisions Log

| Decision | Rationale |
|---|---|
| GitHub calls Morsel (not the other way) | Morsel has no way to reach GitHub or enumerate the org if compromised. |
| Dedicated cloud project | Blast radius of full Morsel compromise is bounded to the Morsel project. |
| No stored GitHub credentials | Credentials cannot be leaked or stolen if they don't exist. |
| Ambient cloud identity throughout | No key files to leak, rotate, or manage. |
| Staging handshake for images | Cross-repo image overwrites are impossible regardless of registry ACLs. |
| Database per app with GRANT scoped to own DB | Cross-app database access is structurally impossible. |
| Pod identity for blob/queue access | Caller identity verified by cloud platform; apps cannot impersonate others. |
| Two-step permanent resource removal | Prevents accidental data loss and is enforced by lint before CI. |
| No pod logs in error responses | Prevents accidental sensitive data leakage through deploy pipeline. |
| Self-hosted runners out of scope | Operational overhead not justified; internet leg accepted as standard GitHub Actions model. |

---

Up: [Index](README.md) · Prev: [Architecture](architecture.md) · Next: [Developer Scenarios](scenarios/developer.md)
