Up: [Index](README.md) · Prev: [Deployment](deployment.md) · Next: [Hibernation](hibernation.md)

---

# Platform Feature — Authentication

> **Status:** Draft · **Date:** May 2026

---

## Summary

Morsel uses a two-token model. GitHub Actions workflows exchange a short-lived deploy identity token for a short-lived Morsel access token. Operators authenticate with a username and password stored in the platform's `principals` table to obtain a Morsel access token plus a long-lived refresh token. All access tokens are JWTs verified by signature — no database lookup per request.

---

## Token Model

| Token | Lifetime | Storage | Purpose |
|---|---|---|---|
| Deploy identity token | Short-lived | Never stored — ephemeral in the deploy environment | Proof of which repo is deploying; platform-specific form (see below) |
| Morsel access token | 10 min (developer) / 15 min (operator) | In memory only — never written to disk | Bearer token for all control plane calls |
| Morsel refresh token | 90 days | SQLite server-side + `~/.config/morsel/<profile>.profile.json` client-side | Silently refreshes the access token |

The deploy identity token form is platform-determined: on GCPPlatform it is a GitHub OIDC JWT; on LocalPlatform it is a locally-signed JWT. In both cases it is submitted to `POST /api/token/deploy` and never persisted.

Operator passwords are stored as bcrypt hashes in the `principals` table (see [components/control-plane.md](../components/control-plane.md)). The plaintext password is never stored or logged.

The access token TTL is the maximum lag before role changes take effect. A password change immediately invalidates existing refresh tokens — the token refresh endpoint rejects tokens issued before the recorded `password_changed_at` timestamp.

---

## Exchange Endpoints

```
POST /api/token/deploy   → access token (10 min), no refresh token
POST /api/token/oidc     → access token (15 min) + refresh token (90 days)
POST /api/token/refresh  → access token (15 min) + refresh token (90 days, rotated)
```

The deployer re-exchanges on every deploy run and does not need a refresh token. The 10-minute access token covers typical deploy duration.

---

## Deploy Auth Flow

`morsel app deploy` always calls `Platform.DeployToken()` to obtain a deploy identity token, then submits it to `POST /api/token/deploy`. The control plane delegates validation to `Platform.ValidateDeployToken()`. The deploy command has no knowledge of GitHub JWKS, local signing keys, or any other platform-specific mechanism.

```
morsel app deploy
  → Platform.DeployToken() → deploy identity token
  → POST /api/token/deploy  { token: "<deploy-identity-token>" }

control plane
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

Platform-specific implementations of `DeployToken()` and `ValidateDeployToken()` are documented in the platform docs. See [platform/gcp.md](../platform/gcp.md) and [platform/local.md](../platform/local.md).

---

## Operator Auth Flow (CLI)

```
Operator runs: morsel operator login
  → CLI prompts for username and password
  → POST /api/token/oidc  { username: "alice", password: "<password>" }

control plane
  → looks up principal in principals table
  → verifies bcrypt(password) against stored hash
  → checks password_reset_required flag
  → issues:
    - Morsel access token (15 min):
        { sub: "alice", role: "operator", exp: ... }
        (role is "admin" if the principal has is_admin = 1)
    - Morsel refresh token (90 days): opaque, stored in SQLite
    - PasswordResetRequired: true if flag is set (operator must change password)

CLI
  → if PasswordResetRequired, prompts for new password
  → calls POST /api/operator/password { current_password, new_password }
  → writes profile to ~/.config/morsel/<profile>.profile.json (0600 permissions)
```

If `password_reset_required` is set on the principal, the token pair is returned together with a `password_reset_required: true` flag. The CLI intercepts this and forces an interactive password change before saving the profile. Token refresh is also rejected until the password is changed.

**First-time setup:** `morsel service deploy` calls `POST /bootstrap` after provisioning to create the first admin principal. A random password is generated and printed once. The first principal is always an admin and always has `password_reset_required` set.

---

## Silent Token Refresh

On every CLI command:

1. Load `~/.config/morsel/<profile>.profile.json`
2. Check `access_token_expires_at` — if valid, proceed
3. If expired, call `POST /api/token/refresh` silently → update profile on success
4. If refresh token expired or absent, prompt for username and password

The operator never sees token expiry in normal use.

`POST /api/token/refresh` rejects tokens in two additional cases:

- `password_reset_required` is set on the principal
- Token was issued before the principal's `password_changed_at` timestamp (password changed since the token was issued)

Refresh tokens are rotated on every use — the old token is invalidated and a new one is issued. A stolen refresh token can be used at most once before the legitimate user's next refresh invalidates it.

---

## Admin UI Auth

The admin UI has its own form-based login page. No external authentication gateway is involved.

```
Operator navigates to https://admin.<baseDomain>
  → GET /login  — renders login form

POST /login { username, password }
  → admin UI calls POST /api/token/oidc { username, password }
  → on success: stores access token + refresh token in a signed HttpOnly session cookie
  → if password_reset_required: redirects to /password-reset before any other page

Session cookie:
  name: morsel_admin
  format: base64url(JSON) + "." + base64url(HMAC-SHA256)
  MaxAge: 8 hours (long-lived compared to the 15-min access token)
  HttpOnly, SameSite=Lax

