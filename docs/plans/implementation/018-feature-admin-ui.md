# Feature 18 — Admin UI

_Delivers: operators have a browser interface for day-to-day management._

**Direct dependencies:** [F03](003-feature-authentication.md), [F05](005-feature-app-lifecycle-api.md), [F13](013-feature-hibernation.md), [F14](014-feature-quota-tiers.md), [F15](015-feature-approvals.md), [F17](017-feature-budget-enforcement.md)

## Tasks

- [ ] React + TypeScript SPA scaffold (Vite); production build outputs a static bundle
- [ ] Operator token exchange on page load — LocalPlatform: POST to local-oidc; calls control plane
- [ ] Token storage — in-memory only (no localStorage, no cookies)
- [ ] App management view — list all apps; filter by repo/status/tier; per-app hibernate/wake/delete actions
- [ ] Repo management view — list repos; tier promotion button
- [ ] Approvals view — pending approvals table with batch approve/reject/ignore UI
- [ ] Cost dashboard — spend vs ceiling progress bar; per-repo breakdown table; hibernate candidates
- [ ] Platform status view — component health, cert alerts, failed deploys, pending approvals count
- [ ] Stale apps view — apps sorted by last deploy date; suppress-for-30-days per entry
- [ ] control plane serve static bundle on LocalPlatform (`GET /admin/*` route)
