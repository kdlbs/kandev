---
id: "01-cascade-aware-switch"
title: "Make archive switching cascade-aware"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/archive-confirmation.md"
---

# Task 01: Make Archive Switching Cascade-Aware

Exclude every task selected by a cascade archive from the shared next-task
search. Defer Home navigation until archive success when no safe task exists.

## Acceptance

- A cascade archive never selects its parent or any descendant as the temporary
  or final task.
- The descendant walk is transitive, deduplicates cached rows, and terminates if
  malformed data contains a parent cycle.
- A non-cascade parent archive can still select an active child.
- A pre-request switch with no safe candidate leaves the active task and URL in
  place.
- A successful final removal with no safe candidate opens the task overview.
- A failed archive restores the original task after a safe pre-switch. It leaves
  the original task in place when no pre-switch occurred.
- Recent-task ordering, live-task validation, layout switching, and manual user
  navigation guards retain their current behavior.

## Files likely touched

- `apps/web/hooks/use-task-removal.ts`
- `apps/web/hooks/use-task-removal.test.ts`
- `apps/web/hooks/use-task-actions.ts`
- `apps/web/hooks/use-task-actions.test.ts`

## Dependencies

None.

## Parallelism

`sequential`. Task 02 depends on this behavior for its rendered regression.

## Inputs

- Spec sections **What**, **Failure modes**, and **Scenarios**.
- Plan sections **Cascade-aware candidate selection** and **Deterministic archive
  transition**.
- `apps/web/lib/ws/handlers/tasks.ts` as the existing successful archive
  redirect owner.
- `apps/web/lib/links.ts` for the existing URL replacement contract. Do not
  change it as part of this task.

## TDD sequence

1. Add failing selection tests for a recent child inside an excluded task tree,
   a deeper descendant, a malformed cycle, and a safe unrelated task.
2. Add a control test that permits a child when cascade is false.
3. Add a failing hook test that proves switch-only mode does not open Home when
   no safe candidate exists. Preserve the final-removal Home test.
4. Add failing action tests for the cascade option before and after the API
   request and for both archive failure paths.
5. Implement the smallest shared option and task-tree walk that pass the tests.
6. Refactor only if needed to keep candidate ordering and transition ownership
   explicit.

## Verification

```bash
(cd apps && pnpm --filter @kandev/web exec vitest run hooks/use-task-removal.test.ts hooks/use-task-actions.test.ts)
(cd apps/web && pnpm run typecheck)
```

## Risks

- Collecting descendants from only one projection can miss a cached row.
  Combine the available workflow and canonical Kanban projections before the
  walk.
- A broad filter would orphan active subtasks after a non-cascade archive. Apply
  tree exclusion only when the archive request has `cascade: true`.
- Do not replace the WebSocket route handler or add a second final navigation
  owner.

## Output contract

Report RED and GREEN evidence, exact files changed, cascade outcomes, and
non-cascade outcomes. Include failure behavior, typecheck, remaining risks, and
synchronized task and plan status. Record each command and outcome in
`## Results`.

## Results

- RED: `cd apps && pnpm --filter @kandev/web exec vitest run hooks/use-task-removal.test.ts hooks/use-task-actions.test.ts` failed with 4 expected regressions: cascade descendants were selected, switch-only mode navigated or selected a doomed child, and cascade exclusion was not forwarded.
- GREEN: the same focused Vitest command passed with 21 tests across both files.
- `cd apps/web && pnpm run typecheck` passed.
- The shared removal hook now walks all cached workflow and canonical Kanban task projections transitively with duplicate and cycle protection. Cascade callers exclude that tree before and after the archive request; non-cascade callers keep child selection; switch-only cleanup leaves the current route in place when no safe candidate exists.
- Changed files: `apps/web/hooks/use-task-removal.ts`, `apps/web/hooks/use-task-removal.test.ts`, `apps/web/hooks/use-task-actions.ts`, and `apps/web/hooks/use-task-actions.test.ts`.
- Archive failure coverage passes for both paths: a safe pre-switch restores the original task/session/URL, while a cascade with no pre-switch leaves the original task in place.
- Review remediation adds snapshot-first duplicate-row reconciliation and carries the pre-request exclusion set into post-archive cleanup so pruned descendants cannot become candidates. The focused suites now pass with 23 tests.
- Remaining risks are limited to the existing WebSocket/manual-navigation race guards and projection freshness; the backend archive contract and route ownership remain unchanged. Task 01 is done and the parent plan is complete.
