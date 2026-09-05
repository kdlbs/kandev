---
created: 2026-09-03
status: done
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation Plan: Pathless Worktree Materialization Failure

## Overview

This change makes a failed worktree materialization terminal and visible.
First, the repository stores the failed environment state without a worktree path.
Then, the frontend waits for an explicit Retry action before it sends another request.

## Scope

### In scope

- Permit a worktree environment to enter `failed` before it has a worktree path.
- Keep the path requirement for reusable `ready` and `stopped` environments.
- Keep a failed `session.ensure` request latched until the user selects Retry.
- Preserve the current task-page and preview error surfaces on desktop and mobile.

### Out of scope

- Automatic environment reset after a materialization error.
- Changes to Git refresh policy, timeout values, or error classification.
- Changes to error-card layout, copy, or recovery controls.
- A database migration or a new environment state.

## Technical approach

### Environment state persistence

Update `UpdateTaskEnvironment` in
`apps/backend/internal/task/repository/sqlite/task_environment.go`.
The validation will permit an empty `workspace_path` only for `creating` and `failed`.
The validation will continue to reject other pathless worktree states.

Add a repository regression test in
`apps/backend/internal/task/repository/sqlite/task_environment_test.go`.
The test will store `creating → failed` with an empty path and reload the row.

### Explicit frontend retry

Update `useEnsureTaskSession` in
`apps/web/hooks/domains/session/use-ensure-task-session.ts`.
The error branch will retain the current request key.
Only `retry()` or a task change will permit another ensure request.

Add a hook regression test in
`apps/web/hooks/domains/session/use-ensure-task-session.test.ts`.
The test will change the session-loader identity after an error.
The request count must remain one until the test calls `retry()`.

## Tests

- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2`: the repository test proves that the terminal environment state persists.
- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8`: the hook test proves that an error remains stable until explicit recovery.
- Existing component tests cover the task-page banner, preview empty state, and Retry control.

## E2E tests

The change does not alter layout, touch behavior, scrolling, or navigation.
Existing desktop coverage remains in `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`.
Existing mobile coverage remains in `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`.

The hook regression test covers the changed request-scheduling behavior directly.
No new mobile Playwright scenario is necessary for this state-only change.

## Work orders

- [x] [Task 01: Persist a Pathless Failed Environment](task-01-persist-pathless-failed-environment.md) (`done`)
- [x] [Task 02: Latch Failed Session Ensure Requests](task-02-latch-failed-session-ensure.md) (`done`)

## Verification results

- The targeted SQLite command passed all five matching tests.
- The targeted frontend command passed all 22 hook tests.

## Risks

- A broad validation change can permit an attachable state without a path.
- A stale frontend latch can block an explicit retry or a new task request.
