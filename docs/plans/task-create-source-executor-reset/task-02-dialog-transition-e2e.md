---
id: "02-dialog-transition-e2e"
title: "Prove the real dialog transition"
status: done
wave: 2
depends_on: ["01-reset-source-executor"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-create-executor-default.md"
---

# Task 02: Prove the Real Dialog Transition

## Intent

Exercise the exact user-reported source-tab sequence through the production-built Create Task
dialog and prove that its visible executor follows the active source.

## Acceptance

- Repo mode initially shows Worktree when the workspace has no executor default.
- None mode shows a direct Local profile, and returning to Repo shows Worktree again.
- The test restores shared workspace and task-create preference state even when an assertion fails.

## TDD sequence

1. Add the Repo to None to Repo scenario to the existing executor-default Playwright suite.
2. Temporarily revert Task 01, then verify RED at the final Worktree assertion.
3. Restore Task 01 and run the focused managed Chromium spec against rebuilt production assets.
4. Verify GREEN. Reconcile the cleanup and recorded result with the task and plan.

## Files likely touched

- `apps/web/e2e/tests/task/create-task-executor-default.spec.ts`

## Dependencies

- Task 01 must be complete.

## Parallelism

`sequential`. It validates the shared source-transition behavior implemented by Task 01.

## Inputs

- Spec scenario for Repo to repository-less mode to Repo.
- Existing `executorProfiles`, `saveTaskCreatePreference`, `openCreateTask`, source-mode test IDs,
  and executor-profile selector patterns in the target spec.
- E2E fixture rule that shared workspace and user settings must be restored in `finally` cleanup.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-executor-default.spec.ts
```

## Mobile parity

No mobile-only E2E is added. This repair changes shared state normalization inside the existing
responsive dialog and does not change layout, navigation, scrolling, focus, touch targets, or
pointer behavior. Existing mobile Create Task specs remain the interaction and layout coverage.

## Risks

- Wait for the visible selector text after each source change. Do not assert transient setter order.
- Restore `default_executor_id` and a Worktree task-create preference in cleanup so the worker-scoped
  backend cannot leak Local into neighboring tests.

## Output contract

Report the focused Playwright result, build evidence, changed files, cleanup evidence, blockers,
and risks. Then update this task and `plan.md` in the same conversation.

## Results

RED:

- With the Task 01 production change temporarily reverted,
  `cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-executor-default.spec.ts`
  ran 3 tests and failed the new transition scenario. The selector stayed at `LocalLocal` after
  returning to Repo.

GREEN:

- `cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-executor-default.spec.ts`
  rebuilt the backend and Vite production assets, then passed all 3 tests.

Changed files:

- `apps/web/e2e/tests/task/create-task-executor-default.spec.ts`

Cleanup and side effects: the managed runner used isolated temporary E2E data and cleaned it up.
No mobile-only test was added because the repair changes shared selection state without changing
responsive composition or touch behavior.
