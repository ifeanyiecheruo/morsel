Up: [Index](../README.md) · Prev: [Database Service](database-service.md) · Next: [Platform Interface](../platform/interface.md)

---

# Component — Admin UI

> **Status:** Draft · **Date:** May 2026

---

## Overview

The admin UI is a server-rendered multipage app deployed as a dedicated Kubernetes Deployment in the morsel namespace. It is the operator's web interface for day-to-day platform management. All data is fetched from the control plane REST API — the admin UI holds no internal state and has no direct database access.

---

## Component Diagram

```
Operator browser
  │
  │  HTTPS → admin.<baseDomain>
  ▼
morsel-admin-ui (Deployment in morsel namespace)
  │  form-based login (POST /login)
  │  HMAC-signed session cookie
  │  server-rendered HTML pages
  │
  │  Makes API calls over cluster-internal DNS:
  │  http://morsel-api.<ns>.svc.cluster.local:8080
  ▼
morsel-api (/api/operator/*, /api/repos/*)
  │  All data returned as JSON
```

The admin UI is a separate Kubernetes Deployment (`morsel-admin-ui`, port 8090). It has no direct database access — all reads and writes go through the REST API using the session's Bearer token.

---

## Personas

**Operators** use the admin UI as their primary management interface. No CLI required for routine operations. The UI is the only interface for batch approvals, stale app review, and cost dashboards.

---

## Sections

### Operator Management

View all principals with admin UI access. Shows username, password-reset-required flag, and admin flag. Per-principal actions (admin only):

- Set password for another principal (forces password reset on first login optionally)
- Invalidate password — sets `password_reset_required` and updates `password_changed_at`, invalidating all existing tokens

Any authenticated operator can view the principals list. Only admin-role sessions can set or invalidate passwords for other principals.

### App Management

View all apps across all repos. Filter by repo, status, tier, or cost. Per-app actions:

- Hibernate / wake
- Delete (with confirmation)
- Transfer ownership to another repo
- Adjust resource tier (triggers approval workflow)

### Repo Management

View all repos with per-repo summary (app count, cost, tier, last deploy date). Per-repo actions:

- Promote or demote quota tier
- View all apps in repo
- Delete all apps in repo

### Approvals

View all pending approvals across all repos. Columns: repo, app, field, current value, requested value, request date, expiry date.

Batch action UI:

- Select individual approvals or select all
- Choose action: approve / reject (with optional reason) / ignore
- Submit batch

### Stale Apps

List of apps sorted by last deploy date, oldest first. Each entry shows the repo, app name, last deploy date, and a note prompting the operator to verify whether the source repo still exists. Per-entry actions: delete app, ignore (suppress from list for 30 days).

### Cost and Quotas

- Total estimated monthly spend vs. budget ceiling (progress bar)
- Per-repo spend breakdown (sortable table)
- Apps approaching quota ceiling (highlighted)
- Hibernate candidates — running apps with no recent activity approaching their idle threshold

### Platform Status

- Cluster health indicator
- control plane health indicator
- Failed deploys count (last 24h) with link to detail
- Certificate alerts (expiring soon, failed)
- Pending approval count
- Upgrade notification if a newer `morsel` binary is available (informational)

---

## Authentication

The admin UI has its own form-based login page. Operators navigate to `https://admin.<baseDomain>` and authenticate with their username and password.

On successful login the admin UI obtains a Morsel access token + refresh token from `POST /api/token/oidc` and stores them in a signed HttpOnly session cookie (`morsel_admin`). The cookie is HMAC-SHA256 signed with a server-side session key to prevent forgery. MaxAge is 8 hours; the underlying access token is 15 minutes and is silently refreshed by the middleware before expiry.

**Password reset flow:** If `password_reset_required` is set on the principal, any session middleware intercepts navigation and redirects to `/password-reset` until the password is changed. Token refresh is also blocked server-side in this state.

See [platform-features/authentication.md — Admin UI Auth](../platform-features/authentication.md) for the full session flow.

---

## Dollar Cost

| Resource | Allocation | Monthly estimate |
|---|---|---|
| CPU request | 0.1 cores | ~$2 |
| Memory request | 64 MB | ~$0.25 |

The admin UI is a lightweight server-rendered app; its compute cost is small.

---

## Operational Cost

- **Upgrades** — the admin UI is a separate Deployment but uses the same image as the control plane. Rolling pod replacement via `morsel service deploy`. Brief unavailability during switchover.
- **Access management** — principals added/removed via `morsel operator principal add/remove`. Password management via the admin UI operators page.
- **Availability** — the admin UI pod is independent of the control plane pod. Both must be healthy for the full operator experience; the REST API remains available even if the admin UI pod is down.

---

## Scalability

No scalability considerations for the UI itself. API call throughput is negligible compared to developer deploy traffic.

---

## Security

- Login page rejects unauthenticated access — `/login` is the only unauthenticated route
- Session cookie is HttpOnly, SameSite=Lax, HMAC-SHA256 signed — cannot be forged or read by JavaScript
- Access token is carried in the session and verified by the control plane on every API call — the admin UI never grants access beyond what the token allows
- Password reset redirect is enforced at the middleware layer for all protected routes — a user with `password_reset_required` cannot access any other page
- Admin-only operations (set/invalidate another user's password) are gated at the API layer, not just the UI — the API enforces the `admin` role claim

---

## Performance

- API calls: data fetched on navigation; no background polling.
- Large repo lists (100+ repos, 500+ apps): paginated API responses; 

---

## Platform Feature Support

### Hibernation
The admin UI surfaces per-app hibernation status and last-activity timestamp. Operators can force-hibernate or wake any app from the app management view. Hibernate candidates are highlighted in the cost dashboard. See [platform-features/hibernation.md](../platform-features/hibernation.md).

### Cost Controls
The cost dashboard is the operator's primary cost visibility tool. Shows total spend vs. budget ceiling, per-repo breakdown, quota ceiling warnings, and hibernate candidates. Tier promotion is initiated from the repo management view. See [platform-features/cost-controls.md](../platform-features/cost-controls.md).

### Approvals
The approvals section is the operator's primary workflow for actioning pending configuration changes. Supports batch approve/reject/ignore with optional rejection reasons. Reconciliation progress is shown inline after a batch action. See [platform-features/approvals.md](../platform-features/approvals.md).

### Authentication
The admin UI owns its own login page, HMAC-signed session cookies, and silent token refresh middleware. It calls `POST /api/token/oidc` and `POST /api/token/refresh` on the control plane to obtain and rotate tokens. The password-reset flow is gated in the session middleware before any protected page is rendered. See [platform-features/authentication.md](../platform-features/authentication.md).

---

Up: [Index](../README.md) · Prev: [Database Service](database-service.md) · Next: [Platform Interface](../platform/interface.md)
