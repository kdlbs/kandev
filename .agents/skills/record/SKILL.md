---
name: record
description: 'Keep durable architecture decisions and product specs in sync with the work happening in the conversation. AUTO-INVOKE when a request establishes or changes a long-lived architectural boundary, public contract, data ownership rule, operational invariant, or repo-wide convention with meaningful alternatives. Also invoke on explicit triggers: "record this", "create an ADR", "document this decision", "update the spec", "ADR for X". Run BEFORE coding when the decision is upfront, or AFTER landing when the right call only became clear during implementation. Do not create ADRs for simple features, local implementation choices, routine dependency changes, or bug fixes that do not establish a durable rule.'
---

# Record Knowledge

Record architectural decisions for future reference, and keep related feature specs in sync.

## Record a decision

When a significant architectural or design choice is made, create an ADR:

1. Choose a decentralized ID in the form `YYYY-MM-DD-short-title`. The short title must be
   specific enough to remain unique among decisions created on the same date.
2. Confirm that `docs/decisions/<id>.md` does not already exist.
3. Create `docs/decisions/<id>.md` using the template below.
4. Update `docs/decisions/INDEX.md` with the new entry.
5. **Reconcile specs** — see "Update or create a spec" below.

Existing numeric ADR IDs remain valid and must not be renamed. References use the complete stable
ID, for example `ADR-2026-07-16-project-shell-output`.

### ADR template

```markdown
# ADR-YYYY-MM-DD-short-title: Short Title

**Status:** accepted | superseded by <adr-id> | deprecated
**Date:** YYYY-MM-DD
**Area:** backend | frontend | infra | protocol | workflow

## Context
What situation prompted this decision. 2-5 sentences.

## Decision
What was decided. Reference file paths, packages, interfaces.

## Consequences
Trade-offs. What becomes easier or harder.

## Alternatives Considered
What else was considered and why it was rejected.
```

### What warrants an ADR

Create an ADR only when all of these are true:

- The choice establishes a durable constraint, boundary, contract, ownership rule, operational
  invariant, or repo-wide convention.
- There were meaningful alternatives with materially different trade-offs.
- Future work will need to follow or deliberately supersede the choice.
- A spec, plan, code comment, or regression test alone would not preserve enough of the reasoning.

Typical examples include selecting a system-wide communication model, defining ownership across
subsystems, changing a public API or persisted-data contract, and adopting a cross-cutting security
or reliability invariant.

### What does NOT need an ADR

- Simple features whose behavior belongs in a product spec
- Local implementation tactics and refactors within an existing pattern
- Routine dependency additions or upgrades
- Bug fixes unless they establish a new rule that future implementations must follow
- Plan sequencing, task breakdown, and temporary migration mechanics
- Anything obvious, uncontested, or easily reversible without cross-system consequences

## Update or create a spec

ADRs capture *why* a decision was made. Specs capture *what* a feature does and why it exists. After recording an ADR, reconcile the affected spec — specs are the canonical product record kept in git, so they must stay accurate.

1. Read `docs/specs/INDEX.md` and identify any spec whose scope the decision touches (e.g., a routing decision affects `office-provider-routing/spec.md`).
2. For each affected spec:
   - **If the decision changes observable behavior, scope, or scenarios:** update `docs/specs/<slug>/spec.md` so the "What" and "Why" sections reflect the new direction. Add a `Decision: ADR-<id>` reference where relevant.
   - **If the decision is purely internal (implementation choice with no spec-visible change):** no spec edit needed — the ADR alone is sufficient.
3. If the decision introduces a new product feature that has no spec yet, invoke `/spec` to create one rather than writing it ad-hoc here.
4. If no spec applies (pure infra/process decision, like this knowledge system itself), skip — note in the ADR that no spec is needed.

Do not duplicate ADR content inside the spec. Specs reference ADRs; they don't restate them.
