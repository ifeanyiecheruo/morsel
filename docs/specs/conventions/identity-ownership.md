Up: [Index](README.md) · Prev: [Resource Model](resource-model.md) · Next: [Idempotency](idempotency.md)

---

# Convention — Identity and Ownership

> **Status:** Draft · **Date:** May 2026

---

## Summary

Apps belong to GitHub repositories. The repository is the unit of identity and ownership in Morsel. Ownership is verified cryptographically from the GitHub OIDC token — no GitHub API calls, no org membership queries, no stored GitHub credentials.

---

## The Repo as Owner

Every app in Morsel belongs to exactly one GitHub repository. The repo is the stable, unambiguous identity that Morsel can verify without any GitHub API access.

When a GitHub Actions workflow deploys an app, it presents a short-lived OIDC JWT signed by GitHub. That token contains a `repository` claim — for example, `org/my-repo`. Morsel validates the token signature against GitHub's public JWKS endpoint and extracts the `repository` claim. No other identity concept is used.

There is no concept of a user owning an app. Anyone who can trigger a GitHub Actions workflow on a repo can deploy that repo's apps. There are no per-developer permissions, no team assignments, and no ACLs within a repo.

---

## What Morsel Can Verify Without GitHub API Access

| Claim | Source | How verified |
|---|---|---|
| `repository` — which repo is deploying | GitHub OIDC JWT | Token signature against GitHub JWKS (public, read-only) |
| `ref` — which git ref triggered the workflow | GitHub OIDC JWT | Same token |
| `sha` — the commit SHA being deployed | GitHub OIDC JWT | Same token |
| `workflow` — which workflow file ran | GitHub OIDC JWT | Same token |

What Morsel cannot verify and does not attempt to verify:
- Org membership
- Team membership  
- Who specifically triggered the workflow
- Whether the repo still exists (no webhook events received)

---

## Auto-Registration on First Deploy

When Morsel receives the first deploy request from a previously unknown repo, it automatically creates a repo record and assigns the default quota tier. The deploy succeeds immediately. The operator receives a digest notification but is not in the critical path.

This means there is no onboarding step, no approval required to join the platform, and no operator involvement for routine first deploys. Cost exposure is bounded by the default quota tier.

---

## Ownership in the Morsel Token

When a GitHub OIDC token is exchanged for a Morsel token, the `repository` claim is encoded directly into the Morsel token:

```json
{
  "sub": "repo:org/my-repo",
  "repo": "org/my-repo",
  "role": "developer",
  "exp": 1234567890
}
```

All subsequent API calls are authorized based on this claim. Developers can only read or modify apps where the token's `repo` claim matches the app's registered repo. Any attempt to access another repo's apps returns 403.

---

## Ownership Lifecycle

| Event | Platform behaviour |
|---|---|
| New contributor gains repo access | Automatically gains deploy access on next workflow run. No operator action. |
| Contributor loses repo access | Access revoked on next OIDC token validation. Apps remain under repo ownership. |
| Repo transferred to another org/team in GitHub | New owners deploy; Morsel updates ownership on next successful deploy. |
| Repo deleted or archived in GitHub | App becomes orphaned — see below. |
| Unusual circumstance | Operator can transfer app ownership directly via admin UI. |

---

## Orphaned Apps

An app is orphaned when its GitHub repo has been deleted or archived. Morsel has no way to detect this condition automatically — it holds no GitHub token and receives no GitHub webhook events. Orphaned apps will continue running and consuming resources until the operator identifies and removes them.

The recommended approach is periodic manual review of the stale apps list in the admin UI, sorted by last deploy date. Apps with no deploy activity for an extended period warrant verification that the source repo still exists.

This is a known limitation recorded in the design. Automated detection would require either a GitHub credential (ruled out by the security model) or a webhook subscription (adds operational complexity). The tradeoff is accepted for a non-production platform with part-time operator oversight.

---

## Operator Identity

The operator authenticates to Morsel via a platform OAuth browser flow. Morsel checks whether the authenticated identity matches the operator principal configured at bootstrap. If it matches, the resulting Morsel token carries the `operator` role. See [platform/gcp.md](../platform/gcp.md) for GCP-specific identity details.

Operator principals are managed via `morsel operator principal add/remove/list`. Morsel never manages Google Group membership — if a group is used as the operator principal, group membership is managed externally and Morsel treats the group as a single opaque principal.

---

## Two Roles

Morsel has exactly two roles:

| Role | How acquired | Scope |
|---|---|---|
| `developer` | GitHub OIDC exchange | Own repo's apps only |
| `operator` | Platform OIDC exchange, identity matches operator principal | All repos and apps |

Role is encoded in the Morsel token at issuance and requires no per-request database lookup. A role change (e.g., revoking operator access) takes effect within one access token TTL (default 15 minutes).

---

Up: [Index](README.md) · Prev: [Resource Model](resource-model.md) · Next: [Idempotency](idempotency.md)
