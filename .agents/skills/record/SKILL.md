---
name: record
description: Keep docs/decisions/ ADRs and docs/specs/ specs in sync with the work happening in the conversation. AUTO-INVOKE proactively the moment the user asks for any change that will alter architecture or observable product behavior — new feature, cross-cutting refactor, dependency swap, data-model or public-API change, new pattern, or a bug fix that changes documented behavior or reveals a spec gap. Also invoke on explicit triggers: "record this", "create an ADR", "document this decision", "update the spec", "ADR for X". Run BEFORE coding when the decision is upfront, or AFTER landing when the right call only became clear during implementation. SKIP for typo/lint fixes, refactors that preserve behavior within an existing pattern, and obvious uncontested choices.
---

# Record Knowledge

Record architectural decisions for future reference, and keep related feature specs in sync.

## Record a decision

When a significant architectural or design choice is made, create an ADR:

1. Read `docs/decisions/INDEX.md` to find the next number
2. Create `docs/decisions/NNNN-short-title.md` using the template below
3. Update `docs/decisions/INDEX.md` with the new entry
4. **Reconcile specs** — see "Update or create a spec" below

### ADR template

```markdown
# NNNN: Short Title

**Status:** accepted | superseded by NNNN | deprecated
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

- Choosing one approach over another (e.g., event bus vs direct calls)
- Adding a new dependency or library
- Changing a data model or API contract
- Selecting a pattern that affects multiple files (e.g., provider pattern for DI)
- Decisions that future developers will ask "why?" about

### What does NOT need an ADR

- Bug fixes, refactors within the same pattern, simple features
- Anything where the choice is obvious and uncontested

## Update or create a spec

ADRs capture *why* a decision was made. Specs capture *what* a feature does and why it exists. After recording an ADR, reconcile the affected spec — specs are the canonical product record kept in git, so they must stay accurate.

1. Read `docs/specs/INDEX.md` and identify any spec whose scope the decision touches (e.g., a routing decision affects `office-provider-routing/spec.md`).
2. For each affected spec:
   - **If the decision changes observable behavior, scope, or scenarios:** update `docs/specs/<slug>/spec.md` so the "What" and "Why" sections reflect the new direction. Add a `Decision: ADR-NNNN` reference where relevant.
   - **If the decision is purely internal (implementation choice with no spec-visible change):** no spec edit needed — the ADR alone is sufficient.
3. If the decision introduces a new product feature that has no spec yet, invoke `/spec` to create one rather than writing it ad-hoc here.
4. If no spec applies (pure infra/process decision, like this knowledge system itself), skip — note in the ADR that no spec is needed.

Do not duplicate ADR content inside the spec. Specs reference ADRs; they don't restate them.
