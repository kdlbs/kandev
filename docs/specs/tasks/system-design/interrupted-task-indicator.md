---
status: draft
system: tasks
requirements:
  - REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001
---

# Interrupted task warning design

## Ownership and data

Keep `tasks.metadata.interrupted_at` and the derived task DTO `interrupted` flag.
No schema, route, or event type is added. Startup reconciliation and periodic
interruption recovery mark lost mid-turn work, including missing executor rows.
Normal idle conversations and surviving executions remain unmarked.

Use concurrent-key-safe repository operations and existing archive guards.
Preserve the marker through waiting-state recovery and subsequent restarts.
Client merges retain existing interruption state when a partial payload omits it.
Successful clearing uses the existing authoritative update contract.

## Marker lifecycle

Current `event_handlers_streaming.go` clears the marker at STARTING entry.
Move that clearing to confirmed successful recovery through the existing
provider-ready or accepted-running paths. Audit every `clearTaskInterruptedMarker`
call, including `updateTaskSessionStateWithHook`, `setSessionStarting`, and
execution-correlated activity handlers. STARTING alone proves only an attempt.

A successful prompt-free resume can end in WAITING_FOR_INPUT. Clear the marker
on that success without requiring a new user prompt. Queued, refused, failed,
and cancelled attempts retain it. Existing activity spinners can temporarily
replace the warning while startup is in progress; the durable marker survives.

Use existing execution correlation and compare the observed interruption value
when clearing it. An old readiness callback must not erase a newer interruption.
Keep the update and publication retryable through the existing event path.
Publish `task.updated` only after a successful marker change. Never clear the
marker from a browser mount, navigation handler, or optimistic STARTING projection.

## Shared rendering

In `apps/web/lib/ui/state-icons.tsx`, update both `TASK_INTERRUPTED_ICON` and
`InterruptedTaskIcon`: use `IconAlertTriangle` and the existing `STYLE_WARNING`
color (`text-yellow-500`). Replace the error-colored focus ring consistently.
Keep the test ID `task-state-interrupted` and translated
`common:interruptedByRestart` label. Do not change unrelated error icons.

Consumers include sidebar rows, board cards, mobile task-switcher rows, graph
nodes, rich task lists, and the open-task header. Using the shared renderer
keeps their appearance consistent. Retain current permission, clarification,
activity, and explicit terminal-state precedence.

## Desktop and phone

Desktop keeps the icon in the current task-row and card status position.
Phone reuses the task drawer and board card status slot. No new overlay or
control is needed. Keep the accessible label and visible triangle shape.
Required meaning must not depend on tooltip hover; the task name remains primary.
Existing layout, scroll ownership, safe areas, and touch behavior remain intact.

## Verification and mapping

| Requirement | Design sections |
| --- | --- |
| REQ-TASKS-INTERRUPTED-TASK-INDICATOR-001 | Ownership and data; Marker lifecycle; Shared rendering; Desktop and phone |

Extend the marker lifecycle tests in `event_handlers_streaming_test.go` and
startup reconciliation tests. Extend `state-icons.test.tsx` to assert triangle,
warning styling, accessible label, and precedence. Browser coverage must verify
sidebar and board rendering, reload persistence, failed recovery retention,
and successful recovery removal on desktop and phone.

The [work order](../../../plans/orphaned-session-open-recovery/task-03-interruption-warning.md)
contains exact commands and the UI preview.
