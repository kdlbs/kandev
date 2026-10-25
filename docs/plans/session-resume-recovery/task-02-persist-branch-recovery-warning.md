---
id: "02-persist-branch-recovery-warning"
title: "Expose typed branch recovery and persist warnings"
status: done
wave: 2
depends_on:
  - "01-create-replacement-worktrees"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.4
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003.11
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 02: Expose Typed Branch Recovery and Persist Warnings

## Summary

Map confirmed branch loss to structured recovery details. Persist one warning
status message after each confirmed explicit branch replacement, including a
replacement committed before a later repository preparation failure.

## In scope

- Add a typed orchestrator recovery error that wraps
  `worktree.ErrBranchUnrecoverable` and retains repository branch context.
- Map the typed error to a WebSocket conflict with
  `kind = branch_unrecoverable` and
  `recovery_action = resume_new_branch`.
- Keep the descriptive message for clients that use only `Error.message`.
- Capture task environment repository branch state before explicit recovery.
- Compare repository state after successful recovery.
- Build a stable decision ID from session, repository, original branch, new
  branch, and base branch.
- Claim warning persistence with a state-guarded
  `SetSessionMetadataKeyIfAbsentIfState` value that includes a timestamp.
- Create a `status` message with `variant = warning`,
  `kind = branch_recreated`, and complete structured metadata.
- Release the claim after message creation fails.
- Reclaim a stale timestamped claim only when no matching warning message
  exists, covering a process crash between the claim and message write.
- Persist one warning for each replaced repository in a multi-repository task.
- Persist the branch transition on every terminal resume path after workspace
  preparation can materialize it, including provider startup/readiness failure.
- Publish the created message through the existing message adapter path.

## Out of scope

- A new runtime stream event.
- Frontend error handling and warning rendering.
- Worktree creation logic.
- Database schema changes.
- Raw Git output in WebSocket details or persisted messages.

## Acceptance

- Only an error chain that matches `ErrBranchUnrecoverable` advertises the
  explicit branch action.
- The recovery details identify the session, repository, old branch, and base
  branch without exposing secrets or raw Git output.
- Successful branch continuation persists one status message with the exact
  warning kind and required metadata.
- Repeated persistence, event replay, and reload do not create a duplicate.
- A failed message write releases the claim and a later attempt can succeed.
- A crash before message creation leaves a reclaimable claim, while a matching
  persisted warning prevents duplicate creation.
- A later preparation failure does not lose warnings for earlier replacements.
- A provider startup or readiness failure after replacement still leaves one
  warning persisted or retryable.
- Each replaced repository in a multi-repository task receives its own warning.

## Verification

Start with handler and persistence tests that fail against the current generic
error and missing message. Then run:

```bash
# From apps/backend:
rtk go test ./internal/orchestrator/handlers -run 'Test.*RecoverSession.*Branch' -race
rtk go test ./internal/orchestrator -run 'Test.*(BranchRecovery|BranchRecreated|WarningClaim)' -race
```

## Files likely touched

- `apps/backend/internal/orchestrator/session_launch.go`
- `apps/backend/internal/orchestrator/session_launch_test.go`
- `apps/backend/internal/orchestrator/handlers/handlers.go`
- `apps/backend/internal/orchestrator/handlers/handlers_test.go`
- `apps/backend/internal/orchestrator/session_branch_recovery.go`
- `apps/backend/internal/orchestrator/session_branch_recovery_test.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/backendapp/adapters.go`

## Dependencies

- Task 01 provides the explicit action and records the replacement in the task
  environment repository state.

## Risks

- A claim made before launch can suppress a warning for a failed replacement.
  Claim only after the old and new branches differ, and persist it even when a
  later repository fails so committed replacements remain visible.
- A process can stop between the claim and message write. Timestamp the claim
  and reclaim it only after it is stale and no matching message exists.
- A decision ID without repository identity can collapse distinct warnings in
  a multi-repository task.
- Persisted content can bypass localization if it contains user-facing prose.
  Keep the content neutral and render localized text from structured metadata.
- A generic internal error code prevents safe action selection. Return a
  conflict only for the typed branch-loss error.

## Parallelism

`sequential`

## Inputs

- Completed Task 01 behavior.
- `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002`.
- `REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003`.
- Existing model-selection warning claim and persistence helpers.
- Existing WebSocket `ErrorPayload` contract.

## Results

- Implemented typed branch-recovery details with the descriptive backend
  message preserved for generic clients. Only an error chain matching
  `ErrBranchUnrecoverable` advertises `resume_new_branch`.
- Implemented before-and-after repository branch snapshots, atomic warning
  claims, claim release after write failure, stale-claim reclamation after a
  crash, and idempotent status-message persistence for each replaced repository,
  including partial preparation failures.
- GREEN: `rtk go test ./internal/orchestrator -run
  'Test.*(BranchRecovery|BranchRecreated|WarningClaim)' -race` (3 passed).
- GREEN: focused handler and orchestrator recovery tests pass with
  `rtk go test ./internal/orchestrator ./internal/orchestrator/handlers -run
  'BranchRecovery|RecoverSession'`. The narrower handler race selector matched
  no tests and exited successfully.
