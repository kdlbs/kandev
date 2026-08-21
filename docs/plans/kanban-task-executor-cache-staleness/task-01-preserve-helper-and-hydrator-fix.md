---
id: "01-preserve-helper-and-hydrator-fix"
title: "Add executor-fields preserve helper and fix mergeKanbanTasks"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/kanban-task-executor-cache-staleness/spec.md"
---

# Task 01: Add executor-fields preserve helper and fix mergeKanbanTasks

Add a shared `preserveOmittedExecutorFields` helper and wire it into
`mergeKanbanTasks`'s wholesale-replace branch so a same/newer-timestamp merge
that omits the executor fields does not blank out an already-known executor
binding in the kanban cache.

## Acceptance

- `apps/web/lib/kanban/map-task.ts` exports
  `preserveOmittedExecutorFields(merged: KanbanTask, existing: KanbanTask):
  void` that copies `primaryExecutorId`, `primaryExecutorType`,
  `primaryExecutorName`, and `isRemoteExecutor` from `existing` onto `merged`
  only when `merged.primaryExecutorType === undefined`.
- `mergeKanbanTasks`'s `incomingTime >= existingTime` branch in
  `apps/web/lib/state/hydration/hydrator.ts` calls this helper before
  `draftTasks[idx] = incoming`, so a merge that omits the executor fields keeps
  the cached values instead of losing them.
- A merge whose incoming task legitimately carries a different
  `primaryExecutorType` still adopts the new value(s) — the helper is a
  gap-fill, not a sticky override.
- `toKanbanTask`'s `is_remote_executor ?? false` mapping and the pinned
  `"defaults isRemoteExecutor to false when missing"` test in
  `apps/web/lib/kanban/map-task.test.ts` are unchanged.

## Files likely touched

- `apps/web/lib/kanban/map-task.ts`
- `apps/web/lib/kanban/map-task.test.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/state/hydration/hydrator.test.ts`

## Dependencies

None.

## Parallelism

`sequential`. Task 02 imports the helper this task adds.

## Inputs

- Spec sections **Desired behavior**, **Regression scenarios**, and
  **Constraints**.
- `apps/web/lib/ws/handlers/tasks.ts:40-61` (`preservePrimaryExecutorFields`) as
  the reference behavior for the WebSocket path — do not modify this file; it
  already handles its own (raw-payload) input shape correctly.
- `apps/web/lib/state/hydration/hydrator.ts`'s existing
  `backfillServerDerivedFields` / `SERVER_DERIVED_TASK_FIELDS` as the existing
  in-file precedent for a gap-fill-only guard.

## TDD sequence

1. In `apps/web/lib/state/hydration/hydrator.test.ts`, add a failing test:
   hydrate a task with all four executor fields populated via `hydrateState`,
   then hydrate again with the same `id`, an equal `updatedAt`, and no executor
   fields on the incoming task payload; assert the resulting
   `draft.kanban.tasks` entry still has the original executor field values.
   Confirm it fails (the fields come back `undefined`/`false`) before touching
   production code.
2. Add a second test asserting that when the second hydration's incoming task
   carries a genuinely different `primaryExecutorType` (plus its sibling
   fields), the merged task adopts the new values.
3. Add direct unit tests for `preserveOmittedExecutorFields` in
   `apps/web/lib/kanban/map-task.test.ts`: incoming omits all four fields
   (preserve fires); incoming carries a real `primaryExecutorType` (preserve
   does not fire).
4. Implement `preserveOmittedExecutorFields` in `map-task.ts` and wire it into
   `mergeKanbanTasks` in `hydrator.ts`. Keep the change minimal — no changes to
   `toKanbanTask` itself.
5. Re-run the tests from step 1-3 and confirm they pass.

## Verification

```bash
cd apps/web
pnpm exec vitest run lib/state/hydration/hydrator.test.ts lib/state/hydration/hydrator-kanban-tasks.test.ts lib/kanban/map-task.test.ts
pnpm run typecheck
```

## Risks

- Gating on `primaryExecutorType` rather than `isRemoteExecutor` itself is
  intentional (see plan **Risks**); do not "simplify" this to a per-field
  `undefined` loop, since that silently fails to preserve `isRemoteExecutor`
  (it is never `undefined` post-mapping).
- Do not extend the preserve to distinguish an explicit executor clear from an
  omitted field — that disambiguation is out of scope (see spec **Constraints**
  and **Out of scope**).

## Output contract

Report the RED failure, GREEN result, exact files changed, and any deviation
from this task file. Record every exact command and outcome in `## Results`.

## Results

Added `preserveOmittedExecutorFields` (exported from `apps/web/lib/kanban/map-task.ts`,
gated on `primaryExecutorType`'s own `undefined`-ness per the plan's rationale)
and wired it into `mergeKanbanTasks`'s wholesale-replace branch in
`apps/web/lib/state/hydration/hydrator.ts`, right before `draftTasks[idx] =
incoming`.

- RED: `pnpm exec vitest run lib/kanban/map-task.test.ts` — 2 new tests failed
  with `TypeError: preserveOmittedExecutorFields is not a function`. Then
  `pnpm exec vitest run lib/state/hydration/hydrator.test.ts` — the
  same-timestamp-omits-executor-fields test failed, asserting the merged task
  lost its executor fields (matched the bug exactly); the
  legitimately-different-value test passed even before the fix, as expected.
- GREEN: same two commands after implementing the helper and wiring it in —
  all tests passed.
- Deviation from the task file: the new `hydrator.test.ts` tests were moved
  into a new sibling file `apps/web/lib/state/hydration/hydrator-kanban-tasks.test.ts`
  instead of staying inline, because adding them to `hydrator.test.ts` pushed
  it from 649 to 663 lines, over the repo's 600-line file limit (already
  pre-existing at 649, worsened by the addition). Similarly, the new
  `map-task.test.ts` `describe("preserveOmittedExecutorFields", ...)` block
  was made a top-level sibling describe instead of nesting inside `"toKanbanTask
  — state normalization"`, which had pushed that arrow function past the
  100-line function limit.
- Full verification: `pnpm exec vitest run lib/kanban/map-task.test.ts
  lib/state/hydration/hydrator.test.ts lib/state/hydration/hydrator-kanban-tasks.test.ts`
  — 48 passed. `pnpm exec eslint lib/kanban/map-task.ts lib/kanban/map-task.test.ts
  lib/state/hydration/hydrator.ts lib/state/hydration/hydrator.test.ts
  lib/state/hydration/hydrator-kanban-tasks.test.ts` — clean, no warnings.
  `pnpm run typecheck` — clean.

Files changed: `apps/web/lib/kanban/map-task.ts`, `apps/web/lib/kanban/map-task.test.ts`,
`apps/web/lib/state/hydration/hydrator.ts`, `apps/web/lib/state/hydration/hydrator.test.ts`
(tests moved out), `apps/web/lib/state/hydration/hydrator-kanban-tasks.test.ts` (new).
