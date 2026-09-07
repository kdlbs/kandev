---
id: "01-preserve-latest-queued-workflow"
title: "Preserve latest queued workflow"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-WORKFLOW-MEMORY-001.1
system_design: []
---

# Task 01: Preserve Latest Queued Workflow

## Summary

Make the frontend task-create overlay retain workflow history for unrelated
workspaces while allowing the newest successful task creation to replace the
entry for its own workspace. Prove the correction with a focused failing test
before changing production code.

## In scope

- Add a same-workspace consecutive-submission regression test.
- Correct map precedence in `queueTaskCreateLastUsedFromPayload`.
- Preserve queued entries for other workspaces and latest scalar defaults.
- Run focused unit, type, and existing production-dialog E2E checks.

## Out of scope

- Backend or database changes.
- Workflow selector presentation or interaction changes.
- New preference fields or persistence sources.
- Changes to cancelled or failed task creation behavior.

## Acceptance

- Queuing workflow one and then workflow two for the same workspace leaves
  workflow two in `workflowIdsByWorkspace`.
- Consecutive submissions in different workspaces retain both workflow entries.
- Existing scalar last-used fields and settings-convergence behavior remain
  green.

## Verification

Run from `apps/web`:

```bash
pnpm test -- --run components/task-create-dialog-handlers.test.ts hooks/use-ensure-user-settings.test.ts components/state-provider.test.tsx
pnpm run typecheck
pnpm e2e:run tests/task/create-task.spec.ts -- --grep "uses the remembered workflow instead of the board filter" --retries=0
```

In a fresh worktree, first run `pnpm install --frozen-lockfile` from `apps`.

## Files likely touched

- `apps/web/components/task-create-dialog-handlers.ts`
- `apps/web/components/task-create-dialog-handlers.test.ts`
- `docs/plans/task-create-workflow-overlay-recency/plan.md`
- `docs/plans/task-create-workflow-overlay-recency/task-01-preserve-latest-queued-workflow.md`

## Dependencies

None.

## Risks

- Merge order must distinguish a newer value for the same workspace from older
  values that still need preservation for other workspaces.
- The overlay is module-scoped and ordering-sensitive, so the test must reset it
  before each scenario and must not depend on timing.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/task-create-workflow-memory.md`
- `docs/plans/task-create-workflow-memory/plan.md`
- `docs/decisions/0028-task-create-last-used-source-of-truth.md`
- `docs/decisions/0041-backend-owned-portable-user-settings.md`
- `docs/decisions/2026-08-08-workspace-scoped-task-create-workflow-memory.md`
- `apps/web/components/task-create-dialog-handlers.ts`
- `apps/web/components/task-create-dialog-handlers.test.ts`
- `apps/web/hooks/use-ensure-user-settings.ts`

## Results

The regression test failed before the production change. It expected workflow
two but received workflow one after two submissions in the same workspace.

The production change now uses prior workflow history as the merge base. The
latest successful payload replaces the entry for its workspace. Entries for
other workspaces remain unchanged.

Final results:

- `pnpm test -- --run components/task-create-dialog-handlers.test.ts hooks/use-ensure-user-settings.test.ts components/state-provider.test.tsx`
  passed 3 files and 37 tests.
- `pnpm run typecheck` passed.
- `pnpm e2e:run tests/task/create-task.spec.ts -- --grep "uses the remembered workflow instead of the board filter" --retries=0`
  passed 1 Chromium test.

The change uses shared state code. It does not change desktop or mobile
composition, navigation, scrolling, focus, safe areas, or touch targets.
