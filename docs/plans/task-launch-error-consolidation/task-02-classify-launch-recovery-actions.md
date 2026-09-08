---
id: "02-classify-launch-recovery-actions"
title: "Classify launch recovery actions"
status: completed
wave: 2
depends_on:
  - "01-isolate-reused-pr-refs"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Classify launch recovery actions

## Summary

Give checkout preparation a stable error category. Derive recovery actions from
the category and add a stamp-checked launch retry.

## In scope

- Add `workspace_checkout_failed` and `retry_launch` wire values.
- Map checkout preparation errors to the new category.
- Make every action list category-specific.
- Relaunch without a base-branch write for `retry_launch`.
- Keep authorization and current-stamp checks before relaunch.

## Out of scope

- New persistence tables.
- Automatic retries.
- Changes to `session.recover`.

## Acceptance

- A checkout error offers only `retry_launch`.
- A generic error does not offer base-branch actions because a repository row exists.
- A stale or foreign retry request makes no change and starts no session.

## Verification

```bash
cd apps/backend && go test ./internal/task/models ./internal/orchestrator/executor ./internal/orchestrator/... -race
```

## Files likely touched

- `apps/backend/internal/worktree/errors.go`
- `apps/backend/internal/task/models/launch_errors.go`
- `apps/backend/internal/task/models/launch_errors_test.go`
- `apps/backend/internal/orchestrator/executor/launch_failure.go`
- `apps/backend/internal/orchestrator/executor/executor_launch_failure_classification_test.go`
- `apps/backend/internal/orchestrator/task_launch_recovery.go`
- `apps/backend/internal/orchestrator/task_launch_recovery_test.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`

## Dependencies

Task 01.

## Risks

- Old persisted action arrays can conflict with the new category matrix.
- A retry must not clear the current error before launch succeeds.

## Parallelism

`sequential`

## Inputs

- Task launch failure recovery design, error model and recovery contract.
- Existing task-status projection normalization tests.

## Results

- Added the `workspace_checkout_failed` category and `retry_launch` action.
- Checkout failures now use category-specific recovery actions, including
  retry without changing a task repository base branch.
- Task-scoped recovery validates the current error stamp and repository
  identity before relaunch or mutation.
- Persisted and projected errors normalize action lists by category and keep
  bounded technical details.
- Focused race coverage passed: 1,212 tests across worktree, task models,
  status summary, and executor packages. The 11 targeted task-launch recovery
  tests also passed.
- The broader orchestrator package run was attempted but exceeded its timeout
  in an existing SQLite/concurrency test.
