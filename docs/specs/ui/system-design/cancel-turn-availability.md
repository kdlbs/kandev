---
status: current
system: ui
created: 2026-09-15
requirements:
  - REQ-UI-CANCEL-TURN-PROGRESS-001
---

# Cancel control availability

## Purpose and boundaries

The existing UI cancellation requirement owns this reusable control contract.
Task lifecycle, agent capability negotiation, and backend cancellation remain
with their current owners. This design covers control availability and command
targeting; the existing cancellation projection is defined by the
[backend-owned progress ADR](../../../decisions/2026-08-03-backend-owned-cancellation-progress.md).

## Requirement mapping

| Criteria under REQ-UI-CANCEL-TURN-PROGRESS-001 | Design section |
| --- | --- |
| AC-UI-CANCEL-TURN-PROGRESS-001.1 through .8 | Existing progress ADR; Shared control |
| AC-UI-CANCEL-TURN-PROGRESS-001.9, .10, .12 | Shared control |
| AC-UI-CANCEL-TURN-PROGRESS-001.11 | Palette ownership |

## Shared control

`useSessionState` already distinguishes `isWorking` from `isAgentBusy`.
The latter describes queue admission; it is false during direct steering and
background work. `useComposerProps` passes `panelState.isWorking` as a required
`ChatInputContainer` prop. `shouldShowCancelAgent` consumes that signal and
retains its connected/disconnected clarification override.

Preserve STARTING eligibility, already offered by the queue-based control.
Use the hook's preparation-inclusive working signal so status and cancel agree
while an identified session prepares its executor. A missing session ID cannot
produce an actionable cancel control. Moving a task alone is not working.

`SubmitButton` receives explicit `canCancelAgent` through the existing body and
both toolbar presentations. Preserve `isAgentBusy` for send visibility, input
copy, queue handling, and styling. Keep pending cancellation disabled and animated
through the existing optimistic and backend session-keyed state. Keep the
existing `agent.cancel` handler, payload, authorization, and retry behavior.
Use the existing localized cancel label as the icon button's accessible name.

The passthrough composer keeps Escape and its parent `onCancel` callback as
editor dismissal. It does not own an agent-cancel transport, so it explicitly
suppresses the agent cancel control even while its session is working. This
prevents a dismissal callback from being mistaken for cancellation.

Phone composition reuses `chat-input-toolbar-mobile.tsx` and its existing
44px controls. Desktop retains the 28px controls. No new overlay or scroll owner
is introduced: messages scroll above the composer, whose current viewport,
keyboard, and safe-area handling remain authoritative.

## Palette ownership

Reuse `buildSessionCommands` for the localized cancel entry; do not mount the
full task-oriented `SessionCommands` in Quick Chat. That component also creates
worktree, panel, archive, and subtask actions that do not belong to this surface.

A small Quick Chat command component uses its explicit session ID, working state,
clarification state, and existing `handleCancelTurn`. Register only when
`quickChat.isOpen`, `activeKind === "conversation"`, and `activeSessionId` matches
that structured conversation. Require the same cancel eligibility as the composer.
Guard dispatch against cancellation already pending for that session.

While Quick Chat is open, suppress only the underlying task's `session-cancel`
entry in `SessionCommands`. Restore it on close. Suppression also applies when
Quick Chat shows setup, terminal, or idle content. Do not change unrelated task
commands. `useRegisterCommands` removes registrations when the source unmounts
or its inputs change. Tab switching must replace the callback and eligibility.

The registry concatenates sources without deduplication. Priority cannot enforce
ownership; tests must assert exactly one eligible cancellation entry, or zero
when the foreground surface is ineligible. Never obtain the Quick Chat target
from `tasks.activeSessionId`.

## Compatibility and verification

No protocol, persistence, runtime flag, or cancellation timeout changes.
Steering and background promptability retain their existing contracts.
Tests distinguish working state from queue state and verify the full prop path.
The queue E2E helper shall wait on the existing queue-derived composer classes
(`chat-input-running` or `chat-input-running-plan`), scoped to the active composer,
instead of treating cancellation visibility as evidence of queue admission.

Use the [fix package](../../../plans/cancel-turn-availability/plan.md) for exact
unit, browser, and mobile verification. This design adds no backend telemetry.
