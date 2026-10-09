---
id: "01-acknowledge-delivered-reservation"
title: "Acknowledge delivered passthrough queue reservations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001
acceptance_criteria:
  - AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.1
system_design:
  - ../../specs/tasks/system-design/passthrough-queued-prompt-dispatch.md
---

# Task 01: Acknowledge Delivered Passthrough Queue Reservations

## Summary

Deliver a queued prompt to a passthrough session exactly once. After a
successful PTY write, acknowledge the retained reservation so the next turn end
selects the next entry instead of rewriting the same one.

## In scope

- The ordinary text-only branch of the passthrough ready drain in
  `handleAgentReady`.
- A regression test with repeated `agent.ready` events.

## Out of scope

- Purge actions for reserved entries.
- Durable lifecycle and attachment-bearing queue delivery.

## Acceptance

- Three consecutive `agent.ready` events after two queued prompts produce one
  PTY write per prompt, in queue order.
- Each delivered entry is removed from storage, not only hidden from the visible
  queue count.
- A failed PTY write still releases the reservation, or requeues it for plan
  comments.

## Verification

```bash
go test ./internal/orchestrator -run 'TestHandleAgentReady_Passthrough' -count=1
go test ./internal/orchestrator/... -count=1
```

Run both commands from `apps/backend`.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_passthrough_running_test.go`

## Dependencies

None.

## Risks

- Acknowledging before the CLI consumes the paste would lose a prompt if the
  CLI discards it. The PTY write is the delivery boundary for passthrough
  sessions, as it is for the empty-prompt acknowledgement.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/passthrough-queued-prompt-dispatch.md`
- `docs/specs/tasks/system-design/passthrough-queued-prompt-dispatch.md`

## Results

- Removed the early return after a successful write, so the existing
  acknowledgement runs for delivered prompts.
- Added `TestHandleAgentReady_PassthroughQueuedMessageDeliveredOnce`. It fails
  before the fix, where the first prompt is written three times and the second
  is starved, and passes after it.
- `go test ./internal/orchestrator/... -count=1` passed.
