Up: [Index](README.md) · Prev: [Deployment](deployment.md) · Next: [Hibernation](hibernation.md)

---

# Platform Feature — Authentication

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel uses a two-token model. GitHub Actions workflows exchange a short-lived GitHub OIDC token for a short-lived Morsel access token. Operators exchange a platform identity token for a Morsel access token plus a long-lived refresh token. All access tokens are JWTs verified by signature — no database lookup per request. No long-lived credentials exist anywhere in the system.

---

## Token Model

| Token | Lifetime | Storage | Purpose |
|---|---|---|---|
| Deploy identity token | Short-lived | Never stored — ephemeral in the deploy environment | Proof of which repo is deploying; platform-specific form (see below) |
| Platform identity token | Short-lived | Never stored | Proof of operator identity for Morsel auth |
| Morsel access token | 15 min (configurable) | In memory only — never written to disk | Bearer token for all Morsel API calls |
| Morsel refresh token | 90 days | SQLite server-side + `~/.config/morsel/<profile>.profile.json` client-side | Silently refreshes the access token |

The deploy identity token form is platform-determined: on GCPPlatform it is a GitHub OIDC JWT; on LocalPlatform it is a locally-signed JWT. In both cases it is submitted to `POST /api/token/deploy` and never persisted.

The access token TTL is the maximum lag before role changes take effect. Revoking a refresh token takes effect within one access token TTL (default 15 minutes).

---

## Exchange Endpoints

```
POST /api/token/deploy            → access token (10 min), no refresh token
POST /api/token/<platform-oidc>   → access token (15 min) + refresh token (90 days)
POST /api/token/refresh           → access token (15 min) + refresh token (90 days, rotated)
```

The platform-specific OIDC endpoint path (e.g., `/api/token/gcp-oidc` on GCP) is determined at bootstrap. See [platform/gcp.md](../platform/gcp.md).

The deployer re-exchanges on every deploy run and does not need a refresh token. The 10-minute access token covers typical deploy duration.

---

## Deploy Auth Flow

`morsel app deploy` always calls `Platform.DeployCredentials()` to obtain a deploy identity token, then submits it to `POST /api/token/deploy`. The Morsel API delegates validation to `Platform.ValidateDeployToken()`. The deploy command has no knowledge of GitHub JWKS, local signing keys, or any other platform-specific mechanism.

```
morsel app deploy
  → Platform.DeployCredentials() → deploy identity token
  → POST /api/token/deploy  { token: "<deploy-identity-token>" }

Morsel API
  → Platform.ValidateDeployToken(token) → repo slug
  → issues:
    - Morsel access token (10 min):
        { sub: "repo:{slug}", repo: "{slug}", role: "developer", exp: ... }
    - staging registry push credentials (10 min, if applicable):
        scoped to the caller's staging registry path

morsel app deploy
  → uses Morsel access token for all subsequent API calls
  → uses registry credentials to push images
  → re-exchanges on next deploy run (no refresh token)
```

Platform-specific implementations of `DeployCredentials()` and `ValidateDeployToken()` are documented in the platform docs. See [platform/gcp.md](../platform/gcp.md) and [platform/local.md](../platform/local.md).

---

## Operator Auth Flow (CLI)

```
Operator runs: morsel operator login
  → CLI opens system browser → platform OAuth consent screen
  → Operator authenticates with their platform identity
  → Platform OAuth callback → localhost listener in CLI binary
  → CLI holds platform identity token in memory

  → POST /api/token/platform-oidc  { token: "<platform-identity-token>" }

Morsel API
  → validates platform identity token
  → checks identity against configured operator principal(s)
  → issues:
    - Morsel access token (15 min): { sub: "operator:alice@example.com", role: "operator", exp: ... }
    - Morsel refresh token (90 days): opaque, stored in SQLite

CLI
  → writes profile to ~/.config/morsel/<profile>.profile.json (0600 permissions)
  → platform identity token is discarded — not persisted
```

The token exchange endpoint path is platform-specific (e.g., `/api/token/gcp-oidc` on GCP). See [platform/gcp.md](../platform/gcp.md).

---

## Silent Token Refresh

On every CLI command:

