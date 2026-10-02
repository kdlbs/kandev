---
id: "02-error-projection"
title: "Publish current workspace recovery errors"
status: done
wave: 2
depends_on:
  - "01-manual-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
acceptance_criteria:
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.1
  - AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
  - AC-TASKS-MANAGED-CLONE-RELOCATION-003.1
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
---

# Task 02: Publish current workspace recovery errors

## Summary

Publish one current session error for verified dirty relocation refusal from
Resume, Restore, or background workspace reconstruction. Keep error ownership,
provider identity, and cancelled-session state intact.

## In scope

- Add one task-service projection operation and a narrow injected lifecycle reporter.
  Wire it in `internal/backendapp/agents.go` without importing task service into lifecycle.
- Capture the current error stamp, session state/execution, selected environment,
  and owner generation before inspection. Condition the write on that observation.
- Support absent typed metadata and legacy `error_message` records. Reuse existing
  metadata CAS machinery, adding predicates where required without a schema migration.
- Reuse a current relocation stamp for repeated refusal of unchanged inventory.
  Publish `TaskSessionErrorChanged` after a new committed projection only.
- Delegate orchestrator relocation persistence to this operation. Restore responses
  preserve existing structured relocation conflict details through `wsLaunchSession`.
- Keep the current error until relocation and relaunch succeed. Preserve newer
  session errors, unrelated metadata, shared errors, and retained history.

## Out of scope

Frontend response handling, rendered recovery controls, changing session state to
FAILED, filesystem copy changes, automatic repair, and altering task error scope.

## Acceptance

1. Each entry path projects the same eligible relocation category and action
   for a cancelled session with an old generic error. Repeated detections retain
   one stamp and do not duplicate notifications or history.
2. A newer error, successor execution, ownership transfer, or changed selected
   inventory defeats the conditional write. Failed persistence emits no actionable
   stamp. Originals, inventory, provider token, and session state remain intact.
3. Restore returns a path-free conflict with the persisted stamp. Status summary,
   boot data, and a later read expose that stamp without another Resume request.

## Regression first

Add `TestWorkspaceRecoveryProjectsRelocationErrorFromEveryEntryPoint` in
`internal/task/service/workspace_recovery_projection_test.go`. Use a real SQLite
repository, the real worktree manager, and both legacy and modern generic error states.
Exercise lifecycle reconstruction, manual preflight, and explicit workspace restore.
Before the fix, workspace reconstruction leaves the generic error unchanged.

Add `TestCommitWorkspaceRecoveryErrorPreservesSuccessorState` in
`internal/task/repository/sqlite/workspace_recovery_error_cas_test.go`. Cover absent,
equal, and newer stamps. Cover changed state/execution/environment/generation,
retained metadata, and duplicate refusal. Include equivalent real-connection PostgreSQL cases
behind `KANDEV_TEST_POSTGRES_DSN` if SQL changes.

Add `TestRestoreWorkspaceReturnsManagedCloneRelocationDetails` in a focused
orchestrator/handler test file. Capture the published event and stored error.
Keep lifecycle reporter tests separate from generic process-start failure settlement.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/service -run '^TestWorkspaceRecoveryProjectsRelocationErrorFromEveryEntryPoint$' -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/task/repository/sqlite ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/handlers ./internal/backendapp -run 'WorkspaceRecovery|CommitWorkspaceRecoveryError|RestoreWorkspace|WorkspaceRestore|ManagedClone|RecoverSessionDirtyClone|SessionMetadataKeyIfStamp|LastAgentError|TaskSessionError' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -tags fts5 -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

The first command is the RED gate and then the GREEN regression.
When PostgreSQL is available, run the second command with `KANDEV_TEST_POSTGRES_DSN`
set and record its result separately. Never print the connection string.
If unavailable, record that limitation without claiming PostgreSQL passed.

## Files likely touched

