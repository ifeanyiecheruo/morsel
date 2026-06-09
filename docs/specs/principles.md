Up: [Index](README.md) · Prev: [Personas](personas.md) · Next: [Architecture](architecture.md)

---

# Design Principles

> **Status:** Draft · **Date:** May 2026

---

These principles explain the compromises made throughout the design. When a later decision seems odd in isolation, it is usually in service of one of these.

---

## Low, controllable dollar cost

The platform must cost little to run even when underutilised. Hibernation (scale-to-zero for idle apps) is central to this. Quota tiers bound per-repo spend. The cost dashboard gives the operator visibility without requiring them to read cloud billing directly. Every design choice that increases cost requires a proportionate justification.

---

## Low operator overhead

The operator role must be viable part-time, by someone without cloud expertise, and must survive rotation. This rules out: ad-hoc kubectl commands, cloud console operations, manual cert renewals, manual DNS changes, and per-app configuration managed outside the platform. If a routine operation requires the operator to consult documentation, it is a platform failure.

---

## Safe to run adjacent to enterprise

Morsel runs in a dedicated cloud project with no cross-project privilege grants, no GitHub credentials, and no ability to enumerate the GitHub organisation. A fully compromised Morsel cannot be used as a stepping stone into the operator's wider infrastructure. This boundary is non-negotiable — any proposed change that would expand the blast radius requires explicit designer sign-off.

---

## Self-service with opinionated defaults

Developers should be unblocked without operator involvement for routine operations. The first deploy from a new repo succeeds automatically. Resources have safe defaults. The operator is in the critical path only for quota increases and protected config changes. This means the defaults must be genuinely good — not minimal stubs that force every developer to ask for an upgrade.

---

## Few knobs

Configuration surface for developers is intentionally small. `morsel.json` has a short field list. Fields that control resource sizes go through the approval workflow. Fields that do not matter for most apps are omitted. When a reasonable default exists, it is used without exposing a knob. Developers who want more control ask the operator, not the config file.

---

## End-to-end solutions, not primitives

Morsel provides complete managed solutions: a working database connection with no credentials to manage, blob storage with no SDK to configure, a queue with fixed semantics. The design explicitly rejects exposing raw cloud primitives (buckets, Pub/Sub topics, Cloud SQL instances) to app developers. Managed access via fixed internal hostnames with zero configuration is the goal.

---

Up: [Index](README.md) · Prev: [Personas](personas.md) · Next: [Architecture](architecture.md)
