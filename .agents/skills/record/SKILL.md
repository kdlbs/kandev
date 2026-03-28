---
name: record
description: Record an architectural decision (ADR) or save an implementation plan. Use after making significant design choices or completing features.
---

# Record Knowledge

Record architectural decisions and implementation plans for future reference.

## Record a decision

When a significant architectural or design choice is made, create an ADR:

1. Read `docs/decisions/INDEX.md` to find the next number
2. Create `docs/decisions/NNNN-short-title.md` using the template below
3. Update `docs/decisions/INDEX.md` with the new entry

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

## Save a plan

After implementing a feature, save the design as a permanent record:

1. Read `docs/plans/INDEX.md` to check for existing plan
2. Create `docs/plans/YYYY-MM-feature-name.md` using the template below
3. Update `docs/plans/INDEX.md` with the new entry

### Plan template

```markdown
# Feature/Plan Title

**Date:** YYYY-MM-DD
**Status:** proposed | approved | implemented | abandoned
**PR:** #NNN
**Decision:** ADR-NNNN (if applicable)

## Problem
What problem this solves.

## Design
File paths, interfaces, data flow. Mermaid diagrams where useful.

## Implementation Notes
Post-implementation: what changed from the plan, gotchas, things the next person should know.
```
