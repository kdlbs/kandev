---
id: "02-retain-lanes-with-hidden-steps"
title: "Retain lanes whose live hidden set is non-empty"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/board-step-visibility-filter.md"
---

# Task 02: Retain lanes whose live hidden set is non-empty

## Acceptance

- In All-Workflows view, a workflow with zero filtered tasks is **kept** on the
  board when `hiddenWorkflowStepIds[workflowId] ∩ liveStepIds` is non-empty, so
  its header and Columns menu stay reachable.
- A workflow with zero filtered tasks and an **empty** live hidden set is dropped
  exactly as before this feature; a hidden set containing only stale ids does not
  retain a lane.
- `getVisibleWorkflows` stays a pure helper — it receives the hidden information
  as an argument or prepared predicate and does not read the store.

## Verification

```
cd apps/web && pnpm vitest run components/kanban/swimlane-container.test.ts && pnpm run typecheck
```

The three retention cases must fail first against the current
`getVisibleWorkflows`, then pass.

## Files likely touched

- `apps/web/components/kanban/swimlane-container.tsx` (`getVisibleWorkflows`
  and its call site around line 506)
- `apps/web/components/kanban/swimlane-container.test.ts`

## Dependencies

None in code. Ship in the same PR as Task 01: without this rule, hiding every
step of a workflow removes the lane and with it the only control that can undo
the hide.

## Risks

- This changes existing board behaviour for one case. Two comments in
  `apps/web/e2e/tests/kanban/step-visibility-filter.spec.ts` (~line 45 and ~line
  266) describe the old drop behaviour and become wrong; Task 04 updates them —
  note the conflict rather than editing that spec here.
- `getVisibleWorkflows` is on the render path; keep the predicate O(1) per
  workflow and memoised at the call site.

## Inputs

- Spec sections `Lane reachability (the recoverability rule)`, the two `(R2)`
  All-Workflows scenarios, and the stale-id boundary
- Plan section `Lane retention`
- The existing live-set intersection in `WorkflowItemContent`'s `hiddenSet` memo
  — reuse its semantics so "live" means the same thing in both places

## Output contract

Report the RED failures, the final signature of `getVisibleWorkflows`, files
changed, exact targeted test result, and the stale E2E comments handed to Task
04. Mark this task `done` and tick its plan checkbox.
