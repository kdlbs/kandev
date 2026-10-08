---
id: "01-resume-admission"
title: "Coordinate outer resume inspection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-METADATA-RECOVERY-003
acceptance_criteria:
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.6
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.7
  - AC-TASKS-WORKTREE-METADATA-RECOVERY-003.9
system_design:
  - ../../specs/tasks/system-design/worktree-metadata-recovery.md
---

# Task 01: Coordinate outer resume inspection

## Summary

Coordinate outer resume admission with workspace inspection through one bounded
budget. A safe inspection deferral retains task/session state and the existing
conflict response. Lifecycle creation remains nonblocking.

## In scope

- Carry one 15-second maximum inspection deadline across outer preflight and
  subsequent resume admission. Respect earlier cancellation and deadlines.
- Preserve immediate default refusal for background and lifecycle callers.
- Reuse complete selection snapshots and current-attempt guards after waiting.
- Return typed safe deferral only after successful owned state/credential rollback.
- Exclude that deferral from generic resume failure bookkeeping.
- Keep the existing conflict kind and all actual failure classifications.

## Out of scope

Browser behavior, locale catalogs, historical task repair, schema changes,
provider changes, and new worktree mutation authority.

## Acceptance

1. A real workspace inspection can overlap session-open resume. After release,
   resume uses the same inventory/conversation and starts at most one agent.
2. Exhaustion retains workflow state and error history. Late admission restores
   only its owned STARTING state and credential snapshot before safe deferral.
3. Cancellation, archive, selection drift, rollback failure, and genuine metadata
   or startup failure retain their guarded outcomes. Lifecycle admission never waits.

## Regression first

Add `TestSessionOpenResumeWaitsForWorkspaceInspection` through the production
resume entry path with a real worktree manager, real Git, and persisted inventory.
Use channel barriers to hold inspection. Prove the current implementation returns
contention and falsely fails the task before changing production code.

Add `TestResumeInspectionContentionPreservesTaskState` for budget exhaustion.
Capture task REVIEW, session WAITING_FOR_INPUT, original failure metadata, branch,
provider token, and message history before the attempt. Assert the same durable
values afterward and no provider prompt/start.

Add `TestResumeInspectionBudgetSharedAcrossAdmissions` around both executor
admissions and the manual-preflight continuation. Prove one deadline survives
request construction. Do not use a real 15-second sleep to prove budget accounting.

Cover contention after STARTING/credential writes and before lifecycle start.
Check guarded rollback, failure precedence, and no peer-runtime cleanup.
Retain a real-error control that still reaches generic failure bookkeeping.
Cover a healthy selected slot plus a changed or invalid sibling.
Use existing cancellation and selection-snapshot tests as patterns.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run 'TestSessionOpenResumeWaitsForWorkspaceInspection|TestResumeInspectionContentionPreservesTaskState' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/worktree ./internal/orchestrator/executor ./internal/orchestrator ./internal/orchestrator/handlers ./internal/agent/runtime/lifecycle -run 'ResumeInspection|SessionOpenResumeWaits|RecoveryInspection|RecoveryAdmissionWaitPolicy|RecoveryAdmissionRejectsSelectionDrift|ManualRecoveryPreflight|WorktreeRecoveryResumeIntegration|SessionRecoveryGuard|Resume.*Cancellation' -count=1)
git diff --check
```

The first command is the behavioral RED gate and subsequent GREEN regression.
The second covers the new matrices and affected existing contracts.

## Files likely touched

- `apps/backend/internal/worktree/recovery_admission.go`
- `apps/backend/internal/worktree/recovery_admission_wait_policy_test.go`
- `apps/backend/internal/worktree/recovery_admission_context_test.go`
- `apps/backend/internal/orchestrator/executor/executor_resume.go`
- `apps/backend/internal/orchestrator/executor/executor_worktree_recovery.go`
- `apps/backend/internal/orchestrator/executor/executor_resume_inspection_contention_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/session_open_inspection_contention_test.go` (new)
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_restore_admission_test.go`

If the code exceeds repository complexity limits, extract focused helpers.
If the manual-only budget identifier changes, update its test references.

## Dependencies

None. Read the completed manual-admission work order in
`docs/plans/managed-clone-recovery-convergence/` for its lock-order constraints.
That historical work order remains complete. This package extends outer callers.

## Risks

If rollback fails or dispatch began, the error cannot receive the safe contention exemption.
Terminal-session cleanup must not turn into removal of a peer's live workspace.
Do not reenter a singleflight bucket or acquire a recovery mutex recursively.

## Parallelism

`sequential`

## Inputs

- Requirement 003, criteria .6, .7, and .9.
- System design: Inspection contention during resume.
- Existing `executor_worktree_recovery_integration_test.go` and
  `executor_manual_recovery_contention_test.go`.
- Existing `manager_workspace_restore_admission_test.go` and
  `task_operations_resume_cancellation_test.go`.

## Results

Implemented one absolute 15-second admission deadline across ordinary
session-open resume, explicit manual preflight, and later executor admissions.
`resumeAttemptRegistry.begin` records it before request cancellation is detached,
preserving a shorter caller deadline without changing attempt cancellation or
ownership semantics.

Lifecycle contention after both outer admissions now uses the same
attempt-fenced rollback as outer contention. It restores the prior session state,
error message, and credential snapshot before returning a verified safe deferral.
Rollback failure, lost attempt ownership, and joined startup or cleanup failures
do not receive that marker; they retain ordinary failure bookkeeping and are not
mapped to the retryable conflict.

Regression coverage includes real-lock session-open contention with a 300 ms
caller deadline, explicit manual preflight deadline retention, equal deadlines
across later admissions, lifecycle refusal after both admissions succeed,
state/error/credential restoration, rollback failure, joined startup failure,
successor-attempt preservation, and task-level failure bookkeeping for unsafe
outcomes. `TestSessionOpenResumeWaitsForWorkspaceInspection` also confirms the
same retained conversation starts after the held lock is released.

Validation passed:

- Full tests for worktree, executor, orchestrator, handlers, and lifecycle
  packages.
- The focused `-race` regression set across those packages.
- Backend build and `git diff --check`.
