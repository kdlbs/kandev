---
id: "03-create-task-e2e"
title: "Prove workflow memory through Create Task"
status: done
wave: 3
depends_on: ["01-backend-workflow-memory", "02-frontend-workflow-resolution"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-workflow-memory.md"
---

# Task 03: Prove Workflow Memory Through Create Task

## Intent

Exercise the real successful-create → backend settings → Go boot payload →
frontend dialog path with conflicting filters and two workspaces.

## Acceptance

- A workspace's remembered workflow is shown in standard Create Task even when
  the board/list filter points at another workflow.
- After two workspaces record different workflows, reloading and opening Create
  Task in each restores its own workflow.
- The existing one-visible-workflow scenario still renders no selector, and the
  test restores worker-scoped settings or uses disposable workspaces so no
  state leaks to neighbouring specs.

## TDD sequence

1. Add the conflicting-filter and two-workspace UI assertions using successful
   HTTP task creation as setup; run the focused grep against the production
   build and confirm RED because workflow history is not yet restored.
2. Complete Tasks 01 and 02, rerun the same scenarios, and confirm GREEN without
   retries.
3. Run the full task-create spec to keep existing single-workflow and selection
   persistence coverage green, recording managed-runner cleanup evidence.

## Files likely touched

- `apps/web/e2e/tests/task/create-task.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if the existing task/workspace seed
  helpers cannot express the two-workspace setup

## Dependencies

- Task 01 records and serves per-workspace history.
- Task 02 maps and resolves that history in the dialog.

## Parallelism

`sequential` — this is the end-to-end proof for both preceding tasks.

## Inputs

- Spec scenarios for conflicting filters, workspace switching, and sole visible
  workflow suppression.
- Existing `KanbanPage`, `ApiClient.createTask`, `ApiClient.createWorkflow`,
  `saveUserSettings`, and task-create dialog test IDs.
- Existing worker-scoped settings cleanup guidance in the E2E skill.

## Verification

- `cd apps/web && pnpm e2e:run --project chromium tests/task/create-task.spec.ts -- --grep "remembers the last workflow per workspace|selects the single visible workflow"`
- `cd apps/web && pnpm e2e:run --project chromium tests/task/create-task.spec.ts`

## Mobile parity

No mobile-only E2E is added because this task proves portable shared-state
resolution without changing the mobile composition or interaction. Existing
mobile Kanban tests continue to cover opening Create Task from a selected
workflow and touch reachability.

## Risks

- `workflow_filter_id` and task-create workflow history are both worker-scoped
  persisted settings; cleanup must restore the baseline even after assertion
  failure.
- A test that asserts only option order would miss the bug; assert selected
  trigger text in the visible dialog after reload.

## Output contract

Report RED/GREEN managed-runner commands, discovered test counts, final pass
counts, artifact paths, cleanup/teardown evidence, blockers, risks, and
synchronize this task plus `plan.md` status/results in the primary conversation.

## Results

Added production-build E2E coverage that creates real tasks through the HTTP
API, verifies the backend recorded Dev and Support under separate workspace
keys, then opens the standard dialog through the UI with conflicting filters
and reloads each workspace. The existing one-visible-workflow selector
omission remains covered.

Verification:

- `cd apps/web && pnpm e2e:run tests/task/create-task.spec.ts -- --grep "remembered workflow"` — 2 passed after a managed backend/Vite build.
- `cd apps/web && pnpm e2e:run --no-build tests/task/create-task.spec.ts -- --grep "single visible workflow|remembered workflow"` — 3 passed.
- `cd apps/web && pnpm e2e:run --no-build tests/task/create-task.spec.ts` — all 15 tests passed in 39 seconds.

The managed runner isolated and cleaned its temporary worker backends and
repositories; the test deletes its disposable workspace/workflows and restores
the seed workspace filter. No failure artifacts were produced.
