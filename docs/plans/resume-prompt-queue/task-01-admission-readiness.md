---
id: "01-admission-readiness"
title: "Close queue admission readiness race"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
acceptance_criteria:
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.3
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.4
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.5
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.7
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.8
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 01: Close Queue Admission Readiness Race

## Summary

Make durable queue admission trigger an automatic readiness check. Cover both
orders of admission and boot readiness through the real dispatch path.

## In scope

- Add a narrow internal collaborator after successful WebSocket queue admission.
- Reuse identity-aware automatic reservation and task-admission guards.
- Preserve accepted entries and report dispatch errors separately from admission.
- Add deterministic regression tests with TDD.

## Out of scope

- Composer changes, new wire actions, migrations, or Auto-run policy changes.

## Acceptance

1. Both readiness orders dispatch one eligible prompt without another readiness event.
2. Auto-run OFF, clarification, cancellation, reset, and workflow barriers keep entries pending.
3. Resume failure preserves the entry. Successful recovery dispatches it, and stale identity cannot redirect it.

## Verification

Run from `apps/backend`:

```bash
rtk go test -tags fts5 ./internal/orchestrator/handlers -run 'TestWsQueueMessage' -count=1
rtk go test -tags fts5 ./internal/orchestrator -run 'TestResumePromptQueue|TestHandleAgentBootReady|TestAgentBootReadyDrains|TestSetQueueAutoRun|TestQueueUserPrompt' -count=1
```

The broad Make test target cannot select these cases. Use these focused Go
commands. Repeat the new ordering cases with `-race` if supported by the environment.

## Files likely touched

- `apps/backend/internal/orchestrator/handlers/queue_handlers.go`
- `apps/backend/internal/orchestrator/handlers/queue_handlers_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/resume_prompt_queue_test.go` (new)

## Dependencies

None.

## Risks

Manual drain enables Auto-run. Admission callbacks must not hold an existing
queue lock while they acquire the dispatch guard. Deferred dispatch must not
turn successful persistence into a retryable admission error.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/resume-prompt-queue.md)
- [Design](../../specs/tasks/system-design/resume-prompt-queue.md), Server dispatch and Failure and identity
- `event_handlers_queue_lifecycle_test.go` and `event_handlers_pending_move_test.go`
- `queue_user_prompt_fastpath_test.go`
- [Server-owned Auto-run](../../decisions/2026-08-16-server-owned-queue-auto-run.md)
- `apps/backend/AGENTS.md`

## Results

Implemented the post-admission readiness collaborator, identity-bound guarded
dispatch recheck, and regression coverage for readiness ordering, Auto-run OFF,
replacement identity, and concurrent checks.

Verification on 2026-09-09:

- `rtk go test -tags fts5 ./internal/orchestrator/handlers -run 'TestWsQueueMessage' -count=1`: passed, 16 tests.
- `rtk go test -tags fts5 ./internal/orchestrator -run 'TestResumePromptQueue|TestHandleAgentBootReady|TestAgentBootReadyDrains|TestSetQueueAutoRun|TestQueueUserPrompt' -count=1`: passed, 23 tests.
- New readiness and identity cases with `-race`: passed, 6 tests.
- Handler readiness callback with `-race`: passed, 1 test.
