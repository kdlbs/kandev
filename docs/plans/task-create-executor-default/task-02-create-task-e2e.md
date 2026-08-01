---
id: "02-create-task-e2e"
title: "Prove portable settings behavior in Create Task"
status: done
wave: 2
depends_on: ["01-executor-first-selection"]
plan: "plan.md"
spec: "../../specs/tasks/task-create-executor-default.md"
---

# Task 02: Prove portable settings behavior in Create Task

## Intent

Exercise the real backend-settings-to-boot-state-to-dialog path so the isolation-first default is
proven across the same boundary used by another browser or an upgraded frontend.

## Acceptance

- With a backend-saved Local profile and no workspace executor default, the visible Create Task
  selector shows Worktree for the seeded repository.
- With an explicit Local workspace default, the visible selector shows Local even when the saved
  profile is Worktree.
- The test restores both workspace and task-create preference state so later worker-scoped tests do
  not inherit its executor choice.

## TDD sequence

1. Add the backend-saved Local-profile UI regression and run the focused Playwright grep to confirm
   RED against the current selection behavior if Task 01 is temporarily reverted or before its
   production change.
2. Add the explicit workspace-default scenario and deterministic cleanup.
3. Run both scenarios against the production build through the managed E2E runner.

## Files likely touched

- `apps/web/e2e/tests/task/create-task-executor-default.spec.ts`

## Dependencies

- Task 01 must be complete.

## Parallelism

`sequential` — it validates Task 01 through shared task-create fixtures and production assets.

## Inputs

- Spec scenarios for portable Local last-used state and explicit workspace defaults.
- Existing `ApiClient.saveUserSettings`, `ApiClient.updateWorkspace`, `ApiClient.listExecutors`,
  `KanbanPage`, and `executor-profile-selector` patterns.
- The fixture warning that partial task-create settings writes preserve omitted fields.

## Verification

- `cd apps/web && pnpm e2e:run --project chromium tests/task/create-task-executor-default.spec.ts`

## Mobile parity

No mobile-only E2E is added because the change modifies shared selection state inside the existing
responsive dialog and does not alter composition, navigation, scrolling, touch targets, or pointer
behavior. Existing mobile Create Task specs continue to cover reachability and layout.

## Risks

- Cleanup must restore `default_executor_id` and a non-empty Worktree
  `task_create_last_used.executor_profile_id`; omitting either can leak through the worker-scoped
  backend.
- Assertions should target the selected profile text, not option ordering.

## Output contract

The focused managed Chromium run passed both scenarios (`2 passed`); each test restores the workspace
default and backend task-create preference in `finally` cleanup, and no blockers remain.
