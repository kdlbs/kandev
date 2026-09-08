---
name: spec
description: Create or update Kandev product requirements and system-design documents before implementation. Use for new product behavior, changed contracts, or explicit specification work. Do not use for implementation plans, work orders, incidents, or behavior-preserving refactors.
---

# Specification Authoring

Use this skill to create or update durable specifications. Requirements define
observable behavior. System designs define the technical path that satisfies
requirements.

The canonical rules are in `docs/specs/guide/`. Read these files before you
write an artifact:

- Always read `structure-and-ownership.md`.
- Read `requirements.md` for requirement work.
- Read `system-design.md` for system-design work.
- Read `traceability-and-lifecycle.md` for IDs, statuses, references, or
  migration work.

Use the templates in `docs/specs/templates/`.

## Artifact routing

Route the request before you write:

| Request | Artifact |
| --- | --- |
| Kandev-wide purpose, actors, principles, measures, or constraints | `docs/specs/product/` |
| Observable behavior for one owning system | `<system>/requirements/` |
| Technical contracts, models, boundaries, or control flow | `<system>/system-design/` |
| Durable choice with meaningful alternatives | `/record` and an ADR |
| Delivery sequence and implementation tasks | `/plan` |
| Incident or behavior-preserving refactor | No product requirement |
| Bug | `/fix`, which checks the existing requirement first |

Do not create a generic `spec.md` file.

## Workflow

### 1. Locate the owning system

Read `docs/specs/README.md` and the likely system `README.md`. If the system has
not migrated, use `docs/specs/INDEX.md` to locate the legacy source.

Search all system indexes, requirements, and designs for the capability name
and its main nouns. Update an existing capability when it owns the same actor,
lifecycle, and contract.

Choose the system that owns the source of truth and durable contract. Do not
choose an owner from the code directories that change. Record one sentence in
the working notes that states why the selected system owns the capability.

User visibility does not make a capability UI-owned. Keep provider state, task
state, permissions, persistence, and recovery with their owning systems. Put
desktop, mobile, accessibility, and visible failure outcomes in that owner's
requirement. Create a UI requirement only for an independent and reusable
presentation contract.

The same system owns the requirement and its design. Other systems link to that
source. They do not copy it or claim its requirement IDs in design frontmatter.

If no system owns the behavior, define the new system boundary before you write
requirements. A new system needs a `README.md` based on the system template.

### 2. Confirm intent

Use `/interview-me` when a missing product choice changes behavior, ownership,
permissions, persistence, or a public contract. Do not hide an unresolved
choice in a draft.

### 3. Write requirements

Create or update:

```text
docs/specs/<system>/requirements/<capability>.md
```

Each requirement document must contain:

- Valid frontmatter.
- One or more stable `REQ-*` IDs.
- At least one `AC-*` acceptance criterion for each requirement.
- Observable behavior and explicit exclusions.

Use user stories only when they clarify a natural actor and outcome. Do not put
files, functions, database queries, or implementation sequences in a
requirement.

Keep one cohesive vertical outcome together. Do not create separate backend and
UI requirements for the same feature. Split only when actors, lifecycles, or
contracts are independent.

### 4. Write system design

Create or update this file when the change needs a technical design:

```text
docs/specs/<system>/system-design/<capability>.md
```

The design must list the applicable `REQ-*` IDs in frontmatter. It can use an
explicit empty list for internal infrastructure with no independent product
requirement.

Describe stable components, models, contracts, flow, failure behavior,
persistence, security, and observability when they apply. Link to global ADRs.
Do not copy requirement or ADR text.

Cover all runtime boundaries that implement the owned outcome. A provider-owned
design can include backend services, storage, projections, frontend components,
responsive behavior, and tests. Do not create a parallel UI design for those
same requirements.

### 5. Update the system index

Add the new documents to the system `README.md`. State the system boundary and
link adjacent systems when ownership can be confused.

During migration, name the new source as authoritative. Replace the old source
with a link or archive it. Do not leave two editable sources of truth.

### 6. Validate

Review the artifacts before you run the linter:

- One system owns each requirement and its design.
- No adjacent system contains a copied requirement or UI-only duplicate.
- Requirements contain observable behavior, not storage, control flow, or file
  details.
- Every acceptance criterion states a testable behavior. No criterion delegates
  its meaning to migrated source detail.
- Designs map requirement IDs without copying requirement text.
- New files do not copy the legacy `Migrated source detail` wrapper.
- New artifacts appear in the owning system index.

Run:

```bash
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions
```

If a file reaches its size limit, split it by capability, lifecycle, or contract
boundary. Do not add a size exception for a new document.

## Design-package behavior

When this skill runs inside `/spec-driven-development` or `/fix`, continue to
the system design, plan, and work orders. Stop after requirements only when the
user explicitly requests a requirements review or a material question blocks
safe design.

For a standalone specification request, report the changed paths, requirement
IDs, design references, validation results, and open questions. Then return
control to the user.
