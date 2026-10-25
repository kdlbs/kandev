---
id: "01-backend-archive-gate"
title: "Gate backend recovery on archive state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Gate backend recovery on archive state

## Summary

Make session recovery eligibility depend on the owning task's current archive
state. Reject direct archived workspace restoration before runtime creation.

## In scope

- Add `TestGetTaskSessionStatus_ArchivedTaskDisablesRecovery` in a focused
  `task_session_archive_status_test.go`. Cover archive cancellation with no
  running row, retained resumable token, ordinary cancellation, completed state,
  and a still-stopping runtime row. Assert no runtime probe or state healing.
  Include authorized missing-task and task/session mismatch cases.
- Add an active-task counterpart that proves unarchived archive cancellations
  retain eligibility. Keep explicit user stops and completed sessions stopped.
- Read the owning task after authorization and binding checks, before runtime
  eligibility work. Return false `is_resumable`, `needs_resume`, and
  `needs_workspace_restore`, with `resume_reason=task_archived` when archived.
  Preserve the stored session state and token. Fail closed on lookup errors.
- Add `TestLaunchRestoreWorkspace_ArchivedTask` in
  `session_launch_archive_test.go`; assert no call to the workspace runtime.
  Reuse `ErrTaskArchived` and preserve existing executor resume locks/checks.
- Add `TestWSLaunchSession_ArchivedConflict` in a focused handler test file.
  Map the sentinel to conflict details `{kind: task_archived}` with a bounded
  message and no internal-error stack trace. Preserve other error mappings.
- Review the direct `session.recover` path for archive rejection before
  identity-changing recovery actions; reuse the same task guard if needed.
  A rejected archived operation must not clear a token before launch rejects it.

## Out of scope

- Client presentation, automatic unarchive, cleanup redesign, schema changes,
  environment reconstruction, or weakening task/session authorization.

## Acceptance

1. New archived status and restore regressions fail for the diagnosed reasons
   before production changes, then pass with no runtime/state mutation.
2. Typed conflicts distinguish archive rejection; missing and foreign tasks
   retain existing authorization behavior.
3. Existing unarchived session and released-worktree recovery tests pass.

## Verification

Run from `apps/backend`. The Make test target runs the entire repository, so
these package-scoped Go commands provide the focused checks.

```bash
rtk go test -tags fts5 ./internal/orchestrator -run 'Test(GetTaskSessionStatus|LaunchRestoreWorkspace|ResumeTaskSession_(Archive|RecreatesMissingWorktreeAfterUnarchive)|RecoverSession)' -count=1 -race
rtk go test -tags fts5 ./internal/orchestrator/handlers -run 'TestWSLaunchSession_ArchivedConflict' -count=1 -race
rtk go test -tags fts5 ./internal/orchestrator/executor -run 'TestResumeSession_(RejectsArchivedTask|ArchiveCancelled)|TestWorkspaceReuseAllowed' -count=1 -race
rtk go test -tags fts5 ./internal/worktree -run 'TestCreate_RestoresReleasedWorktreeAfterArchive' -count=1 -race
```

Run each new regression alone for RED first. Confirm each filter discovers its
tests. If handler tests use an existing name convention, update this command
to the actual added name before recording GREEN.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_session_archive_status_test.go` (new)
- `apps/backend/internal/orchestrator/task_operations_unarchive_resume_test.go`
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/task_session_archive_status_test.go` (new)
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/session_archive_conflict_test.go` (new)

## Dependencies

None.

## Risks

- Test repositories that omit owning tasks need realistic task fixtures.
- An early guard is not a replacement for existing archive/launch race barriers.
- Recovery status fields describe current eligibility, not loss of provider data.

## Parallelism

`sequential`

## Inputs

- [Requirement 004](../../specs/agents/requirements/agent-resume-runtime-recovery.md).
- [Archive transition eligibility](../../specs/agents/system-design/agent-resume-runtime-recovery.md#archive-transition-eligibility).
- Existing `task_operations_unarchive_resume_test.go`, `session_scope_test.go`,
  `session_launch_test.go`, and executor archive-rejection tests.
- [Completed worktree recovery package](../worktree-resume-after-unarchive/plan.md).

## Results

RED evidence:

- `rtk go test -tags fts5 ./internal/orchestrator -run 'Test(GetTaskSessionStatus_ArchivedTaskDisablesRecovery|RecoverSession_ArchivedTaskDoesNotClearResumeToken|LaunchRestoreWorkspace_ArchivedTask)' -count=1` failed in 5 subtests. Archived status advertised recovery, fresh start cleared the resume token, and direct restore returned nil.
- `rtk go test -tags fts5 ./internal/orchestrator/handlers -run 'TestWSLaunchSession_ArchivedConflict' -count=1` failed with `INTERNAL_ERROR` instead of `CONFLICT`.

GREEN evidence:

- `rtk go test -tags fts5 ./internal/orchestrator -run 'Test(GetTaskSessionStatus_ArchivedTaskDisablesRecovery|RecoverSession_ArchivedTaskDoesNotClearResumeToken|LaunchRestoreWorkspace_ArchivedTask)' -count=1`: 6 passed.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'Test(GetTaskSessionStatus|LaunchRestoreWorkspace|ResumeTaskSession_(Archive|RecreatesMissingWorktreeAfterUnarchive)|RecoverSession)' -count=1 -race`: 33 passed.
- `rtk go test -tags fts5 ./internal/orchestrator/handlers -run 'TestWSLaunchSession_ArchivedConflict' -count=1 -race`: 1 passed.
- `rtk go test -tags fts5 ./internal/orchestrator/executor -run 'TestResumeSession_(RejectsArchivedTask|ArchiveCancelled)|TestWorkspaceReuseAllowed' -count=1 -race`: 22 passed.
- `rtk go test -tags fts5 ./internal/worktree -run 'TestCreate_RestoresReleasedWorktreeAfterArchive' -count=1 -race`: 1 passed.

Scope notes:

- The status regression uses the planned focused sibling file and covers archive cancellation, retained resumable runtime, ordinary cancellation, completed state, and a stopping runtime row. Existing authorization and active unarchived archive-cancelled tests remain unchanged.
- The implementation reuses `executor.ErrTaskArchived`, checks the owning task after authorization and session binding, prevents fresh-start token clearing, guards direct workspace restore, and maps the sentinel to a typed WS conflict.
