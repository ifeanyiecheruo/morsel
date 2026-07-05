Up: [Index](README.md) · Prev: [Goals](goals.md) · Next: [Principles](principles.md)

---

# Personas

> **Status:** Draft · **Date:** May 2026

---

## Developer / app owner

A member of the GitHub organisation. Deploys and operates apps on behalf of their team. No cloud or Kubernetes knowledge required or expected. Their entire interface is a git repository and `*.morsel.json` configuration files.

Developers care about: fast deploys, predictable URLs, working persistence, and clear error messages when something goes wrong.

---

## Operator

Manages the running platform on a part-time basis. The role may rotate. The operator has no expectation of Kubernetes or Cloud provider knowledge. Their entire workflow is two tools: the `morsel` CLI (for installation, upgrades, and access management) and the admin UI (for day-to-day management). Routine tasks should take minutes.

The operator's task surface is deliberately narrow:

- Install or upgrade the platform
- Review and promote repos to higher quota tiers on request
- Transfer app ownership in unusual circumstances
- Periodically review and clean up apps from deleted or archived repos

---

## Admin

A special operator role assigned to the first principal created at bootstrap. The admin role is a superset of operator: all operator actions are available plus password management for other principals (set password, force password reset, invalidate credentials).

---

## Designer / implementer

Builds and evolves the platform itself. Comfortable with Kubernetes internals and cloud infrastructure. Interacts with the full stack. This role is not the operator — complexity is acceptable here because it is absorbed once during construction and rarely during maintenance.

---

Up: [Index](README.md) · Prev: [Goals](goals.md) · Next: [Principles](principles.md)
