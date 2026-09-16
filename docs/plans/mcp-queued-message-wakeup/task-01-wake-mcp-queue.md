---
id: "01-wake-mcp-queue"
title: "Wake eligible MCP queue admissions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001
  - REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001
acceptance_criteria:
  - AC-TASKS-PARENT-CHILD-MESSAGE-INTERRUPT-001.1
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.3
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.4
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.6
system_design:
  - ../../specs/tasks/system-design/parent-child-message-interrupt.md
  - ../../specs/ui/system-design/message-queue-automation-controls.md
---

# Task 01: Wake Eligible MCP Queue Admissions

## Summary

Connect ordinary MCP peer-message admission to the existing guarded automatic
readiness check. Prove that a completed readiness event cannot strand a later
admission based on an older busy-session snapshot.

## In scope

- Required readiness method on `SessionLauncher`, real wiring, and test doubles.
- Both callers of `queueTaskMessage`, unchanged captured identity, and metadata.
- The regression matrix and backend end-to-end case in [the plan](plan.md#tests).

## Out of scope

Timeout diagnosis, deferred moves, new retries, startup recovery, storage changes,
and frontend work. Do not claim these issue symptoms are fixed by this work order.

## Acceptance

1. After readiness wins before admission, the full MCP flow delivers exactly one
   prompt without another turn event. Insertion-first and concurrent triggers
   also deliver once, preserving FIFO and sender attribution.
2. Auto-run OFF, active clarification, WIP wait, and stale incarnation retain
   their existing barriers. Rejected admission performs no wakeup. Non-parent
   queued sends never cancel a turn and cannot acquire interrupt authority.
3. Committed admission still returns accepted `queued` when dispatch defers.
   The operation does not wait for turn completion, use manual drain, or require
   another browser event. Required collaborator wiring is verified by compilation.

## Implementation and TDD

First add `TestMessageTaskReadiness_ReadyBeforeAdmission` and reproduce the missing
check. Use the existing `newTestTaskService`, `seedTaskWithSession`, and
`newMessageTaskHandler` fixtures; hold the busy snapshot while the database
session becomes idle. Then add the full handler delivery case with a real
orchestrator and controlled executor. A callback-only assertion is insufficient
as final proof of delivery.

Add the required method to `SessionLauncher` and invoke it after successful
`QueueMessageWithMetadataForSession`, outside its locks and before status
publication. Reuse the orchestrator implementation without adding eligibility
checks to the MCP layer. Keep dispatch errors separate from committed admission.

Complete the plan's regression matrix, including a second FIFO entry and
concurrent readiness. Test the paused queue remains OFF. Use existing
`queue_user_prompt_fastpath_test.go` and `resume_prompt_queue_test.go`
patterns, but assert accepted prompts rather than queue-count reduction alone.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test ./internal/mcp/handlers -run 'TestMessageTaskReadiness_' -count=1)
(cd apps/backend && go test -race ./internal/mcp/handlers ./internal/orchestrator ./internal/orchestrator/handlers ./internal/orchestrator/messagequeue -count=1)
(cd apps/backend && go test ./internal/mcp/... -run '^$')
(cd apps/backend && make build)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

All commands passed after implementation. The first command failed before the
production change with zero readiness calls instead of one, then passed after
the change. The race command passed the MCP handlers, orchestrator,
orchestrator handlers, and message queue packages. The compile-only MCP sweep,
catalog validation, specification lint, whitespace check, and backend build
also passed. The backend build reported only the existing unsigned macOS
artifact warning.

## Files likely touched

- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/message_task_readiness_test.go` (new)
- `apps/backend/internal/mcp/handlers/message_task_test.go` (existing launcher fake)
- `apps/backend/internal/orchestrator/resume_prompt_queue_test.go`
- Other MCP launcher test doubles only where the required method needs it.

## Dependencies

None. Reuse the existing `CheckQueueAdmissionReadiness` implementation in
`apps/backend/internal/orchestrator/service.go`.

## Risks

Preserve lock order and session incarnation. Do not treat a reservation as
provider acceptance. Avoid waiting for a full agent turn in the request path.
The confirmed defect may coexist with the issue's unconfirmed timeout cause.

## Parallelism

`sequential`

## Inputs

- [Peer dispatch design](../../specs/tasks/system-design/parent-child-message-interrupt.md)
- [Peer-message requirement](../../specs/tasks/requirements/parent-child-message-interrupt.md)
- [Auto-run requirement](../../specs/ui/requirements/message-queue-automation-controls.md)
- `apps/backend/internal/orchestrator/handlers/queue_handlers_admission_test.go`
- `apps/backend/internal/orchestrator/queue_user_prompt_fastpath_test.go`

## Results

Implemented the required MCP-to-orchestrator readiness seam. Ordinary
`queueTaskMessage` admissions now invoke `CheckQueueAdmissionReadiness` with
the captured queue session identity after successful insertion and before
queue-status publication. Explicit parent interrupt admissions retain their
atomic queue-and-interrupt path.

Added regression coverage for stale busy snapshots, the prepared busy branch,
rejected capacity, unrelated senders, real handler-to-provider delivery, FIFO
delivery, concurrent readiness, Auto-run OFF, active clarification, WIP wait,
and replaced session incarnations. The handler delivery test observes provider
acceptance and queue cleanup, not only queue-count reduction.

The design-turn temporary probe failed as expected before the production change
and was removed. No timeout, deferred-move, or zombie-turn claim is made.
