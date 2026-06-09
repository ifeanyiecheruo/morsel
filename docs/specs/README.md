# Morsel Specification

This folder contains the complete design specification for Morsel, a self-hosted platform-as-a-service for non-production applications.

**Start here:** [vision.md](vision.md) — what Morsel is and the core security constraints.

---

## Reading Guide

### For Everyone: Start With These

1. **[vision.md](vision.md)** (1 min) — what Morsel is and the core security constraints
2. **[goals.md](goals.md)** (1 min) — goals, non-goals, and core objectives
3. **[personas.md](personas.md)** (1 min) — the three personas (developer, operator, designer)
4. **[principles.md](principles.md)** (2 min) — design principles that guide all decisions
5. **[architecture.md](architecture.md)** (6 min) — the system architecture, components, and data flows at 10,000 feet

### Then Choose Your Path

#### "I'm an Operator Who Will Run This Platform"

Read these to understand the operational model:

1. **[scenarios/operator.md](scenarios/operator.md)** (7 min) — 10 concrete scenarios from bootstrap through day-to-day management
2. **[components/cli.md](components/cli.md)** (10 min) — the `morsel` CLI and bootstrap process
3. **[components/admin-ui.md](components/admin-ui.md)** (5 min) — the admin interface
4. **[platform-features/cost-controls.md](platform-features/cost-controls.md)** (11 min) — quota tiers and budget management
5. **[platform-features/approvals.md](platform-features/approvals.md)** (5 min) — the approval workflow for protected config changes
6. **[security-model.md](security-model.md)** (14 min) — security constraints and what you're responsible for

#### "I'm a Developer Who Will Deploy Apps"

Read these to understand the developer experience:

1. **[scenarios/developer.md](scenarios/developer.md)** (6 min) — 10 concrete scenarios from first deploy to local development
2. **[conventions/resource-model.md](conventions/resource-model.md)** (4 min) — how persistence works
3. **[platform-features/deployment.md](platform-features/deployment.md)** (7 min) — the deploy flow and GitHub Actions workflow
4. **[platform-features/authentication.md](platform-features/authentication.md)** (6 min) — how you authenticate
5. Browse **[platform-features/](platform-features/)** for features you care about (hibernation, persistence, etc.)

#### "I'm an Engineer Building or Extending Morsel"

Read in order:

1. All of the "For Everyone" section above
2. **[security-model.md](security-model.md)** (14 min) — comprehensive threat model and design rationale for security decisions
3. **[conventions/](conventions/)** (22 min) — all 5 convention documents; these are the design patterns everything else is built on
4. **[schemas/](schemas/)** (4 min) — authored file schemas; `morsel.json` field reference and JSON Schema for IDE validation
5. **[platform-features/](platform-features/)** (45 min) — all 7 feature docs; each describes a platform capability and which components contribute to it
6. **[components/](components/)** (43 min) — all 6 component docs; deep dive into implementation details for each piece
7. **[platform/interface.md](platform/interface.md)** (6 min) — the Platform abstraction that decouples business logic from cloud-specific code
8. **[platform/gcp.md](platform/gcp.md)** (6 min) — or **[platform/local.md](platform/local.md)** for the platform you're interested in

#### "I Need to Do a Security Review"

1. **[security-model.md](security-model.md)** (14 min) — core constraints, threat model, and attack scenarios
2. **[platform/gcp.md](platform/gcp.md)** (6 min) — or other cloud platform; IAM, WIF, authentication details
3. **[components/morsel-api.md](components/morsel-api.md#security)** — Security section covers API-level auth and RBAC
4. **[platform-features/authentication.md](platform-features/authentication.md)** (6 min) — token model and exchange flows

#### "I Need to Understand a Specific Component"

Example: understand how database provisioning works

1. Read **[platform-features/persistence.md](platform-features/persistence.md)** — gives the feature-level overview
2. Read **[components/database-service.md](components/database-service.md)** — implementation details
3. Read the **Platform Feature Support — Persistence** section in **[components/morsel-api.md](components/morsel-api.md#persistence)** — how Morsel API orchestrates it

---

## Key Concepts

**The Pattern:** Every platform feature (e.g., hibernation, cost controls, approvals) is documented at two levels:

1. **Feature docs** (`platform-features/*.md`) — what the feature does, why it exists, the user-facing behavior
2. **Component contributions** — each component doc has sections describing how it supports each feature

For example, to understand hibernation end-to-end:
- Start with [platform-features/hibernation.md](platform-features/hibernation.md)
- Then read the "Hibernation support" sections in:
  - [components/morsel-api.md](components/morsel-api.md#hibernation)
  - [components/queue-service.md](components/queue-service.md#hibernation)
  - [components/admin-ui.md](components/admin-ui.md#hibernation)

**Conventions vs. Components:** 

- **Conventions** are design patterns and agreements that apply across the entire platform (e.g., "all resources belong to one app", "errors are always structured JSON")
- **Components** are the technical pieces that implement Morsel (Morsel API, blob service, database service, etc.)
- **Platform-features** are the user-visible capabilities (hibernation, quotas, approvals, etc.)

---

## Common Questions

**Q: Where do I find the REST API reference?**
A: [components/morsel-api.md — Functionality](components/morsel-api.md#functionality) lists all endpoints with descriptions.

**Q: How do I understand the deploy flow?**
A: Read [platform-features/deployment.md](platform-features/deployment.md) for the conceptual flow, then [components/morsel-api.md — Deployment](components/morsel-api.md#deployment) for implementation details.

**Q: Where is security documented?**
A: [security-model.md](security-model.md) covers the threat model and architectural constraints. Cloud-specific security is in [platform/gcp.md](platform/gcp.md).

**Q: How do I understand how apps declare what they need (persistence, etc.)?**
A: [conventions/resource-model.md](conventions/resource-model.md) explains the model. [platform-features/deployment.md](platform-features/deployment.md) covers app declarations and file placement. [platform-features/persistence.md](platform-features/persistence.md) describes the persistence feature.

**Q: What happens when an app is deleted?**
A: [conventions/permanence.md](conventions/permanence.md) covers the lifecycle and grace periods. [components/morsel-api.md — Persistence](components/morsel-api.md#persistence) describes the implementation.

**Q: How do I bootstrap the platform?**
A: [scenarios/operator.md — Scenario 1](scenarios/operator.md#scenario-1--initial-platform-setup) walks through the setup. [components/cli.md](components/cli.md) describes the `morsel` CLI. [platform/gcp.md](platform/gcp.md) or [platform/local.md](platform/local.md) cover the cloud-specific details.

---

## Navigating Cross-References

Documents are heavily cross-referenced. Use them to jump between levels of detail:

- In a component doc and want to understand the feature? Look for "Platform Feature Support" sections.
- In a feature doc and want implementation details? Look for "Component Contributions" sections.
- Confused by a convention? Check if it's explained in one of the convention documents.
- Need the REST API shape? Check `components/morsel-api.md`.

---

## Next Steps

- **New to Morsel?** Start with [vision.md](vision.md), [goals.md](goals.md), and [architecture.md](architecture.md)
- **Want to deploy apps?** Go to [scenarios/developer.md](scenarios/developer.md)
- **Want to run the platform?** Go to [scenarios/operator.md](scenarios/operator.md)
- **Building Morsel?** Follow the "For Everyone" path then the engineer path above
- **Security review?** Start with [security-model.md](security-model.md)
