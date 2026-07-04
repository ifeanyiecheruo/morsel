Up: [Index](../README.md) · Prev: [Database Service](database-service.md) · Next: [Platform Interface](../platform/interface.md)

---

# Component — Admin UI

> **Status:** Draft · **Date:** May 2026

---

## Overview

The admin UI is a server rendered multipage App protected by the platform's operator authentication gateway. It is the operator's web interface for day-to-day platform management. No dedicated server pod is required and all data is fetched from the control plane.

---

## Component Diagram

```
Operator browser
  │
  │  HTTPS → admin.example.com
  ▼
Platform operator authentication gateway
  │  validates operator identity
  │  operator principal check
  ▼
Platform HTTP aplication
  │  HTML / JS / CSS served
  │
  │  Makes API calls:
  ▼
control plane (/api/operator/*, /api/repos/*)
  │  All data returned as JSON
```

---

## Personas

**Operators** use the admin UI as their primary management interface. No CLI required for routine operations. The UI is the only interface for batch approvals, stale app review, and cost dashboards.

---

## Sections

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

The admin UI is protected by the platform's operator authentication gateway. The operator navigates to `https://admin.apps.example.com` and is prompted to sign in with their platform identity. The gateway validates the identity and checks that the principal is in the operator principals list configured at bootstrap.

The gateway injects a signed identity token into requests forwarded to the control plane. The control plane verifies the token and exchanges it for a Morsel operator token stored in the server-side session for the duration of the session.

No separate password. No Morsel-specific account. Operators use their existing platform identity. See [platform/gcp.md](../platform/gcp.md) for GCP-specific details (IAP, Google account).

---

## Dollar Cost

| Resource | Cost |
|---|---|
| Operator auth gateway | Platform-dependent (see [platform/gcp.md](../platform/gcp.md)) |
| Compute | Zero — no server pod |

The admin UI has essentially zero marginal cost. All compute cost for the operator experience is borne by the control plane.

---

## Operational Cost

- **Upgrades** — the UI is part of the control plane binary. Updates are applied during normal platform upgrades alongside the control plane.
- **Access management** — operators added/removed via `morsel operator principal add/remove`. No admin UI changes required.
- **Availability** — the control plane serves the UI. The UI is unavailable only if the control plane is unavailable.

---

## Scalability

No scalability considerations for the UI itself. API call throughput is negligible compared to developer deploy traffic.

---

## Security

- Platform operator authentication gateway enforces authentication before any content is served — no anonymous access possible
- Operator principal list is the only access control — managed via `morsel operator principal *`
- All API calls use HTTPS

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
The admin UI relies on the platform's operator authentication gateway for authentication — no login screen in the UI itself. Gateway-issued tokens are exchanged for Morsel operator tokens by the control plane and held in the server-side session. See [platform-features/authentication.md](../platform-features/authentication.md).

---

Up: [Index](../README.md) · Prev: [Database Service](database-service.md) · Next: [Platform Interface](../platform/interface.md)
