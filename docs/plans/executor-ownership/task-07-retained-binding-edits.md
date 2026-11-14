---
id: "07-retained-binding-edits"
title: "Allow edits past unresolvable retained executor bindings"
status: in_progress
wave: 2
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-OWNERSHIP-005
acceptance_criteria:
  - AC-EXECUTORS-OWNERSHIP-005.4
system_design:
  - ../../specs/executors/system-design/per-user-ownership.md
---

# Task 07: Allow edits past unresolvable retained executor bindings

Fixes the step-edit 404 in
[#5172](https://github.com/kdlbs/kandev/issues/5172). Task edits and workflow
step edits re-check the executor bindings retained on a task: its
`executor_profile_id` and `executor_id` metadata, its active sessions, and
their task environments. A binding whose profile was deleted, whose executor
was retired, or whose environment row is gone failed that check with a
not-found error. Owners and collaborators could no longer edit the task, and a
step PUT or DELETE was reported as `404 Step not found` for a step that GET
still returned.

## Behavior

- A retained binding that admission refuses for every caller confers no
  compute, so it no longer blocks the edit. A missing profile falls back to its
  stored executor before that decision.
- Every retained binding that still resolves keeps the strict ownership check.
  A collaborator is still refused when the task names an owner's live private
  profile, executor, active-session executor, or environment executor.
- `executor_profile_id` and `executor_id` metadata values supplied at task
  creation or newly written by an update are always checked. Resubmitting the
  unchanged stale values during an update is allowed.
- Repository read errors fail closed and are not reported as a denial.
- `AuthorizeWorkflowExecutorMutation` wraps executor eligibility refusals in
  `models.ErrExecutorMutationDenied`. Step handlers and workflow HTTP updates
  report `403 executor mutation denied`. Workflow WebSocket updates report
  `FORBIDDEN` with the same message. The response names no executor or profile.
- A missing workspace default executor has a separate sanitized conflict
  response with the `workspace_default_executor_unavailable` error code.
- Task updates wrap executor eligibility refusals in the same denial marker.
  HTTP updates report 403 and WebSocket updates report `FORBIDDEN`.
- Retained metadata, session, and environment rows are not rewritten, and no
  deleted or retired resource is revived.

## Unchanged behavior and known limits

- Launch, resume, and prompt admission are unchanged. They re-check the
  resolved executor independently, so a skipped retained binding cannot start
  compute.
- Workspace visibility and roles are unchanged: non-members and other
  workspaces still see not-found, and viewers are still refused.
- The workflow creator check (`PrepareAutonomousExecutor`) still blocks step
  edits when a configured workspace default executor no longer resolves. The
  response now identifies the workspace configuration problem without naming
  the executor.
- Independent QA observed that a collaborator's HTTP step PUT on this path
  returns 500. That response predates this change and is not addressed here.
- A profile ID recreated with the same value is treated as live again and is
  checked strictly.
- The 403 body does not identify the blocking dependency.
- Prepared-profile precedence, retained-environment migration, and session
  stop behavior discussed on the issue are separate follow-ups.

## Scope

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/executor_mutation.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/executor_mutation_retained_test.go`
- `apps/backend/internal/task/service/executor_mutation_creation_test.go`
- `apps/backend/internal/task/handlers/errors.go`
- `apps/backend/internal/task/handlers/workflow_handlers.go`
- `apps/backend/internal/task/handlers/workflow_executor_mutation_test.go`
- `apps/backend/internal/workflow/handlers/handlers.go`
- `apps/backend/internal/workflow/handlers/executor_mutation_denial_test.go`
- The ownership system design paragraph on retained bindings.

## Validation

The service tests use an owned workspace with an owner and a collaborator.
The task's metadata and active `WAITING_FOR_INPUT` session reference a deleted
profile, a retired private executor, and a missing environment.

- `TestRetainedExecutorBindingsDoNotBlockContentEdits`
- `TestRetainedExecutorLiveBindingsStillDenyCollaborator`
- `TestRetainedExecutorSelectionChangesAreChecked`
- `TestRetainedExecutorEditKeepsWorkspaceAuthorization`
- `TestRetainedExecutorEndedSessionsAreNotConsidered`
- `TestRetainedExecutorStorageErrorsFailClosed`
- `TestRetainedExecutorCanceledEditFailsClosed`
- `TestRetainedExecutorEditsRacingProfileDeletion`
- `TestStepMutationExecutorDenialIsForbidden` and
  `TestStepMutationExecutorGuardAllowsEdit` use a stub guard in the step
  handler.

```bash
cd apps/backend
go test -trimpath -race -count=1 ./internal/task/service/ ./internal/workflow/... \
  -run 'TestRetainedExecutor|TestStepMutationExecutor|TestExecutorOwnership|ExecutorMutation'
```

Results on the contributor branch:

- The command above passed. The race and cancellation tests also passed with
  `-count=20`.
- With the production files reverted to the base, the new tests fail. Content
  edits fail with `executor profile not found`, and step denials return 404.
- Changed-file Go lint reported zero issues.
- The full backend suite failed identically with and without the change in the
  contributor environment, because host Git worktree removal was refused there.
  That run is not counted as passing.

There is no end-to-end HTTP test that combines stale bindings with a step edit.
Nothing was validated against a live deployment.

## Maintainer review corrections (2026-11-08)

- Task creation now uses the same selection check as task updates. The check
  runs after the external-ID lookup and before task allocation or persistence.
  New regression cases cover foreign and missing metadata selections, plus
  successful owner and shared-profile selections.
- Workflow HTTP and WebSocket updates report the existing denial marker
  through their forbidden response channels. Workflow edits with a missing
  workspace default return a separate sanitized conflict code.
- Task updates wrap executor eligibility refusals in the denial marker, and
  their HTTP and WebSocket handlers report sanitized forbidden responses.
- The first focused follow-up test run found a missing test import. After the
  import was added, the same command passed in all three packages.
- The full backend suite remains unrun for this follow-up.
- `bash scripts/lint-go-changed` passed for the ten changed Go files (0 issues).
- `node --test scripts/validate-public-docs.test.mjs` passed all 63 tests.
- `node scripts/validate-public-docs.mjs` validated 55 published pages.
- `python3 scripts/list-docs.py validate` validated 437 decisions and 1937
  specifications.
- `python3 scripts/lint-spec-files.py --all` passed.

Focused maintainer follow-up command:

```bash
cd apps/backend
go test -trimpath -race -count=1 ./internal/task/service/ ./internal/task/handlers/ ./internal/workflow/handlers/ \
  -run 'TestRetainedExecutor|TestExecutorMetadataSelection|TestHTTPWorkflowExecutorMutation|TestWSWorkflowExecutorMutation|TestHTTPWorkflowMissingDefaultExecutorConflict|TestWSWorkflowMissingDefaultExecutorConflict|TestStepMutationExecutor|TestStepMutationMissingWorkspaceDefaultHasConflictCode|TestHTTPUpdateTaskExecutorMutationDenialIsForbidden|TestWSUpdateTaskExecutorMutationDenialIsForbidden'
```

Targeted checks for a later validation step:

```bash
cd apps/backend
go test -trimpath -race -count=1 ./internal/task/service/ ./internal/task/handlers/ ./internal/workflow/... \
  -run 'TestRetainedExecutor|TestExecutorMetadataSelection|TestHTTPWorkflowExecutorMutation|TestWSWorkflowExecutorMutation|TestStepMutationExecutor|TestExecutorOwnership|ExecutorMutation'
```
