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
execution, completion-signal persistence, clarification lifecycle handling, or
the future ADR 0015 `manual_fallback` signal path.

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
- `useNextWorkflowStep` exposes the existing adjacent-next-step action only for
  a signal-gated `move_to_next` action. Ungated move actions remain suppressed,
  as do signal-gated `move_to_previous` and `move_to_step` actions, because the
  existing `moveTask` operation submits the adjacent next step rather than a
  configured arbitrary destination.
- `ChatStatusBar` and `PassthroughToolbar` consume the shared next-step
  projection and shared eligibility policy. Both retain the busy-state gate
  and suppress the action while message-derived `pendingClarification` is
  present or the durable session `pending_action` is `clarification`.
- The phone task drawer remains the alternate manual step-move surface. This
  correction adds no phone-only layout or interaction.

## Data and contracts

`WorkflowStep.auto_advance_requires_signal` already exists on the public
frontend HTTP type. The derived `KanbanState.steps` item adds the same optional
boolean so older or partial payloads continue to behave as ungated steps.

Every projection from `WorkflowSnapshot.steps` or workflow-step WebSocket
payloads into `KanbanState.steps` copies the field without defaulting it. The
visibility rule treats only the literal value `true` as signal-gated.

The session `pending_action` projection is authoritative while transcript
messages hydrate. Composer surfaces retain the message-derived clarification
fallback after hydration and combine both signals through the shared chat
eligibility predicates.

## Control flow

1. Initial hydration, snapshot refresh, workspace switching, or a live
   workflow-step event writes the step events and signal-gated flag into the
   Kanban store.
2. `useNextWorkflowStep` locates the current and adjacent next step.
3. It inspects current-step `on_turn_complete` actions for `move_to_next`,
   `move_to_previous`, or `move_to_step`.
4. An ungated move suppresses the composer action because the turn completion
   can perform the move. A signal-gated `move_to_next` action does not suppress
   it because a bare halt cannot perform the move. Signal-gated
   `move_to_previous` and `move_to_step` actions remain suppressed because the
   adjacent-next-step control would submit the wrong destination.
5. The standard and passthrough composer surfaces show the existing action only
   when the shared projection returns a next-step name, the agent is idle, and
   neither the durable session projection nor the message-derived fallback
   reports a pending clarification.
6. Selecting the action uses the existing manual task-move request and its
   existing error handling.

## Failure and recovery

- Missing task, workflow, current step, or next step data produces no action.
- An omitted signal-gated field preserves the legacy ungated behavior.
- A busy agent keeps the action hidden. Returning to idle recomputes the surface
  without a reload.
- A pending clarification keeps the action hidden while the session waits for
  the user's answer. The durable session projection covers the message
  hydration window; clearing both signals recomputes the surface and restores
  eligibility when the agent is idle.
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
client projection paths, the gated versus ungated decision, the unsupported
gated move destinations, the durable clarification hydration window, and the
message-derived fallback. The boot mapper has a regression test for direct
task-page hydration. A browser test proves the user-visible action after an
idle signal-gated turn and waits for the causal backend move before asserting
the stepper.

## Responsive behavior

The standard and passthrough composer surfaces share the state policy, including
the clarification barrier. The phone task drawer continues to expose its
existing step-move control, so no new compressed desktop control, touch target,
breakpoint, or scroll behavior is introduced.

## Related decisions

- [ADR 0015: Explicit Completion Signal for Auto-Advance](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
