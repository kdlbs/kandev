---
name: record
description: 'Keep durable architecture decisions, requirements, and system designs in sync. Auto-invoke when a request establishes or changes a long-lived boundary, public contract, ownership rule, operational invariant, or repo-wide convention with meaningful alternatives. Also use for explicit ADR or specification-recording requests.'
---

# Record Knowledge

Record significant decisions for future work. Reconcile the affected
requirements and system designs after the decision.

## Record a decision

Create an ADR when all statements are true:

- The choice creates a durable constraint, boundary, contract, ownership rule,
  operational invariant, or repository-wide convention.
- Meaningful alternatives have different trade-offs.
- Future work must follow or supersede the choice.
- A requirement, system design, test, or work order does not preserve enough
  rationale.

Do not create an ADR for a simple feature, local refactor, routine dependency
change, plan sequence, or temporary migration step.

## ADR procedure

1. Choose an ID in the form `YYYY-MM-DD-short-title`.
2. Make sure that `docs/decisions/<id>.md` does not exist.
3. Create the ADR with the template below.
4. Add it to `docs/decisions/INDEX.md`.
5. Reconcile affected specifications.

Existing numeric ADR IDs remain valid. Do not rename them.

```markdown
# ADR-YYYY-MM-DD-short-title: Short title

**Status:** accepted | superseded by <adr-id> | deprecated
**Date:** YYYY-MM-DD
**Area:** backend | frontend | infra | protocol | workflow

## Context

Explain the situation and the need for a decision.

## Decision

State the selected rule, boundary, or contract.

## Consequences

State benefits, costs, and follow-up constraints.

## Alternatives Considered

State each meaningful alternative and why it was not selected.
```

## Reconcile specifications

Read `docs/specs/README.md`, the owning system index, and the relevant files in
`docs/specs/guide/`.

Apply these rules:

- Confirm the owning system from the durable contract. Do not assign ownership
  to UI only because users observe the decision there.
- If the decision changes observable behavior, update the owning requirement
  and its `REQ-*` or `AC-*` criteria.
- If the decision changes technical boundaries or contracts, update the owning
  system design.
- If both change, update both artifacts and preserve their references.
- If the decision is internal and does not change a documented design, the ADR
  is sufficient.
- If no product system applies, state that fact in the ADR.

Do not copy the ADR into requirements or system design. Link the ADR from the
affected design. Requirements contain observable outcomes, not decision
rationale.

During legacy migration, use `docs/specs/INDEX.md` to locate existing sources.
Do not create a new generic `spec.md` file.

## Validation

Run:

```bash
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/decisions docs/specs
```

Report the ADR, affected requirement IDs, affected system designs, and the
validation result.
