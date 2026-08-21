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

Wire request-start executor freshness into `useAllWorkflowSnapshots`'s
per-task snapshot merge. A stale workflow response must not blank out or
restore an executor binding changed by a live event in `kanbanMulti.snapshots`.

## Acceptance

- In `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`'s
  `fetchAndWriteSnapshot`, compare the current task with its request-start
  task before applying the HTTP response.
- Copy all four current executor fields when the task changed after request
  start or first appeared during the request.
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

1. Add a failing deferred-response test where a live detach clears the cached
   executor and the stale response returns the old explicit executor bundle.
2. Add a failing deferred-response test where a task first appears during the
   request with live executor data and the stale response omits that data.
3. Add a complete-copy helper and wire it into the snapshot merge.
4. Re-run the tests from steps 1-2 and confirm they pass.

## Verification

```bash
cd apps/web
pnpm exec vitest run hooks/domains/kanban/use-all-workflow-snapshots.test.ts hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts
pnpm run typecheck
```

## Risks

- `mapSnapshotTask` returns `null` for tasks outside `stepIds`; make sure the
  new test's task is on a step included in the mocked snapshot's `steps` so it
  survives the existing filter before the preserve logic is reached.

## Output contract

Report the RED failure, GREEN result, exact files changed, and any deviation
from this task file. Record every exact command and outcome in `## Results`.

## Results

The merge now uses request-start freshness for the complete executor bundle.
When the live cache changes during the fetch, or when a task appears while the
fetch is in flight, the current bundle wins. When it does not change, the
snapshot remains authoritative and can clear the executor.

- RED: `pnpm exec vitest run
  hooks/domains/kanban/use-all-workflow-snapshots.test.ts` — both new race
  regressions failed. A stale snapshot restored the old executor after a live
  detach, and a task added during the fetch lost its live executor.
- GREEN: the same command after the fix — all 20 tests passed.
- Focused verification: `pnpm exec vitest run
  lib/kanban/map-task.test.ts lib/state/hydration/hydrator.test.ts
  lib/state/hydration/hydrator-kanban-tasks.test.ts
  hooks/domains/kanban/use-all-workflow-snapshots.test.ts
  hooks/domains/kanban/use-all-workflow-snapshots-inflight.test.ts` — 80
  tests passed.
- `pnpm exec eslint` passed for all touched TypeScript files. `pnpm run
  typecheck` passed.
- Updated the spec and implementation plan to document the request-start
  freshness rule and the separate hydration gap-fill rule.

Files changed: `apps/web/hooks/domains/kanban/use-all-workflow-snapshots.ts`,
`apps/web/hooks/domains/kanban/use-all-workflow-snapshots.test.ts`,
`apps/web/lib/kanban/map-task.ts`, and the related spec and plan files.
