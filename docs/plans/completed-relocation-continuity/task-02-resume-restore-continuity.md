---
id: "02-resume-restore-continuity"
title: "Prove resume and restore continuity"
status: done
wave: 2
depends_on:
  - "01-completed-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.3
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.6
  - AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 02: Prove resume and restore continuity

## Summary

Prove that existing session and executor paths use corrected completed admission.
Cover restart and retry from the previous false corruption error.
Keep the same task, session, current checkout, and provider conversation.

## In scope

- Add `TestCompletedRelocationExecutorContinuity` with real Git and SQLite inventory.
- Exercise `PreflightSessionWorktreeRecovery` and `ResumeSessionWithOptions`.
- Add `TestCompletedRelocationSessionContinuity` with the real admission manager.
- Exercise ordinary `ResumeTaskSessionWithOptions`, explicit
  `RecoverSessionWithOptions`, and `LaunchSession` with `restore_workspace`.
- Cover single and multi-repository tasks, FAILED retry, and reconstructed services.
- Record the launch request and verify its current paths, session, and resume token.
- Send correlated readiness events through the existing service test pattern.
- Verify current matching-error retirement and preservation of unrelated history.
- Verify restore performs no provider launch or conversation reset.
- Verify an invalid sibling or edited unfinished replacement prevents startup.
  A valid unfinished operation retains its existing guarded reconciliation path.
- Verify final lifecycle admission still rejects invalid current checkout identity.
- Change executor/service code only if a real integration regression requires it.
- After passing integration tests, update `docs/public/git-operations.md` with
  completed-relocation continuity and upgrade-and-retry guidance. This page is a
  recovery how-to. Do not recommend editing journals or discarding user work.
- Record actual task results and synchronize this plan's work-order statuses.

## Out of scope

- New controls, browser fixtures, production task mutation, or release publication.
- Replacing the admission manager with a stub or importing live checkout content.
- Broad refactors, SQL schema changes, generic verification, or automatic dirty repair.

## Acceptance

1. Resume uses the existing session and provider conversation with current work.
   Existing complete records need no migration, manual edit, or new action.
2. Restore returns the current workspace without agent startup.
   Matching success does not clear a newer or unrelated failure.
3. All selected slots validate through real manager admission.
   Negative cases launch no provider and preserve files, inventory, and tokens.

## Verification

Run from the repository root. For regression evidence, exercise the new tests
against the pre-fix source in a disposable checkout with isolated fixtures.
Do not reverse Task 01 in the shared workspace. Implement any exposed caller
defect with a failing targeted test first.

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor ./internal/orchestrator -run 'Test(CompletedRelocation|PermissionRecovery|RecoverSessionPermission|ResumeTaskSession_FailedKeepsResumeToken|LaunchRestoreWorkspace_)' -count=1)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestValidateLaunchWorkspaceAdmission' -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/completed-relocation-continuity
```

The lifecycle command covers the existing four `TestValidateLaunchWorkspaceAdmission` tests.
Run the plan's PR documentation coverage preflight with final changed paths.
Record all commands and outcomes without substituting historical package counts.

## Files likely touched

- New `apps/backend/internal/orchestrator/executor/executor_completed_relocation_integration_test.go`
- New `apps/backend/internal/orchestrator/session_completed_relocation_integration_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go` only for an exposed caller defect.
- `apps/backend/internal/orchestrator/session_launch.go` only for an exposed caller defect.
- `apps/backend/internal/orchestrator/task_operations.go` only for an exposed caller defect.
- `docs/public/git-operations.md`
- `docs/plans/completed-relocation-continuity/plan.md`
- This work order's results.

## Dependencies

[Task 01](task-01-completed-admission.md).

## Risks

- Mocked admission hides the defect and gives false integration evidence.
- Uncorrelated readiness events can bypass real resume settlement.
- Clearing a newer failure violates existing session recovery fencing.
- Two repositories must both validate before startup or restore success.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md), criteria above.
- [Design](../../specs/tasks/system-design/managed-clone-relocation.md#completed-relocation-continuity).
- Executor real-Git/SQLite patterns in `executor_legacy_clone_recovery_test.go`
  and `executor_permission_recovery_integration_test.go`.
- Service readiness/error pattern in `session_permission_recovery_integration_test.go`.
- Existing final lifecycle admission tests.

## Results

Implemented real-Git and SQLite-backed executor and service coverage through the
real worktree admission manager. The session tests cover ordinary resume,
explicit recovery, correlated readiness, matching-error retirement, newer
error preservation, a two-repository invalid sibling, and read-only workspace
restore. Provider startup remains unused for restore and invalid inventory.

The invalid-sibling test also exposed a caller defect: resume request setup could
reconcile a shared Git origin before selected worktree admission, hiding the
invalid sibling. Resume now admits the selected inventory before request setup
for a selected Worktree executor and repeats selected admission immediately
before provider launch. The second inspection catches checkout changes that
occur during request setup, even when the database snapshot stays unchanged.
Executor changes skip recovery of the abandoned Worktree, and inventory repair
performs its admission after the repair request completes.

Review follow-up fixed two resume-preflight regressions. Environment selection
now resolves on a copy of the session, so the early snapshot retains the
persisted empty binding for legacy task-owned environments; the guarded resume
boundary still persists the selected environment ID. A real SQLite/worktree
manager regression verifies normal resume keeps the provider token and current
checkout contents. Terminal-session stale execution cleanup now runs before
selected recovery admission can claim the environment. A SQLite-backed missing
checkout regression verifies cleanup permits recovery and that a live sibling
continues to block recovery without changing its row or checkout.

Updated `docs/public/git-operations.md` with normal post-relocation use and
upgrade-and-retry guidance. Added the manager, executor, service, and new test
paths to the Git documentation coverage area.

Verification:

- New regressions against a disposable worktree at pre-fix `7d55a59950` failed
  with the historical `published replacement commit could not be verified`
  error in manager, executor, resume, explicit-recovery, and restore paths.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor ./internal/orchestrator -run 'Test(CompletedRelocation|PermissionRecovery|RecoverSessionPermission|ResumeTaskSession_FailedKeepsResumeToken|LaunchRestoreWorkspace_)' -count=1)`: passed.
