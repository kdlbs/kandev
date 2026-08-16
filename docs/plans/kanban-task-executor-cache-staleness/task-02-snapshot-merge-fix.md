---
id: "02-snapshot-merge-fix"
title: "Preserve executor fields in the workflow snapshot merge"
status: done
wave: 1
depends_on: ["01-preserve-helper-and-hydrator-fix"]
plan: "plan.md"
spec: "../../specs/kanban-task-executor-cache-staleness/spec.md"
---

# Task 02: Preserve executor fields in the workflow snapshot merge

Wire the `preserveOmittedExecutorFields` helper from task 01 into
`useAllWorkflowSnapshots`'s per-task snapshot merge, so a fresh workflow
snapshot response that omits the executor fields does not blank out an
already-known executor binding in `kanbanMulti.snapshots`.

## Acceptance

- In `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`'s
  `fetchAndWriteSnapshot`, the `if (existing)` block that already preserves
  `primarySessionId`/`primarySessionState`/`autopilot`/`statusSummary` also
  calls `preserveOmittedExecutorFields(mapped, existing)`.
- A fresh snapshot response whose task omits all four executor fields keeps the
  cached task's executor field values in the merged snapshot.
- A fresh snapshot response whose task carries a genuinely different
  `primaryExecutorType` (plus its sibling fields) still adopts the new values.

## Files likely touched

- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`
- `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`

## Dependencies

Task 01 (`preserveOmittedExecutorFields` must exist in `map-task.ts` first).

## Parallelism

`sequential`.

## Inputs

- Spec sections **Desired behavior** and **Regression scenarios**.
- The existing `"preserves a cached autopilot marker when a fresh snapshot
  omits it"` and `"keeps an explicit false autopilot value from the fresh
  snapshot"` tests in `use-all-workflow-snapshots.test.ts` as the pattern to
  follow (mock `fetchWorkflowSnapshot`, seed `kanbanMulti.snapshots`, assert on
  `mockSetWorkflowSnapshot`'s recorded call).

## TDD sequence

1. Add a failing test to `use-all-workflow-snapshots.test.ts`: seed
   `kanbanMulti.snapshots` with a task carrying all four executor fields, mock
   `fetchWorkflowSnapshot` to return the same task `id` with none of the
   executor fields, run the hook's fetch, and assert
   `mockSetWorkflowSnapshot`'s recorded task still has the original executor
   field values. Confirm it fails first.
2. Add a second test asserting a fresh response carrying a genuinely different
   `primary_executor_type` (plus its sibling fields) still wins.
3. Wire `preserveOmittedExecutorFields(mapped, existing)` into the `if
   (existing)` block in `fetchAndWriteSnapshot`.
4. Re-run the tests from step 1-2 and confirm they pass.

## Verification

```bash
cd apps/web && pnpm exec vitest run hooks/domains/kanban/use-all-workflow-snapshots.test.ts
cd apps/web && pnpm run typecheck
```

## Risks

- `mapSnapshotTask` returns `null` for tasks outside `stepIds`; make sure the
  new test's task is on a step included in the mocked snapshot's `steps` so it
  survives the existing filter before the preserve logic is reached.

## Output contract

Report the RED failure, GREEN result, exact files changed, and any deviation
from this task file. Record every exact command and outcome in `## Results`.

## Results

Wired `preserveOmittedExecutorFields` (from task 01) into the `if (existing)`
block in `fetchAndWriteSnapshot`, right after the existing `statusSummary`
preserve, in `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`.

- RED: `pnpm exec vitest run hooks/domains/kanban/use-all-workflow-snapshots.test.ts`
  — the new "preserves cached executor fields..." test failed, asserting the
  merged snapshot task lost its executor fields (`undefined` instead of the
  cached values); the "adopts a legitimately different executor value..." test
  passed even before the fix, as expected.
- GREEN: same command after wiring in the helper — all 16 tests passed.
- Deviation from the task file: the two new tests were split into their own
  top-level `describe("useAllWorkflowSnapshots — executor field
  preservation", ...)` block (with a small `seedCachedExecutor()` helper)
  instead of living inside the existing `"snapshot mapping"` describe, because
  adding them there pushed that describe's arrow function to 139 lines, over
  the repo's 100-line function limit. The file's total line count (466) stayed
  well under the 600-line file limit.
- Full verification: `pnpm exec vitest run
  hooks/domains/kanban/use-all-workflow-snapshots.test.ts
  hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts
  lib/kanban/map-task.test.ts lib/state/hydration/hydrator.test.ts
  lib/state/hydration/hydrator-kanban-tasks.test.ts` — 67 passed.
  `pnpm exec eslint hooks/domains/kanban/use-all-workflow-snapshots.ts
  hooks/domains/kanban/use-all-workflow-snapshots.test.ts` — clean, no
  warnings. `pnpm run typecheck` — clean.

Files changed: `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`,
`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`.

