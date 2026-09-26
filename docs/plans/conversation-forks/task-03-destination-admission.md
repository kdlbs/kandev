---
id: "03-destination-admission"
title: "Bind snapshots to destination launches"
status: done
wave: 3
depends_on: ['02-attachment-copies']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-FORK-002
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-004
  - REQ-TASKS-CONVERSATION-FORK-005
acceptance_criteria:
  - AC-TASKS-CONVERSATION-FORK-002.6
  - AC-TASKS-CONVERSATION-FORK-003.6
  - AC-TASKS-CONVERSATION-FORK-004.1
  - AC-TASKS-CONVERSATION-FORK-004.2
  - AC-TASKS-CONVERSATION-FORK-004.3
  - AC-TASKS-CONVERSATION-FORK-004.4
  - AC-TASKS-CONVERSATION-FORK-004.5
  - AC-TASKS-CONVERSATION-FORK-004.6
  - AC-TASKS-CONVERSATION-FORK-004.7
  - AC-TASKS-CONVERSATION-FORK-004.8
  - AC-TASKS-CONVERSATION-FORK-005.1
  - AC-TASKS-CONVERSATION-FORK-005.3
  - AC-TASKS-CONVERSATION-FORK-005.4
system_design:
  - ../../specs/tasks/system-design/conversation-forks.md
---

# Task 03: Bind snapshots to destination launches

## Summary

Attach frozen context atomically to each destination and deliver it through the existing first-prompt pipeline. Preserve context across delayed starts and retries.

## In scope

- Enforce AC-004.8: agents share the source execution workspace, new tasks use a separate workspace, and child tasks offer both modes.

- Own optional fork/request fields in existing task-create and session-launch contracts.
- Own transactional destination creation, snapshot and attachment binding, creation receipts, and rollback.
- Own pending task-to-first-session binding, capacity deferral, restart recovery, and initial-message provenance.
- Keep context estimates informational. Test that an over-window estimate still reaches provider dispatch and a provider rejection uses existing error handling.
- Own separate historical/current prompt composition and destination snapshot read authorization.
- Cover new task, child task, new agent, workflow templates, structured and supported passthrough destinations.
- Extend cleanup for destination deletion, source deletion, and workspace deletion.

## Out of scope

A new launcher, provider-native state copying, workspace materialization changes, and browser UI.

## Acceptance

- All three destinations preserve their normal settings and receive the same accepted snapshot before the new request.
- Duplicate requests, conflicting fingerprints, revoked source access, and failed persistence cannot produce partial or duplicate destinations.
- Deferred starts, restarts, workflow composition, and launch retries retain one context reference without historical saved-prompt expansion.

## Verification

Run from the repository root. Add failing behavioral tests before production changes.
All new test names and files are specified in the plan's coverage table.
A missing test file or selector alone is not behavioral RED evidence.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite ./internal/orchestrator -run '^TestConversationFork' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable PostgreSQL test DSN}"
(cd apps/backend && go test ./internal/task/repository/sqlite -run '^TestConversationForkPostgres' -count=1)
(cd apps/backend && go test ./internal/orchestrator -run '^(TestLaunchSession_ExpandsSavedPromptsWithoutWorkflowStep|TestStartCreatedSession_InitialTaskBrief|TestStartCreatedSessionConsumesTheDeferredLaunch)' -count=1)
```

PostgreSQL uses a disposable database and isolated test schemas. A skipped suite leaves this work order incomplete.

## Files likely touched

- `apps/backend/internal/task/service/conversation_fork_admission.go` (new), `service_tasks.go`
- `apps/backend/internal/task/repository/interface.go` and `sqlite/conversation_fork.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go` and `conversation_fork_handlers.go`
- `apps/backend/internal/orchestrator/session_launch.go`, `task_operations.go`, `task_create_prompt.go`, `workflow_start_prompt.go`
- `apps/backend/internal/orchestrator/deferred_launch_consume.go` and first-message ownership helpers
- `apps/backend/internal/task/models/` and `apps/backend/pkg/api/v1/` bounded fork descriptors
- `apps/backend/internal/task/service/conversation_fork_admission_test.go` (new)
- `apps/backend/internal/orchestrator/conversation_fork_launch_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/conversation_fork_postgres_test.go` (extend)

## Dependencies

Task 02 must pass before this work starts.

## Risks

Creation spans multiple existing persistence boundaries. Replaying launch strings or trusting request metadata would bypass snapshot authority.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-forks.md) and [system design](../../specs/tasks/system-design/conversation-forks.md).
- `LaunchSession`, `prepareStartAgentSession`, `Service.CreateTask`, initial-task-brief design, saved-prompt design, and design sections Admission and delivery through Authorization and failures.

## Results

Completed after behavioral RED/GREEN coverage for transactional session binding, request admission, prompt delivery, provenance, deferred replay payloads, source deletion retention, and destination/workspace cleanup. The repository also preserves an attached copy after source deletion and removes it with the destination.

Validation passed:

- `(cd apps/backend && go test ./internal/task/service ./internal/task/handlers ./internal/task/repository/sqlite ./internal/orchestrator -run '^TestConversationFork' -count=1)`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/task/repository/sqlite -run '^TestConversationForkPostgres' -count=1` against the disposable local PostgreSQL container
- `(cd apps/backend && go test ./internal/orchestrator -run '^(TestLaunchSession_ExpandsSavedPromptsWithoutWorkflowStep|TestStartCreatedSession_InitialTaskBrief|TestStartCreatedSessionConsumesTheDeferredLaunch)' -count=1)`
- `(cd apps/backend && go build ./...)`

The PostgreSQL check initially exposed an existing missing-table transaction-abort path when purging queue-session policy rows. Added a savepoint around that optional cleanup and extended its regression test to include a task session with the queue policy schema absent.

### Review remediation

Task-level fork provenance no longer becomes session delivery metadata through task metadata cloning. The repository stamps the fork reference only on the session that atomically claims the pending snapshot. Regression coverage verifies selected copied attachments reach agent, new-task, and delayed-task launch requests, with task uploads kept alongside fork copies, then starts an ordinary additional session and verifies it receives neither fork attachments nor delivery metadata. New-task and separate-child admission now fail before creation when the resolved executor is Local and cannot isolate its execution workspace.

Destination receipts remain retryable until synchronous setup completes. Failed creation restores the attached draft and copied attachment claims before deleting the partial destination. Failed first-session delivery retries the same session with the persisted user message and frozen fork payload; ordinary later sessions and workflow-created sessions receive no fork metadata. The first claim and attachment merge are covered by launch regressions.

Validation passed:

- `go test ./internal/task/repository/sqlite -run '^TestConversationForkPendingTaskBindsFirstSessionAtomically$' -count=1`
- `go test ./internal/task/service -run '^(TestConversationForkSeparateWorkspaceRejectsLocalExecutor|TestConversationForkTaskAdmissionIsIdempotentAndBindsMetadata)$' -count=1`
- `go test ./internal/orchestrator -run '^(TestConversationForkLaunch|TestLaunchSessionDeliversAgentForkAttachments|TestStartTaskDeliversForkAttachmentsWithNewPromptAttachments|TestStartCreatedSessionDeliversPendingForkAttachments)$' -count=1`