- The legacy empty-binding integration test failed before the fix with `selected worktree inventory changed while waiting for inspection`, then passed after preserving the persisted session value for the early snapshot.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor ./internal/orchestrator -run 'Test(CompletedRelocation|TerminalResumeCleansStaleExecution|PermissionRecovery|RecoverSessionPermission|ResumeTaskSession_FailedKeepsResumeToken|LaunchRestoreWorkspace_)' -count=1)`: passed after both review fixes.
- `(cd apps/backend && go build -trimpath ./...)`: passed after both review fixes.
- `(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestValidateLaunchWorkspaceAdmission' -count=1)`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 47 published docs pages validated.
- `python3 scripts/list-docs.py validate`: 356 decisions and 1,406 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Final-path PR documentation coverage preflight: passed with no errors.
- `(cd apps/backend && go build -trimpath ./...)`: passed.
- `git diff --check`: passed.

Review follow-up verification:

- `(cd apps/backend && go test ./internal/orchestrator/executor -run
  '^(TestResumeSession_PropagatesTaskEnvironmentID|TestResumeSessionWorkspaceBindingReadFailureDoesNotRollBackSuccessorAttempt|TestResumeSessionWorkspaceBindingWriteErrorDoesNotRollBackSuccessorAttempt|TestResumeSessionChangedEnvironmentPreventsWorkspaceBindingWrite|TestResumeSessionRechecksWorktreeAfterRequestBuild|TestResumeSessionSkipsOldWorktreeRecoveryAfterExecutorSwitch)$' -count=1)`:
  passed.
- `(cd apps/backend && go test ./internal/orchestrator/executor -run
  '^TestTerminalResumeCleansStaleExecutionBeforeRecoveryAdmission$' -count=1)`:
  passed.
- `(cd apps/backend && go test ./internal/orchestrator -run
  '^TestCompletedRelocationLegacyEmptyEnvironmentBindingResumes$' -count=1)`:
  passed.
- `make -C apps/backend build`: passed.
- `(cd apps/backend && go test ./internal/orchestrator/executor ./internal/orchestrator ./internal/worktree -count=1)`:
  executor and worktree passed; the orchestrator package had one failure in
  `TestResumeTurnStartGenuineFailureRetainsRecovery` during the combined run.
- `(cd apps/backend && go test ./internal/orchestrator -run '^TestResumeTurnStartGenuineFailureRetainsRecovery$' -count=1)`: passed in isolation.
- The next full orchestrator run exposed a stale count assertion in
  `TestRecoverSessionPermissionRetryRetiresMatchingError`; the added final
  pre-launch inspection makes the expected admission count four. The focused
  test passed after updating the assertion.
- `(cd apps/backend && go test ./internal/orchestrator -count=1)`: passed after
  updating the admission-count assertion.
- `git diff --check`: passed.

Post-PR recovery-ordering follow-up:

- A repeated manual-recovery E2E run exposed that a recognized provider
  continuation transport failure could overtake already queued ACP output and
  tool events, allowing an unsafe retry before the lifecycle had observed the
  turn activity. The adapter now emits that terminal failure after prior
  replay-sensitive activity on the same ordered stream. Failures before output
  keep the synchronous error path, and native continuation metadata remains
  gated by its existing feature and safety checks.
- `(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -count=1)`:
  passed.
- `(cd apps/backend && go test ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/orchestrator ./internal/worktree -count=1)`:
  passed.
- The grouped agentctl-process check hit one temporary-directory removal race
  in `TestWorkspaceTracker_StopsWhenWorkDirDeleted`; that test passed when run
  alone.
- The disabled-continuation manual-recovery E2E passed all ten iterations.
- `make -C apps/backend build`: passed.

PR merge and CI follow-up:

- After merging the current `main`, relocation integration tests now resolve
  private v2 relocation records through the SQLite recovery-artifact registry
  instead of assuming the journal is adjacent to the checkout.
- A failed-start regression showed that the final resume inspection returns a
  borrowed admission when preflight already owns the recovery claim. Resume
  now verifies the claim identity and transfers the owning admission through
  launch, preserving release failures in the returned error.
- `(cd apps/backend && go test -race -trimpath ./internal/orchestrator/executor -run
  '^(TestContinuationNativeStartupFailureRetainsAdmissionReleaseFailure|TestCompletedRelocationExecutorContinuity|TestEditedMaterializedRelocationRefusesExecutorStartup)$' -count=1)`: passed.
- `(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run
  '^TestCompletedRelocation' -count=1)`: passed.
- `(cd apps/backend && go test -race -trimpath ./internal/orchestrator/executor -count=1)`: passed.
- `(cd apps/backend && go test -race -trimpath ./internal/orchestrator -count=1)`: passed.
- `make -C apps/backend build`: passed.
- `(cd apps/backend && golangci-lint run ./... --new-from-rev=330e02a47808c11ca315ae30456fcce7f4806db5 --timeout=5m)`: passed with 0 issues.
