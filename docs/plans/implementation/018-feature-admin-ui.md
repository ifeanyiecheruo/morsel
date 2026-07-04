# Feature 18 — Admin UI

_Delivers: operators have a browser interface for day-to-day management._

**Direct dependencies:** [F03](003-feature-authentication.md), [F05](005-feature-app-lifecycle-api.md), [F13](013-feature-hibernation.md), [F14](014-feature-quota-tiers.md), [F15](015-feature-approvals.md), [F17](017-feature-budget-enforcement.md)

## Tasks

- [ ] Server-rendered multipage app scaffold; HTML pages served directly by the control plane
- [ ] Operator token exchange on first page load — LocalPlatform: POST to local-oidc; calls control plane
- [ ] Token stored in server-side session (signed cookie); no client-side token storage
- [ ] App management view — list all apps; filter by repo/status/tier/cost; per-app actions: hibernate/wake, delete (with confirmation), transfer ownership, adjust resource tier (triggers approval workflow)
- [ ] Repo management view — list repos with per-repo summary (app count, cost, tier, last deploy date); per-repo actions: promote or demote quota tier, view all apps in repo, delete all apps in repo
- [ ] Approvals view — pending approvals table (repo, app, field, current value, requested value, request date, expiry date); batch approve/reject/ignore with optional rejection reason; reconciliation progress shown inline after batch submit
- [ ] Cost and Quotas view — spend vs ceiling progress bar; per-repo breakdown table (sortable); apps approaching quota ceiling highlighted; hibernate candidates (running apps with no recent activity approaching idle threshold)
- [ ] Platform status view — cluster health, control plane health, failed deploys last 24h, cert alerts, pending approval count, upgrade notification if newer `morsel` binary is available
- [ ] Stale apps view — apps sorted by last deploy date oldest-first; per-entry note prompting operator to verify source repo still exists; per-entry actions: delete app, ignore (suppress for 30 days)
- [ ] control plane serves admin UI pages on LocalPlatform (`GET /admin/*` routes)

---

## Page Mockups

Cloudflare console visual style: dark sidebar with `▌` active-item marker, top bar with search and account, content area with title → horizontal rule → table or card layout. Orange primary action buttons in real implementation.

### App Management

```text
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  morsel                                                  [Search...]    admin ▾      │
├───────────┬──────────────────────────────────────────────────────────────────────────┤
│           │  Apps                                                [Refresh]           │
│ ▌ Apps    │  42 apps across 8 repos                                                  │
│   Repos   │  ───────────────────────────────────────────────────────────────────     │
│   Approvals  Repo [All ▾]  Status [All ▾]  Tier [All ▾]  Cost [Any ▾]              │
│   Cost    │                                                                          │
│   Status  │  ☐  App              Repo      Status           Tier   Cost    Deploy   │
│   Stale   │  ─────────────────────────────────────────────────────────────────────  │
│           │  ☐  api-gateway      team-a    ● Running         Pro   $12/mo  2h ago  ⋯│
│           │  ☐  web-frontend     team-a    ● Running        Hobby   $3/mo  1d ago  ⋯│
│           │  ☐  worker-cron      team-b    ◌ Hibernated     Hobby   $0/mo  5d ago  ⋯│
│           │  ☐  data-pipeline    team-b    ● Running         Pro   $18/mo  3d ago  ⋯│
│           │  ☐  auth-service     team-c    ✕ Failed          Pro    $9/mo  8h ago  ⋯│
│           │  ─────────────────────────────────────────────────────────────────────  │
│           │  Showing 1–5 of 42                            [< Prev]  1  2  3  [Next >]│
└───────────┴──────────────────────────────────────────────────────────────────────────┘

                                          ⋯ row menu:
                                          ┌──────────────────────┐
                                          │  Hibernate           │
                                          │  Wake                │
                                          │  Transfer ownership  │
                                          │  Adjust tier…        │
                                          │  ────────────────    │
                                          │  Delete…             │
                                          └──────────────────────┘
```

### Repo Management

```text
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  morsel                                                  [Search...]    admin ▾      │
├───────────┬──────────────────────────────────────────────────────────────────────────┤
│           │  Repos                                               [Refresh]           │
│   Apps    │  8 repos                                                                 │
│ ▌ Repos   │  ───────────────────────────────────────────────────────────────────     │
│   Approvals                                                                         │
│   Cost    │  Repo           Apps   Est. Cost   Tier    Last Deploy                  │
│   Status  │  ─────────────────────────────────────────────────────────────────────  │
│   Stale   │  team-a           12    $42/mo      Pro     2h ago                    ⋯ │
│           │  team-b            8    $31/mo     Hobby    1d ago                    ⋯ │
│           │  team-c            5    $18/mo      Pro     8h ago                    ⋯ │
│           │  team-d            2     $6/mo     Hobby    12d ago                   ⋯ │
│           │  ─────────────────────────────────────────────────────────────────────  │
│           │  Showing 1–4 of 8                              [< Prev]  1  2  [Next >]  │
└───────────┴──────────────────────────────────────────────────────────────────────────┘

                                          ⋯ row menu:
                                          ┌────────────────────┐
                                          │  View apps         │
                                          │  Promote tier      │
                                          │  Demote tier       │
                                          │  ────────────────  │
                                          │  Delete all apps…  │
                                          └────────────────────┘
```

### Approvals