- `apps/backend/internal/task/service/workspace_recovery_projection.go` (new)
- `apps/backend/internal/task/service/workspace_recovery_projection_test.go` (new)
- `apps/backend/internal/task/service/service_turns.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/metadata_launch_error_cas.go`
- `apps/backend/internal/task/repository/sqlite/workspace_recovery_error_cas_test.go` (new)
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_workspace_recovery_projection_test.go` (new)
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/agents_workspace_recovery_test.go` (new)
- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/workspace_recovery_projection_test.go` (new)
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/workspace_recovery_projection_test.go` (new)

Use focused new test files. Do not append to the oversized `task_operations_test.go`.

## Dependencies

Task 01. Its admission ordering must not emit a corruption result before projection.

## Risks

An error writer that checks ownership before an unconditional write still races
with successor state. The write itself must carry the predicates. Background
callbacks can arrive after recovery. Their captured identity must reject that result.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/managed-clone-relocation.md), criteria listed in frontmatter.
- [Design](../../specs/tasks/system-design/managed-clone-relocation.md#durable-workspace-recovery-error-projection).
- Existing `SetSessionMetadataKeyIfStamp` and `CommitBootstrapFailureIfCurrentAttempt` transaction patterns.
- Existing `persistManagedCloneRelocationRequired`, `TaskSessionErrorChanged`, and status-summary tests.

## Results

`TestWorkspaceRecoveryProjectsRelocationErrorFromEveryEntryPoint` passed after
first failing against the generic-error behavior. Manual preflight and lifecycle
observations now carry the complete selected inventory, including each active
slot's worktree identity and the registered repository identity and path. The
transaction compares this snapshot under the task, session, environment, slot,
and repository locking conventions before either writing an error or reusing an
existing relocation stamp.

SQLite regressions cover absent, equal, and newer stamps plus successor state,
retained metadata, duplicate refusal, changed worktree ID/path/branch, changed
registered repository path, and a newly added active sibling slot. A delayed
service report after worktree drift returns no stamp, preserves the current
session error and state, and emits no actionable error event. PostgreSQL-gated
regressions cover worktree drift and sibling-slot addition for both a delayed
write and duplicate-stamp reuse. The stale-execution PostgreSQL assertion now
expects an empty stamp, matching the production guard and SQLite case.

Passed:

- `go test -tags fts5 ./internal/task/repository/sqlite -run 'TestCommitWorkspaceRecoveryError|TestPostgresCommitWorkspaceRecoveryError' -count=1`
- `go test -tags fts5 ./internal/task/service -run 'WorkspaceRecoveryProjectionRejectsInventoryChangedAfterInspection|WorkspaceRecoveryProjectsRelocationErrorFromEveryEntryPoint' -count=1`
- `go test -tags fts5 -race ./internal/worktree ./internal/task/repository/sqlite ./internal/task/service ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/executor ./internal/orchestrator/handlers ./internal/backendapp -run 'WorkspaceRecovery|CommitWorkspaceRecoveryError|RecoveryAdmissionWaitPolicy|RecoveryAdmissionRejectsSelectionDriftAfterInspectionWait|ReadRecoverySelectionSnapshot|ManualRecoveryPreflight|FreshStartPreflightPreservesProviderState|RestoreWorkspace|WorkspaceRestore' -count=1`
- `go run ./cmd/sqlguard ./internal`
- `go test -tags fts5 -race ./internal/persistence/storeconformance -count=1`
- `make -C apps/backend build`
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`
- `git diff --check`

`KANDEV_TEST_POSTGRES_DSN` was unavailable. PostgreSQL cases compiled but were
skipped; no PostgreSQL execution pass is claimed.

Review follow-up: Inventory equality includes the persisted worktree identity
and registered repository path in the same transaction as error writes and
duplicate-stamp reuse. Branch-owner values retain their stored representation,
including an explicitly empty value. The final race-enabled regression set and
backend build passed. PostgreSQL behavior remains covered by the gated cases,
but was not executed in this environment.
