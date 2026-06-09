# Morsel Specification Refactoring - Conversation Archive

> **Date:** May 2026
> **Archive Date:** 2026-06-06

---

## Overview

This archive contains the complete conversation history for the Morsel specification refactoring project. The conversation involved breaking up a monolithic design document (`overall-design.md`) into a set of focused, modular specification documents organized by purpose.

---

## What Was Accomplished

### Phase 1: Initial Refactoring
- Refactored `overall-design.md` into 27 focused markdown documents
- Created document categories: scenarios, conventions, platform-features, components, and platform implementations
- Established cross-reference patterns between feature docs and component docs

### Phase 2: Documentation Organization
- Created `README.md` as a navigation guide with role-based reading paths
- Organized documents by reader type (developers, operators, engineers, security reviewers)
- Added "Common Questions" section with pointers to relevant documents

### Phase 3: Separating Core Concepts
- Moved Goals, Non-Goals, Personas, and Design Principles from monolithic spec into individual documents:
  - `goals.md` — Goals and Non-Goals
  - `personas.md` — Three personas (Developer, Operator, Designer)
  - `principles.md` — Seven design principles
  - `vision.md` — Simplified to high-level description and security summary

- Updated `overall-design.md` to be a pure design reference (removed introductory material)
- Updated `README.md` to reflect new document structure

---

## Final Document Structure

```
docs/specs/
├── README.md                    Navigation guide and reading paths
├── vision.md                    What Morsel is
├── goals.md                     Goals and non-goals
├── personas.md                  Personas (developer, operator, designer)
├── principles.md                Design principles
├── architecture.md              System architecture
├── security-model.md            Threat model and security design
│
├── scenarios/
│   ├── developer.md             10 developer scenarios
│   └── operator.md              10 operator scenarios
│
├── conventions/                 (7 documents)
│   ├── resource-model.md
│   ├── naming.md
│   ├── identity-ownership.md
│   ├── async-operations.md
│   ├── idempotency.md
│   ├── permanence.md
│   └── error-model.md
│
├── platform-features/           (7 documents)
│   ├── hibernation.md
│   ├── deployment.md
│   ├── authentication.md
│   ├── cost-controls.md
│   ├── approvals.md
│   ├── networking.md
│   └── persistence.md
│
├── components/                  (6 documents)
│   ├── morsel-api.md
│   ├── bootstrap-cli.md
│   ├── blob-service.md
│   ├── queue-service.md
│   ├── database-service.md
│   └── admin-ui.md
│
└── platform/                    (3 documents)
    ├── interface.md
    ├── gcp.md
    └── local.md
```

**Total: 30 focused specification documents**

---

## Key Design Decisions

1. **Modular organization:** Documents grouped by purpose (features, components, conventions)
2. **Cross-referencing pattern:** Each feature doc includes "Component Contributions" sections; each component doc includes "Platform Feature Support" sections
3. **Role-based navigation:** README provides different reading paths for developers, operators, engineers, and security reviewers
4. **Separation of concerns:** 
   - Vision (what Morsel is)
   - Goals (what we're trying to achieve)
   - Principles (how we make decisions)
   - Architecture (how it's built)
   - Security (risk model and mitigations)
   - Scenarios (user journeys)
   - Features (user-visible capabilities)
   - Components (technical implementations)

---

## Files in This Archive

- **morsel-conversation-transcript.md** — This file; summary of the conversation and work completed
- **morsel-conversation-raw.jsonl** — Raw JSONL transcript of all exchanges (for detailed reference)

---

## Raw Transcript

The full conversation history is stored in `morsel-conversation-raw.jsonl` in JSONL format (one JSON object per line). Each line represents a message exchange and includes:

- Message type (user, assistant, or system)
- Message content (text, tool calls, or structured data)
- Metadata about tool use and responses

To parse the JSONL file:
```bash
# Extract just user messages
jq 'select(.type == "user") | .content' morsel-conversation-raw.jsonl

# Extract tool calls
jq 'select(.type == "assistant") | .content[] | select(.type == "tool_call")' morsel-conversation-raw.jsonl
```

---

## Key Patterns in the Specifications

### Platform Features + Components Pattern

Every platform feature (e.g., hibernation, cost controls) is documented at two levels:

1. **Feature document** (`platform-features/*.md`) describes user-visible behavior and rationale
2. **Component sections** describe how each component contributes to that feature

For example, hibernation:
- Start with `platform-features/hibernation.md` for the big picture
- Then read "Hibernation support" sections in `components/morsel-api.md`, `components/queue-service.md`, and `components/admin-ui.md`

### Cross-Cutting Conventions

Design patterns that apply across the platform:
- All resources belong to one app with fixed hostnames
- Async operations use 202 + Location + polling
- All operations are idempotent (upsert everywhere)
- Permanent resources require two-step removal
- Errors are always structured JSON

---

## Reading Recommendations

**New to Morsel?**
1. Start with `vision.md` (what it is)
2. Read `goals.md` and `principles.md` (why it exists)
3. Read `architecture.md` (how it's built)
4. Read `security-model.md` (safety constraints)

**Want to deploy apps?**
- `scenarios/developer.md` for concrete workflows
- `platform-features/deployment.md` for the deploy model

**Want to run the platform?**
- `scenarios/operator.md` for day-to-day operations
- `components/bootstrap-cli.md` for setup

**Building Morsel?**
- Complete "For Everyone" path above
- `conventions/` directory (7 design patterns)
- `platform-features/` directory (7 capabilities)
- `components/` directory (6 implementations)
- `platform/interface.md` for the Platform abstraction
- `platform/gcp.md` or `platform/local.md` for cloud specifics

---

## Notes for Future Maintenance

1. **overall-design.md is scheduled for removal** — All its content has been distributed to focused documents. It remains as a reference for comprehensive detail.

2. **Version consistency** — All spec documents have a header with Status and Date fields. Update these when making revisions.

3. **Cross-reference maintenance** — When adding new features or components, ensure:
   - Feature docs have a "Component Contributions" section
   - Component docs list which features they support

4. **README updates** — The README.md is the primary navigation document. Keep it current as the spec evolves.

---

**End of archive summary.**