```text
┌──────────────────────────────────────────────────────────────────────────────────────────┐
│  morsel                                                      [Search...]    admin ▾      │
├───────────┬──────────────────────────────────────────────────────────────────────────────┤
│           │  Approvals                                     3 pending                     │
│   Apps    │  ───────────────────────────────────────────────────────────────────────     │
│   Repos   │  ☐ Select all    [Approve selected]  [Reject…]  [Ignore]                    │
│ ▌ Approvals                                                                              │
│   Cost    │  ☐  Repo     App              Field      Current   Requested  Req'd   Exp.  │
│   Status  │  ──────────────────────────────────────────────────────────────────────────  │
│   Stale   │  ☐  team-a   api-gateway      tier        Hobby     Pro       Jun 28  Jul 12 │
│           │  ☐  team-b   data-pipeline    tier        Hobby     Pro       Jun 30  Jul 14 │
│           │  ☐  team-c   auth-service     replicas    1         3         Jul 1   Jul 15  │
│           │  ──────────────────────────────────────────────────────────────────────────  │
└───────────┴──────────────────────────────────────────────────────────────────────────────┘

  [Reject…] opens:
  ┌─────────────────────────────────────────────┐
  │  Reject 2 approvals                          │
  │  ───────────────────────────────────────     │
  │  Reason (optional)                           │
  │  ┌───────────────────────────────────────┐   │
  │  │ Budget freeze until Q3                │   │
  │  └───────────────────────────────────────┘   │
  │                      [Cancel]  [Reject]       │
  └─────────────────────────────────────────────┘

  After submit, inline reconciliation progress replaces the batch bar:
  ┌────────────────────────────────────────────────────────────────────────────────────┐
  │  Rejecting 2 approvals…  ████████████████████░░░░░  team-b/data-pipeline ✓   …    │
  └────────────────────────────────────────────────────────────────────────────────────┘
```

### Cost & Quotas

```text
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  morsel                                                  [Search...]    admin ▾      │
├───────────┬──────────────────────────────────────────────────────────────────────────┤
│           │  Cost & Quotas                                                           │
│   Apps    │  ───────────────────────────────────────────────────────────────────     │
│   Repos   │  Monthly Spend                                                           │
│   Approvals  $112/mo of $200 budget   ██████████████████░░░░░░░░░░░░   56%          │
│ ▌ Cost    │                                                                          │
│   Status  │  Per-repo breakdown  ↕ Cost                                              │
│   Stale   │  ─────────────────────────────────────────────────────────────────────  │
│           │  team-a       $42/mo    21%   12 apps   Pro                              │
│           │  team-b       $31/mo    16%    8 apps   Hobby                            │
│           │  team-c       $18/mo     9%    5 apps   Pro                              │
│           │  team-d        $6/mo     3%    2 apps   Hobby                            │
│           │                                                                          │
│           │  ⚠ Approaching quota ceiling                                             │
│           │  ─────────────────────────────────────────────────────────────────────  │
│           │  team-a / api-gateway   Pro quota   $48 of $50   ████████████████░  96% │
│           │                                                                          │
│           │  Hibernate candidates                                                    │
│           │  ─────────────────────────────────────────────────────────────────────  │
│           │  team-b / worker-cron   idle 5d   threshold 7d   $3/mo   [Hibernate]    │
└───────────┴──────────────────────────────────────────────────────────────────────────┘
```

### Platform Status

```text
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  morsel                                                  [Search...]    admin ▾      │
├───────────┬──────────────────────────────────────────────────────────────────────────┤
│           │  Platform Status                                                         │
│   Apps    │  ───────────────────────────────────────────────────────────────────     │
│   Repos   │  ● Cluster           Healthy                                             │
│   Approvals  ● Control plane     Healthy                                             │
│   Cost    │                                                                          │
│ ▌ Status  │  ───────────────────────────────────────────────────────────────────     │
│   Stale   │  Failed deploys (last 24h)       2              [View details →]        │
│           │  Certificate alerts              1 expiring soon                        │
│           │  Pending approvals               3                                       │
│           │                                                                          │
│           │  ───────────────────────────────────────────────────────────────────     │
│           │  ⓘ morsel v1.2.0 available (you have v1.1.3)   [View release notes →]  │
└───────────┴──────────────────────────────────────────────────────────────────────────┘
```

### Stale Apps

```text
┌──────────────────────────────────────────────────────────────────────────────────────┐
│  morsel                                                  [Search...]    admin ▾      │
├───────────┬──────────────────────────────────────────────────────────────────────────┤
│           │  Stale Apps                                                              │
│   Apps    │  Apps with no deploy in 30+ days. Verify the source repo still exists.  │
│   Repos   │  ───────────────────────────────────────────────────────────────────     │
│   Approvals                                                                         │
│   Cost    │  Repo       App               Last Deploy   Note                        │
│   Status  │  ─────────────────────────────────────────────────────────────────────  │
│ ▌ Stale   │  team-d     legacy-worker     90d ago       ⚠ Verify source repo exists │
│           │  team-b     old-api           67d ago       ⚠ Verify source repo exists │
│           │  team-c     test-harness      45d ago       ⚠ Verify source repo exists │
│           │  team-a     proto-v1          33d ago       ⚠ Verify source repo exists │
│           │  ─────────────────────────────────────────────────────────────────────  │
│           │  Per entry:  [Delete app]  [Ignore for 30d]                             │
└───────────┴──────────────────────────────────────────────────────────────────────────┘
```
