---
id: "01-release-abandoned-creating-claims"
title: "Release and take over abandoned creating claims"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.6
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.9
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 01: Release and take over abandoned creating claims

## Summary

Make a launch that gives up release the `creating` claim it owns, and let a
session binding take over a claim whose owner is gone, without touching live or
young owners.

## In scope

- Write failing repository, executor, and orchestrator tests first.
- Add `FailTaskEnvironmentMaterialization(sessionID)`, a
  `creating` to `failed` write that succeeds only for the current owner, and
  route `markTaskEnvironmentMaterializationFailed` and the inline `LaunchAgent`
  error branch through it.
- Split `LaunchPreparedSession` into a wrapper that releases the session's claim
  after any error except `ErrExecutionAlreadyRunning`.
- Release the claim of a session the start created in the `startTask`
  early-return helper, unless the error is `ErrCeilingLaunchDeferred`.
- Extend `failAbandonedWorkspaceMaterialization` with the launch-budget rule and
  add `reelectTaskEnvironmentMaterializationTx` for abandoned claims without
  physical evidence, in the task-local and shared-group bindings.
- Refuse readiness publication in `persistTaskEnvironment` when the launch
  started as the initial materializer and the fresh row is no longer its claim.

## Out of scope

- Re-election of `failed` rows and reset guidance (work order 02).
- Session state changes on release, and any periodic sweep or setting.

## Acceptance

- An early `LaunchPreparedSession` error and a `startTask` early return release
  the claim; a ceiling deferral, a reused session, and
  `ErrExecutionAlreadyRunning` leave it untouched.
- A binding against a `creating` row whose owner is missing, terminal, or stale
  past the budget without a live owner becomes the new owner of the same row; a
  live or young owner still yields `workspace_preparing` with no write; a
  deleting task or held recovery claim still refuses.
- Concurrent take-overs produce one owner, and a former owner can neither write
  `failed` nor publish readiness over the new owner.

## Verification

From the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run 'Workspace|Environment|Binding|SharedGroup|Materializ' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator/executor -run 'Launch|Environment|Materializ|Workspace' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'StartTask|QuickChat|LaunchFailure|Environment' -count=1)
git diff --check
```

## Files likely touched

- apps/backend/internal/task/repository/sqlite/session.go
- apps/backend/internal/task/repository/sqlite/task_environment.go
- apps/backend/internal/task/repository/sqlite/session_workspace_binding_test.go
- apps/backend/internal/orchestrator/executor/executor.go
- apps/backend/internal/orchestrator/executor/executor_execute.go
- apps/backend/internal/orchestrator/executor/executor_resume.go
- apps/backend/internal/orchestrator/executor/executor_mocks_test.go
- apps/backend/internal/orchestrator/executor/executor_environment_test.go
- apps/backend/internal/orchestrator/quick_chat_launch_failure.go
- apps/backend/internal/orchestrator/service.go
- apps/backend/internal/orchestrator/task_operations_test.go

## Dependencies

None.

## Risks

- The budget rule reads `constants.SetupScriptTimeout` and
  `constants.AgentLaunchTimeout`; tests must set and restore them.
- `startTask` callers that relied on the `CREATED` session keeping its claim
  after a returned error now see a `failed` environment until work order 02
  lets the next start re-elect it.

## Parallelism

`sequential`

## Inputs

- Issue #4322.
- REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004 and its system design section.

## Results

These are historical contributor results. Current-head review remediation and
hosted acceptance requirements are recorded in [the plan](plan.md#review-remediation-validation).

- RED on the unmodified code:
  - The three binding tests that pinned the old outcome, now
    `TestCreateTaskSessionWithWorkspaceBindingTakesOver{AbandonedCreatingClaim,OwnerlessCreatingClaim,TerminalCreatingOwner}`,
    and `TestWorkspaceBindingConcurrentTakeOverHasOneOwner` failed with
    `workspace reuse is unsafe: the initial workspace materialization did not complete`.
  - `TestWorkspaceBindingTakesOverStaleClaimOfSilentOwner` failed with
    `workspace is preparing`, and `TestSharedGroupBindingTakesOverAbandonedGroupClaim`
    with the shared-workspace `workspace is preparing` refusal.
  - `TestFailTaskEnvironmentMaterializationChangesOnlyTheCurrentOwnersClaim`
    did not compile: the repository had no owner-only failure write.
  - `TestLaunchPreparedSessionReleasesOwnedClaimOnEarlyError` left the claim
    `creating`.
  - `TestLaunchFailureLeavesClaimTakenOverDuringLaunch` showed the former
    owner overwriting the new owner's claim with `failed`.
  - `TestPersistTaskEnvironmentRefusesPublicationAfterClaimTakenOver`
    published the row.
  - `TestStartReleasesEnvironmentClaimOfItsSessionOnEarlyReturn` and the
    generic-error case of `TestReleaseStartEnvironmentClaimKeepsDeferredAndReusedClaims`
    left the claim `creating`.
- GREEN: all of these pass. Their guards pass before and after the change:
  young and live owners, an abandoned claim with a worktree row, a held
  recovery claim, `ErrExecutionAlreadyRunning`, a cancelled superseding resume,
  a ceiling deferral, and a reused session.
- Related packages:
  - The `sqlite` tests matching
    `Workspace|Environment|Binding|SharedGroup|Materializ|TakesOver|TakeOver|Claim`
    pass, except `TestUpdateEnvironmentWritesMutableFieldsAndReportsMissing`.
  - The `orchestrator` tests matching
    `StartTask|QuickChat|LaunchFailure|Environment` pass.
  - The `executor` tests matching
    `Launch|Environment|Materializ|Workspace|PersistTask|PrepareSession` pass,
    except 22 Kubernetes resume and workspace inventory repair or attestation
    tests.
  - The excluded tests fail on Windows with and without this change; the same
    22 executor names fail on the unmodified code.
- `go vet` on `./internal/orchestrator/...` and
  `./internal/task/repository/sqlite` reports nothing.