1. Load `~/.config/morsel/<profile>.profile.json`
2. Check `access_token_expires_at` — if valid, proceed
3. If expired, call `POST /api/token/refresh` silently → update profile on success
4. If refresh token expired or absent, trigger interactive platform OAuth browser flow

The operator never sees token expiry in normal use.

Refresh tokens are rotated on every use — the old token is invalidated and a new one is issued. A stolen refresh token can be used at most once before the legitimate user's next refresh invalidates it.

---

## Admin UI Auth

The admin UI is protected by the platform's operator authentication gateway. The gateway handles the full OAuth flow before any request reaches the admin UI or the Morsel API. The operator authenticates with their existing platform identity — no separate password.

The gateway calls the Morsel API's platform-specific token exchange endpoint on behalf of the authenticated operator to obtain a Morsel token for API calls made by the UI. See [platform/gcp.md](../platform/gcp.md) for GCP-specific details.

---

## RBAC

Role is determined at token exchange time and encoded in the access token. No per-request directory lookup is needed.

| Role | How acquired | Encoded in token |
|---|---|---|
| `developer` | Deploy identity token exchange (`POST /api/token/deploy`) | `repo: "{slug}"` claim |
| `operator` | Platform OIDC exchange matching operator principal | `role: "operator"` claim |

| Operation | Developer | Operator |
|---|---|---|
| Deploy, update own repo's apps | ✓ | ✓ |
| Hibernate, wake, delete own repo's apps | ✓ | ✓ |
| List and view own repo's apps, cost, and quota | ✓ | ✓ |
| View pending approvals for own repo | ✓ | ✓ |
| View all repos and apps | ✗ | ✓ |
| Batch approve, reject, or ignore approvals | ✗ | ✓ |
| Transfer app ownership | ✗ | ✓ |
| Promote repo tier | ✗ | ✓ |
| Override permanent resource protection (`?force=true`) | ✗ | ✓ |
| Delete apps from archived or deleted repos | ✗ | ✓ |
| `morsel service bootstrap` | ✗ | ✓ |

Developers are scoped to their own repo's apps. Any request where the token's `repo` claim does not match the `:slug` in the URL returns 403.

---

## Token Subject

The `sub` claim in a Morsel token identifies the principal:

```
Developer token:  sub = "repo:org/my-repo"
Operator token:   sub = "operator:alice@example.com"
```

For developer tokens the subject is the repo slug returned by `Platform.ValidateDeployToken()` — `org/my-repo` on GCPPlatform, `localhost/my-app` on LocalPlatform. It is the stable identity used for all authorization checks — renaming a repo or moving a local directory changes the subject, and the app association must be updated via operator transfer.

---

## Revocation

Revoking a refresh token is a single SQLite delete. The associated access token remains valid until expiry — at most one TTL period (default 15 minutes).

```
morsel operator principal remove --principal alice@example.com
```

This revokes all refresh tokens held by that operator principal. Their access token expires within 15 minutes.

---

## No Long-Lived Credentials

| Relationship | Mechanism | Stored secret? |
|---|---|---|
| Deployer → staging container registry | Platform identity federation (GCPPlatform); direct push (LocalPlatform) | No |
| Deployer → Morsel API | Deploy identity token via `POST /api/token/deploy` (platform-specific form, short-lived) | No |
| Morsel API → container registry | Ambient cloud identity | No |
| Morsel API → object storage | Ambient cloud identity | No |
| Cluster nodes → container registry | Ambient node identity | No |
| Operator CLI → Morsel API | Morsel refresh token (rotated on use) | Profile file only |

---

## Component Contributions

### Morsel API
Owns all token exchange endpoints, JWT signing key management, refresh token store in SQLite, and RBAC middleware. See [components/morsel-api.md — Authentication](../components/morsel-api.md).

### CLI
Owns the platform OAuth browser flow, profile file management, silent refresh logic, and `morsel operator login/logout`. See [components/cli.md — Authentication](../components/cli.md).

### Platform
Owns identity federation configuration, operator auth gateway setup, and the platform-specific token exchange mechanism. See [platform/gcp.md](../platform/gcp.md) for GCP specifics.

---

Up: [Index](README.md) · Prev: [Deployment](deployment.md) · Next: [Hibernation](hibernation.md)