Silent token refresh:
  → when access token has < 30 s remaining, the admin UI calls POST /api/token/refresh
  → on success: writes a new session cookie to the response
  → on failure: redirects to /login
```

Admin UI sessions carry the same role-encoded access token as CLI sessions. Pages and actions that require the `admin` role check this claim in the session's access token without a round-trip to the API.

---

## RBAC

Role is determined at token exchange time and encoded in the access token. No per-request directory lookup is needed.

| Role | How acquired | Encoded in token |
|---|---|---|
| `developer` | Deploy identity token exchange (`POST /api/token/deploy`) | `repo: "{slug}"` claim |
| `operator` | Password exchange (`POST /api/token/oidc`) for a principal where `is_admin = 0` | `role: "operator"` claim |
| `admin` | Password exchange (`POST /api/token/oidc`) for a principal where `is_admin = 1` | `role: "admin"` claim |

`admin` is a superset of `operator` — all operator-level access is granted, plus admin-only operations.

| Operation | Developer | Operator | Admin |
|---|---|---|---|
| Deploy, update own repo's apps | ✓ | ✓ | ✓ |
| Hibernate, wake, delete own repo's apps | ✓ | ✓ | ✓ |
| List and view own repo's apps, cost, and quota | ✓ | ✓ | ✓ |
| View pending approvals for own repo | ✓ | ✓ | ✓ |
| View all repos and apps | ✗ | ✓ | ✓ |
| Batch approve, reject, or ignore approvals | ✗ | ✓ | ✓ |
| Transfer app ownership | ✗ | ✓ | ✓ |
| Promote repo tier | ✗ | ✓ | ✓ |
| Override permanent resource protection (`?force=true`) | ✗ | ✓ | ✓ |
| Delete apps from archived or deleted repos | ✗ | ✓ | ✓ |
| `morsel service deploy` | ✗ | ✓ | ✓ |
| Require password reset for any principal | ✗ | ✓ | ✓ |
| Set another principal's password | ✗ | ✗ | ✓ |
| Invalidate another principal's password | ✗ | ✗ | ✓ |

Developers are scoped to their own repo's apps. Any request where the token's `repo` claim does not match the `:slug` in the URL returns 403.

The first principal created via `POST /bootstrap` is always assigned `is_admin = 1`. Subsequent principals added via `morsel operator principal add` default to `is_admin = 0` (operator role).

---

## Token Subject

The `sub` claim in a Morsel token identifies the principal:

```
Developer token:  sub = "repo:org/my-repo"
Operator token:   sub = "alice"  (the principal's username)
```

For developer tokens the subject is the repo slug returned by `Platform.ValidateDeployToken()` — `org/my-repo` on GCPPlatform, `localhost/my-app` on LocalPlatform. It is the stable identity used for all authorization checks — renaming a repo or moving a local directory changes the subject, and the app association must be updated via operator transfer.

---

## Revocation

Revoking a refresh token is a single SQLite delete. The associated access token remains valid until expiry — at most one TTL period (default 15 minutes).

```
morsel operator principal remove --principal alice
```

This deletes the principal row and revokes all refresh tokens held by that principal. Their access token expires within 15 minutes.

A password change has an equivalent effect on refresh tokens: `POST /api/token/refresh` rejects any token whose `created_at` is earlier than the principal's `password_changed_at`. This means a password reset forces all existing sessions to re-authenticate.

An admin can also force this effect without changing the password via `POST /api/operator/principals/:principal/invalidate-password`, which sets `password_changed_at = now()` and `password_reset_required = 1`.

---

## Credential Storage

| Relationship | Mechanism | Stored secret? |
|---|---|---|
| Deployer → staging container registry | Platform identity federation (GCPPlatform); direct push (LocalPlatform) | No |
| Deployer → control plane | Deploy identity token via `POST /api/token/deploy` (platform-specific form, short-lived) | No |
| control plane → container registry | Ambient cloud identity | No |
| control plane → object storage | Ambient cloud identity | No |
| Cluster nodes → container registry | Ambient node identity | No |
| Operator CLI → control plane | Morsel refresh token (rotated on use) | Profile file only |
| Operator → control plane (auth) | bcrypt-hashed password | SQLite `principals` table (server-side only) |

---

## Component Contributions

### Control Plane
Owns all token exchange endpoints, JWT signing key management, refresh token store in SQLite, RBAC middleware, and the `principals` table (bcrypt password hashes, `password_reset_required`, `password_changed_at`, `is_admin`). See [components/control-plane.md — Authentication](../components/control-plane.md).

### Admin UI
Owns the form-based login page, HMAC-signed session cookie management, silent token refresh, and the password-reset flow. See [components/admin-ui.md — Authentication](../components/admin-ui.md).

### CLI
Owns profile file management, interactive username/password prompts, silent refresh logic, `morsel operator login/logout`, and the inline password-change flow for first-login. See [components/cli.md — Authentication](../components/cli.md).

### Platform
Owns deploy identity token generation and validation (platform-specific). See [platform/gcp.md](../platform/gcp.md) and [platform/local.md](../platform/local.md).

---

Up: [Index](README.md) · Prev: [Deployment](deployment.md) · Next: [Hibernation](hibernation.md)
