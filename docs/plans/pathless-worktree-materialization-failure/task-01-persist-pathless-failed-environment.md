---
id: "01-persist-pathless-failed-environment"
title: "Persist a Pathless Failed Environment"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Persist a Pathless Failed Environment

## Summary

The repository must store a failed worktree environment when materialization creates no path.
Reusable environment states must keep the non-empty path requirement.

## In scope

- Add a failing repository regression test for `creating → failed` with an empty path.
- Update the worktree path validation for the `failed` state.
- Reload the row and make sure that the failed state and empty claim persist.

## Out of scope

- Changes to worktree creation or Git refresh behavior.
- Automatic removal of the failed environment.
- Changes to ready-state inventory validation.

## Acceptance

- `UpdateTaskEnvironment` stores a pathless worktree environment in `failed`.
- The stored materialization-session claim is empty.
- Pathless reusable worktree states remain invalid.

## Verification

Run this command from `apps/backend`:

```bash
go test -tags fts5 ./internal/task/repository/sqlite -run 'TestUpdateTaskEnvironment' -count=1
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/task_environment.go`
- `apps/backend/internal/task/repository/sqlite/task_environment_test.go`

## Dependencies

None.

## Risks

- The validation must not permit `ready` or `stopped` without a path.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2`
- `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8`
- `docs/specs/tasks/system-design/task-launch-failure-recovery.md`
- Issue `https://github.com/kdlbs/kandev/issues/3335`

## Results

- RED: the targeted command failed because `UpdateTaskEnvironment` rejected the pathless `failed` state.
- GREEN: the targeted command passed all five matching repository tests.
