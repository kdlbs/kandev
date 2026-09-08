---
created: 2026-09-01
status: implemented
requirements:
  - REQ-TASKS-PROMPT-ATTACHMENTS-001
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
system_design:
  - ../../specs/tasks/system-design/prompt-attachments.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
legacy_specs: []
---

# Implementation Plan: Session Launch Prompt Admission

## Overview

The `session.launch` entry point did not claim staged attachments. The runtime
therefore rejected attachment materialization before the initial ACP prompt.

The asynchronous dispatch path logged that error but did not publish a terminal
execution result. The session and its turn remained `RUNNING` for five hours.

This plan first adds attachment admission to the shared launch boundary. It then
makes every genuine initial-prompt error use the existing agent-failure path.

## Scope

### In scope

- Claim file-backed attachments before a launch intent starts runtime work.
- Preserve idempotency for task-scoped claims from task creation.
- Reject cross-task and cross-session attachment reuse.
- Settle the current execution, turn, and session after initial-prompt errors.
- Add backend regression coverage and one New Agent end-to-end scenario.

### Out of scope

- Changes to upload limits, file retention, or delivery modes.
- Changes to the New Agent layout or prompt composer.
- Automatic cancellation of quiet turns that already produced agent activity.
- Recovery of the original incident session or its discarded ACP thread.

## Technical approach

### Launch attachment admission

Add a narrow attachment-claimer dependency to
`apps/backend/internal/orchestrator/service.go`. Wire the task service through
`apps/backend/internal/backendapp/orchestrator.go`.

In `apps/backend/internal/orchestrator/session_launch.go`, claim file-backed
descriptors after task and session authorization. Complete the claim before the
selected launch intent can change task, session, turn, or executor state.

Use the request session ID when it exists. A launch without a session ID uses a
task-scoped claim because the orchestrator has not created the session.

Update `apps/backend/internal/task/repository/sqlite/attachment.go` so an
existing task-scoped claim is idempotent for a later session on the same task.
Keep an existing session-scoped claim unavailable to another session.

### Initial prompt failure reconciliation

Change the initial-prompt failure callback in
`apps/backend/internal/agent/runtime/lifecycle/session.go` to carry the dispatch
error. Wire the callback in `NewManager` from
`apps/backend/internal/agent/runtime/lifecycle/manager.go`.

For a genuine dispatch error, use the existing terminal execution path. It
publishes `agent.failed` with current prompt evidence and releases execution
activity.

Keep shutdown and transport teardown on the existing stopped-session path. Do
not create a user-visible failure for backend shutdown.

The orchestrator uses its existing execution and prompt correlation before it
settles the turn and session. Duplicate process-exit errors remain idempotent.

### User-visible regression coverage

Extend `apps/web/e2e/tests/session/new-session-dialog.spec.ts`. The scenario
uploads a file in the New Agent dialog and starts the second session.

The scenario waits for agent output and a settled session. It also checks the
persisted user-message attachment descriptor after a reload.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.2` | `starts a second session with a staged attachment` in `apps/web/e2e/tests/session/new-session-dialog.spec.ts` |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.3` | `TestLaunchSession_RejectsAttachmentClaimBeforeStart` in `apps/backend/internal/orchestrator/session_launch_test.go` |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.4` | `TestClaimMessageAttachments_AllowsTaskScopedClaimForSameTaskSession` in `apps/backend/internal/task/repository/sqlite/attachment_test.go` |
| `AC-TASKS-PROMPT-ATTACHMENTS-001.5` | `TestDispatchInitialPromptReportsDeliveryFailure` in `apps/backend/internal/agent/runtime/lifecycle/session_attachments_test.go` |
| `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.9` | `TestInitialPromptFailureMarksExecutionFailedAndReleasesActivity` in `apps/backend/internal/agent/runtime/lifecycle/activity_test.go` |
| `AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.10` | `TestInitialPromptFailureCannotFailReplacementExecution` in `apps/backend/internal/agent/runtime/lifecycle/activity_test.go` |

## E2E tests

`apps/web/e2e/tests/session/new-session-dialog.spec.ts` will contain
`starts a second session with a staged attachment`.

The test covers `AC-TASKS-PROMPT-ATTACHMENTS-001.2`. It must fail before the fix
because the second session receives no agent output and does not settle.

`TestDispatchInitialPromptReportsDeliveryFailure` and
`TestInitialPromptFailureMarksExecutionFailedAndReleasesActivity` provide the
failure-path evidence for `AC-TASKS-PROMPT-ATTACHMENTS-001.5`.

## Work orders

- [x] [Task 01: Admit launch attachments](task-01-admit-launch-attachments.md)
- [x] [Task 02: Settle initial prompt failures](task-02-settle-initial-prompt-failures.md)

## Verification results

- The attachment repository claim tests passed, including task-scoped promotion
  and rejection of a different session after a session-scoped claim.
- The orchestrator package passed all 2,317 tests.
- The task repository and backend composition packages passed all 1,597 tests.
- The complete backend test suite passed after removing the runtime-injected
  `KANDEV_INTERNAL_CONFIG_FILE` variables from the test command.
- The focused lifecycle tests passed under the race detector.
- The New Agent attachment Playwright scenario failed before implementation
  with a `RUNNING` session timeout, then passed after implementation and reload.

## Risks

- Task creation already claims attachments before it creates a session. The
  launch claim must accept that exact task scope without broadening access.
- Initial prompt dispatch runs in a goroutine. A process exit or replacement
  can race with the new terminal callback.
- Some launch callers are internal. The claimer must ignore inline descriptors
  and preserve trusted calls that carry no staged attachment IDs.
- Playwright uses prebuilt backend and web artifacts. Stale artifacts can hide
  or misreport the regression.
