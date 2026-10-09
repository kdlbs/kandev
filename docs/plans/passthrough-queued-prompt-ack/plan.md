---
created: 2026-09-24
status: done
requirements:
  - REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001
system_design:
  - ../../specs/tasks/system-design/passthrough-queued-prompt-dispatch.md
legacy_specs: []
---

# Implementation Plan: Passthrough Queued Prompt Acknowledgement

## Overview

The ordinary passthrough ready drain reserves the queue head with a retained
reservation. After a successful PTY write it returned before acknowledging the
reservation. Every later turn end therefore re-reserved the same entry and
wrote it again, and the entries behind it were never delivered. One work order
restores the acknowledgement and adds the regression proof.

## Scope

### In scope

- Acknowledge the reservation after a successful PTY write of a text-only
  queued prompt in `handleAgentReady`.
- Add a regression test over repeated `agent.ready` events.

### Out of scope

- A user-facing action to purge a reserved queue entry.
- Durable lifecycle and attachment-bearing entries, which already acknowledge
  through the guarded executor path.
- Changes to the write-failure path, which already releases or requeues.

## Technical approach

Remove the early `return` after a successful write in the ordinary passthrough
branch of `handleAgentReady`. The success path then falls through to the
existing `AcknowledgeQueuedForSession` call, which already served the
empty-prompt case, with the same error filtering.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.1` | `TestHandleAgentReady_PassthroughQueuedMessageDeliveredOnce` queues two prompts and sends three `agent.ready` events. It asserts one PTY write per prompt, in order, and that neither entry remains in storage, reserved or otherwise. |

## Work orders

- [x] [Task 01: Acknowledge delivered passthrough queue reservations](task-01-acknowledge-delivered-reservation.md)

## Verification results

- `go test ./internal/orchestrator -run TestHandleAgentReady_PassthroughQueuedMessageDeliveredOnce -count=1` fails before the fix (the first prompt is written three times and the second never) and passes after it.
- `go test ./internal/orchestrator/... -count=1` passed.

## Risks

- A durable plan comment is now acknowledged right after a successful write.
  This matches the attempted-delivery acknowledgement already made by the
  write-failure path.
