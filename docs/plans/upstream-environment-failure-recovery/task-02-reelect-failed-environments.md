---
id: "02-reelect-failed-environments"
title: "Re-elect failed unmaterialized environments and point the rest at reset"
status: done
wave: 2
depends_on:
  - "01-release-abandoned-creating-claims"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.8
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.10
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 02: Re-elect failed unmaterialized environments and point the rest at reset

## Summary

Let a start re-materialize a `failed` environment that never created anything
physical, and make the refusal of a `failed` environment that holds a workspace
name the reset action.

## In scope

- Write failing repository, executor, and MCP handler tests first.
- Use `reelectTaskEnvironmentMaterializationTx` for `failed` rows without owner
  and physical evidence in the task-local and shared-group bindings.
- Add `ReelectFailedTaskEnvironmentMaterialization` with the cleanup barrier
  and recovery-claim admission, and call it from the launch's `failed` branch
  for the session's own task. On a lost race, wait for a `creating` winner or
  adopt a `ready` or `stopped` row, and fail closed otherwise. Keep
  `inherit_parent` fail-closed.
- Add `models.ErrTaskEnvironmentResetRequired`, wrapping
  `ErrWorkspaceReuseUnsafe`, for `failed` rows with physical evidence in the
  bindings and the launch, and map it to reset guidance in
  `spawn_session_kandev` with the unchanged reason.

## Out of scope

- Retaining and replacing a `failed` environment with physical evidence.
- Changes to the election-loser wait and retry.
- Rendered UI.

## Acceptance

- A binding or the launch of a bound session against a `failed` row without
  owner and physical evidence becomes the owner and prepares the workspace;
  shared groups behave the same; concurrent re-elections produce one owner.
- A `failed` row with a path, container, sandbox, or live repository row, and an
  inherited environment, still refuse with `workspace_reuse_unsafe`; the
  evidence case names the reset action in the error and in the
  `spawn_session_kandev` retry guidance.
- No request path re-elects without a new binding, start, or launch request.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'Workspace|Environment|Binding|SharedGroup|Materializ' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'Launch|Environment|Materializ|Workspace' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/mcp/handlers -run 'SpawnSession' -count=1)
(cd apps/backend && go vet -trimpath ./internal/orchestrator/... ./internal/task/... ./internal/mcp/handlers)
(cd apps/backend && go build -trimpath ./...)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/internal/task/models/workspace_binding.go
- apps/backend/internal/task/repository/sqlite/session.go
- apps/backend/internal/task/repository/sqlite/task_environment.go
- apps/backend/internal/task/repository/sqlite/session_workspace_binding_test.go
- apps/backend/internal/orchestrator/executor/executor.go
- apps/backend/internal/orchestrator/executor/executor_execute.go
- apps/backend/internal/orchestrator/executor/executor_mocks_test.go
- apps/backend/internal/orchestrator/executor/executor_environment_test.go
- apps/backend/internal/orchestrator/service.go
- apps/backend/internal/mcp/handlers/spawn_session.go
- apps/backend/internal/mcp/handlers/spawn_session_test.go

## Dependencies

- Task 01 provides the conditional failure write and the re-election helper.

## Risks

- Clearing container and sandbox handles must not drop a reference that a row
  with physical evidence still needs; such rows are never updated.
- Shared-group siblings observe `workspace_preparing` again while a re-elected
  owner prepares the group environment.

## Parallelism

`sequential`

## Inputs

- Issue #4322.
- REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004 and its system design section.
- Work order 01 results.

## Results

These are historical contributor results. Current-head review remediation and
hosted acceptance requirements are recorded in [the plan](plan.md#review-remediation-validation).

- RED on the code after work order 01:
  - `TestWorkspaceBindingReelectsFailedEnvironmentWithoutEvidence` and
    `TestWorkspaceBindingConcurrentFailedReelectionHasOneOwner` failed with
    `existing task environment is not attachable`, and
    `TestSharedGroupBindingReelectsFailedGroupEnvironment` with
    `shared workspace is not attachable`.
  - `TestLaunchPreparedSessionRematerializesOwnFailedEnvironment` failed with
    `existing task environment is not attachable`.
  - `TestHandleSpawnSession_FailedEnvironmentWithResourcesPointsToReset` got
    the retry guidance `restore the task workspace, then retry`.
  - The tests that use `models.ErrTaskEnvironmentResetRequired` or
    `ReelectFailedTaskEnvironmentMaterialization` did not compile, because
    neither existed.
- GREEN: all of these pass, together with the guards:
  - a failed row with a worktree or workspace path stays unchanged;
  - an inherited environment is refused without reset guidance;
  - a lost launch race adopts or waits for the winner;
  - re-election refuses during task cleanup and while a recovery claim is
    held.
- Related packages:
  - The `sqlite` tests matching
    `Workspace|Environment|Binding|SharedGroup|Materializ|TakesOver|TakeOver|Claim|Reelect`
    pass, except `TestUpdateEnvironmentWritesMutableFieldsAndReportsMissing`.
  - The `executor` tests matching
    `Launch|Environment|Materializ|Workspace|PersistTask|PrepareSession|Reelect`
    pass, except the same 22 Windows failures as in work order 01.
  - The `orchestrator` tests matching
    `StartTask|QuickChat|LaunchFailure|Environment` and every
    `TestHandleSpawnSession` test pass.
- `go vet` on the changed packages and `golangci-lint` with
  `--new-from-rev` on them report nothing. `go build -trimpath ./...` passes.
