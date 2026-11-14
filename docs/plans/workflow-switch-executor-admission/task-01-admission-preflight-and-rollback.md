---
id: "01-admission-preflight-and-rollback"
title: "Admit the destination executor in preflight and roll back SCHEDULING"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.14
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.16
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
  - ../../specs/tasks/system-design/workflow-profile-readiness.md
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 01: Admit the destination executor in preflight and roll back SCHEDULING

## Summary

Make the shared step-entry preflight run executor admission for the
destination's bindings. A move whose destination is refused then fails
before commit. Separately, restore task state whenever a session switch
fails after it wrote `SCHEDULING`. Use TDD: write both regression tests
first and see them fail for the reasons in the plan's root cause.

## In scope

1. **Sentinel.** Add `ErrDestinationExecutorUnavailable` to
   `apps/backend/internal/workflow/move/errors.go`. The task service and the
   orchestrator already import that package.
2. **Executor preflight.** In `internal/orchestrator/executor/`, add an
   exported, side-effect-free
   `(*Executor).PreflightSessionExecutorAdmission(ctx, task *v1.Task, executorID, executorProfileID, taskEnvironmentID string) error`.
   It builds metadata exactly as `prepareSessionAttempt` does: it sets
   `executor_profile_id`, calls `resolveExecutorConfig`, then calls
   `prepareExecutorAdmission` on a transient `models.TaskSession` with
   `ExecutorProfileID` and `TaskEnvironmentID` set and no ID. Finally it calls
   `admitConcreteExecutorRuntime`. It never persists anything. Admission runs
   with an empty session ID, so `AdmitExecutorUse` writes nothing. Add it to
   the orchestrator's executor interface and test fakes.
3. **Orchestrator preflight.** In `preflightFixedWorkflowStepCredentials` and
   `preflightExplicitWorkflowStepCredentials`
   (`internal/orchestrator/event_handlers_workflow.go`), call the new method
   for `targetSession` before `PreflightManagedGitCredentials`. Pass the
   executor, executor profile, and environment of the reused session, or the
   source session's for a fresh destination. Wrap a refusal as
   `fmt.Errorf("%w: %w", workflowmove.ErrDestinationExecutorUnavailable, err)`.
   Lookup failures that are not refusals stay unwrapped and fail closed. Leave
   the keep-current early return unchanged.
4. **Rollback.** In `switchSessionForStepWithPoliciesAndCandidateAndRoute`,
   capture `dbTask.State` before the `SCHEDULING` write. Every error return
   after that write calls one helper that runs
   `taskRepo.UpdateTaskStateIfCurrentIn(ctx, taskID, prior, []v1.TaskState{v1.TaskStateScheduling})`
   and publishes the task update through the existing task-event path when a
   row changed. Never write an unconditional state.

## Out of scope

HTTP/WS error codes and frontend copy (WO-02), runner rebinding, deletion
guards, and `applyProfile` behavior.

## Acceptance

1. A manual move whose destination inherits a deleted executor profile returns
   an error wrapping `workflowmove.ErrDestinationExecutorUnavailable`. The step
   and primary session do not change, and no session row is created.
2. A switch that fails after `SCHEDULING` leaves the task in its prior state.
   A concurrent non-`SCHEDULING` state is preserved.
3. Existing preflight, profile-session policy, and rollback tests still pass,
   including automatic-transition recovery (001.14).

## Tests (write first)

- `internal/orchestrator/event_handlers_workflow_executor_admission_test.go`
  (a new file, because the existing preflight test file is near the lint
  limit):
  - `TestPreflightWorkflowStepMove_RejectsDeletedExecutorProfile`: profile
    `new`/`park` destination with a different agent profile, where admission
    returns `repoerrors.ErrExecutorProfileNotFound`. Asserts `errors.Is`
    matches the sentinel.
  - `TestPreflightWorkflowStepMove_KeepCurrentSkipsExecutorAdmission`: the same
    profile with `reuse` makes no admission call.
  - `TestSwitchSessionForStep_DeletedExecutorProfileRestoresTaskState`: the
    task starts at `REVIEW` with the source session `WAITING_FOR_INPUT`, and
    preparation fails. Asserts the task state is `REVIEW` and was not left at
    `SCHEDULING`. Before the fix this fails with `SCHEDULING`.
  - `TestSwitchSessionForStep_RollbackDoesNotOverwriteConcurrentState`.
- `internal/orchestrator/executor/`: a unit test that
  `PreflightSessionExecutorAdmission` passes an empty session ID and the
  inherited profile to the admission func, and creates no session.
- `internal/task/service/service_workflow_test.go` (or a new sibling file if
  that one is at the size limit): a rejecting `workflowMovePreflight` leaves
  the task's step and state unchanged.

## Commands

```bash
cd apps/backend
go test -trimpath ./internal/orchestrator/... ./internal/workflow/move/... -count=1
go test -trimpath ./internal/task/service/ -run 'Move|Workflow' -count=1
cd ../.. && bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Likely files

- `apps/backend/internal/workflow/move/errors.go`
- `apps/backend/internal/orchestrator/executor/executor_ownership.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/service.go` (executor interface)
- test fakes implementing that interface
- new test files listed above

## Results

- Added transient destination-executor admission before manual move commit and
  restored the prior task state with a SCHEDULING-only compare-and-set when a
  later session switch fails.
- `go test -trimpath ./internal/orchestrator/... ./internal/workflow/move/... -count=1` passed.
- `go test -trimpath ./internal/task/service/ -run 'Move|Workflow' -count=1` passed.
- `go test -trimpath ./internal/orchestrator/executor -run 'PreflightSessionExecutorAdmission' -count=1` passed after the final executor helper edit.
- Changed-file Go lint passed with `bash scripts/lint-go-changed` across the changed backend files.
- `make -C apps/backend build` passed.
- Review follow-up: the rollback regression now refuses through executor admission with `repoerrors.ErrExecutorProfileNotFound` inside `PrepareSessionForExistingEnvironment`; it asserts restoration to `REVIEW`, one task update, and no replacement session row. Explicit-target reuse of the current session also skips admission.
- `go test -trimpath ./internal/orchestrator/... ./internal/task/handlers/ -count=1` passed after the review fixes. Changed-file Go lint and `make -C apps/backend build` passed again.
