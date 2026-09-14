---
id: "06-e2e-quorum-required-count"
title: "E2E quorum case asserts exactly one decision is required"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/participant-seat-provenance-assurance.md"
---

# Task 06: E2E quorum case asserts exactly one decision is required

Implements `AC-OFFICE-SEAT-ASSURANCE-002.6`, pinning
`AC-OFFICE-SEAT-PROVENANCE-006.1` and `-006.2`.

## Acceptance

- `apps/web/e2e/tests/office/workflow-quorum-transitions.spec.ts` already walks
  the scenario the whole contract exists for: a step entry casts an `auto` seat,
  an operator then registers a *different* agent, and the slate must end at one
  seat. It asserts the guard's role and the participant listing. It does not
  assert how many decisions the guard requires — which is the operator-visible
  consequence.
- The count is already on the wire. Widen `getQuorumGuards`' local response type
  (currently `Array<{ role, satisfied, reason? }>`) to carry the required count,
  and assert it is exactly one — **alongside** the existing role assertion, not
  instead of it.
- A regression to two seats in that slate must fail this assertion. With two
  seats the role stays correct and the listing is arguably explicable; the gate
  simply never fires.
- Confirm the field name against the actual `/quorum` response rather than
  assuming it. Read the handler's response shape before naming it in the type.
- No new endpoint, fixture or spec file. No production frontend change.

## Constraints

- Do not reorder the case. The registration happens **after** the step move
  because seats bind to the task's current step; that ordering is the scenario,
  not an accident to tidy.
- Do not weaken the existing role or listing assertions.
- New user-facing copy is not involved here, so the i18n gates are not in play —
  but if any assertion string is added to a non-E2E file, `t()` rules apply.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/office/workflow-quorum-transitions.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
```

Demonstrate the assertion is decisive: confirm what the required count reads as
in the passing run, and state in the handoff how a two-seat slate would present.

## Files likely touched

- `apps/web/e2e/tests/office/workflow-quorum-transitions.spec.ts`
- this task file

## Inputs

- `docs/specs/office/requirements/participant-seat-provenance.md` — `-006.1`, `-006.2`
- `docs/specs/office/system-design/participant-seat-provenance-assurance-01.md`
  ("The guard requires exactly one decision")
- `/e2e`

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact E2E verification commands and results,
uncertainties, and this task status set to `done`. Do not edit `plan.md`.
