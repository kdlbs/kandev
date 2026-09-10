---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002
created: 2026-09-10
owners:
  - kandev
---

# Signal-Gated Manual Move Visibility System Design

## Purpose and boundaries

The task system owns whether a workflow transition is automatic, signal-gated,
or manually recoverable. The web application presents that state in the normal
chat and passthrough composer controls. This design changes only the visibility
policy for the existing next-step action. It does not change transition
execution, completion-signal persistence, clarification handling, or the future
ADR 0015 `manual_fallback` signal path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-002` | [Control flow](#control-flow), [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- The workflow HTTP and WebSocket payloads remain the authority for
  `auto_advance_requires_signal` and step events.
- `KanbanState.steps` retains both values for the active workflow and cached
  workflow snapshots.
- HTTP hydration, multi-workflow snapshot refresh, mobile workspace switching,
  workflow-step WebSocket mapping, and live Kanban update mapping preserve the
  signal-gated flag.
- `useNextWorkflowStep` classifies a turn-complete move as self-advancing only
  when `auto_advance_requires_signal` is not true. It continues to identify the
  adjacent next step and use the existing `moveTask` operation.
- `ChatStatusBar` and `PassthroughToolbar` retain their existing busy-state gate
  and consume the shared next-step projection.
- The phone task drawer remains the alternate manual step-move surface. This
  correction adds no phone-only layout or interaction.

## Data and contracts

`WorkflowStep.auto_advance_requires_signal` already exists on the public
frontend HTTP type. The derived `KanbanState.steps` item adds the same optional
boolean so older or partial payloads continue to behave as ungated steps.

Every projection from `WorkflowSnapshot.steps` or workflow-step WebSocket
payloads into `KanbanState.steps` copies the field without defaulting it. The
visibility rule treats only the literal value `true` as signal-gated.

## Control flow

1. Initial hydration, snapshot refresh, workspace switching, or a live
   workflow-step event writes the step events and signal-gated flag into the
   Kanban store.
2. `useNextWorkflowStep` locates the current and adjacent next step.
3. It inspects current-step `on_turn_complete` actions for `move_to_next`,
   `move_to_previous`, or `move_to_step`.
4. An ungated move suppresses the composer action because the turn completion
   can perform the move. A signal-gated move does not suppress it because a bare
   halt cannot perform the move.
5. The standard and passthrough composer surfaces show the existing action only
   when the shared projection returns a next-step name and the agent is idle.
6. Selecting the action uses the existing manual task-move request and its
   existing error handling.

## Failure and recovery

- Missing task, workflow, current step, or next step data produces no action.
- An omitted signal-gated field preserves the legacy ungated behavior.
- A busy agent keeps the action hidden. Returning to idle recomputes the surface
  without a reload.
- A rejected manual move keeps the task on the current step and uses the current
  localized error toast.
- No completion signal is synthesized, so this path cannot increment the future
  manual-fallback counter or bypass clarification barriers.

## Persistence

No schema or persisted value changes. The design preserves an existing workflow
step field in client projections that previously dropped it.

## Security

No authorization boundary changes. Manual moves continue through the existing
task-move API and backend policy checks.

## Observability

No new production metric is required for visibility. Unit tests cover all
client projection paths and the gated versus ungated decision. A browser test
proves the user-visible action after an idle signal-gated turn.

## Responsive behavior

The standard and passthrough composer surfaces share the state policy. The
phone task drawer continues to expose its existing step-move control, so no new
compressed desktop control, touch target, breakpoint, or scroll behavior is
introduced.

## Related decisions

- [ADR 0015: Explicit Completion Signal for Auto-Advance](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
