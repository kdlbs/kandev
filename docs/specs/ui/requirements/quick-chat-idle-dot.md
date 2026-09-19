---
status: draft
system: ui
created: 2026-08-16
updated: 2026-08-23
owners:
  - kandev
---
# Quick Chat Activity Indicators Requirements

## Overview

Quick Chat agents continue to work after the user closes the dialog. Users need
to see which tab is active and whether background work is complete.

## Requirements

### REQ-UI-QUICK-CHAT-IDLE-DOT-001: Quick Chat Activity Indicators

**Intent:** Show live work and unseen completions for Quick Chat sessions without
adding a second notification ledger or changing the backend event contract.

#### Acceptance criteria

- **AC-UI-QUICK-CHAT-IDLE-DOT-001.1:** Each server-backed Quick Chat conversation tab shows the shared grid spinner while its agent has live work, including session startup, a running session, reported background activity, and environment preparation.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.2:** Setup tabs and Quick Terminal tabs do not show the Quick Chat grid spinner.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.3:** The Quick Chat entry shows an activity bubble only while the Quick Chat dialog is closed. A blue bubble means that at least one Quick Chat agent in the active workspace has live work.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.4:** An emerald bubble means that all Quick Chat agents in the active workspace have settled and at least one result is unseen. The running state has priority when some agents are settled and at least one agent still has live work.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.5:** The bubble appears on the collapsed sidebar item, expanded sidebar shortcut, tablet header, mobile header, and mobile task-switcher entry.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.6:** Opening the Quick Chat dialog clears all unseen completion markers and hides the entry bubble. Closing it re-arms tracking for later work or completions.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.7:** Activity and completion markers are scoped to the active workspace. A tab removal or backing-task deletion removes that session's marker.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.8:** Failed, canceled, and abandoned turns count as settled. A completion received while the dialog is open does not create an unseen completion marker.
- **AC-UI-QUICK-CHAT-IDLE-DOT-001.9:** Button labels describe the running or finished state. The colored bubble remains decorative and does not change the labels, tooltips, or dialog behavior.

## Migrated source detail

## Why

Quick Chat agents continue to work after the user closes the dialog. Users need
to see which tab is active and whether background work is complete.

## What

- Each server-backed Quick Chat conversation tab shows the shared grid spinner
  while its agent has live work.
- Live work includes session startup, a running session, reported background
  activity, and environment preparation.
- Setup tabs and Quick Terminal tabs do not show the grid spinner.
- The Quick Chat entry shows an activity bubble only while the Quick Chat dialog
  is closed.
- A blue bubble means that at least one Quick Chat agent in the active workspace
  has live work.
- An emerald bubble means that all Quick Chat agents in the active workspace
  have settled and at least one result is unseen.
- The running state has priority when some agents are settled and at least one
  agent still has live work.
- The bubble appears on the collapsed sidebar item, expanded sidebar shortcut,
  tablet header, mobile header, and mobile task-switcher entry.
- Opening the Quick Chat dialog clears all unseen completion markers and hides
  the entry bubble.
- If the user closes the dialog while work continues, the blue bubble appears
  again.
- If the user closes the dialog after all results are seen, no bubble appears
  until new work starts or a new result arrives.
- Completion markers are scoped to a workspace. A tab removal or backing-task
  deletion removes its marker.
- Button labels describe the running or finished state. The colored bubble
  remains decorative.

## State machine

The entry indicator is derived from live session rows and the existing client-only
completion markers.

| State | Condition while the dialog is closed | Transition |
|---|---|---|
| hidden | No agent has live work and no unseen completion exists | A new turn starts or an unseen completion arrives |
| running | At least one Quick Chat agent has live work | The last working agent settles, or the dialog opens |
| finished | No agent has live work and at least one unseen completion exists | A new turn starts, the dialog opens, or the marked tab is removed |

Opening the dialog clears `unseenIdleByWorkspace` for all workspaces. Live
activity remains in the session store and is not copied into a second ledger.

## Failure modes

- If the WebSocket is disconnected, the live row can be stale until the existing
  resync updates it. If a completion event is missed, no finished bubble appears
  for that result. The indicator remains a best-effort cue.
- If a session row is not loaded, that session does not count as running. Its
  existing unseen marker can still produce the finished state.
- A completion received before its Quick Chat tab is known does not create a
  marker. The user has not seen that chat window.
- Failed, canceled, and abandoned turns count as settled. This preserves the
  existing completion indicator behavior.
- Duplicate settle events use the existing 60-minute, 500-session ledger. They
  do not restore a marker while their ledger entry remains. A duplicate may
  restore a marker after its ledger entry expires or is removed by the entry
  limit.
- `AbandonOpenTurns` publishes `session.turn.completed` for orphan turns whose
  completion time equals their start time. The marker treats these uniformly
  with live completions because the session settled to idle; filtering them is
  out of scope.
- Marker timing uses receipt time. An event handled while the dialog is open is
  seen, even if the turn settled before the dialog opened. Separate event-bus
  subscriptions do not guarantee relative delivery order, so the marker relies
  on session-tab membership and the selector for workspace scoping.

## Persistence guarantees

The running state comes from live session data. Unseen completion markers remain
browser-memory state and reset on page reload.

## Scenarios

- **GIVEN** a Quick Chat agent starts work, **WHEN** its conversation tab is
  visible, **THEN** that tab shows a grid spinner.
- **GIVEN** a Quick Chat agent settles, **WHEN** its conversation tab is
  visible, **THEN** that tab does not show a grid spinner.
- **GIVEN** the dialog is closed and one agent has live work, **WHEN** the Quick
  Chat entry renders, **THEN** it shows a blue running bubble.
- **GIVEN** one agent has settled and another agent still works, **WHEN** the
  Quick Chat entry renders, **THEN** it keeps the blue running bubble.
- **GIVEN** the dialog is closed and the last working agent settles, **WHEN**
  the completion arrives, **THEN** the bubble changes from blue to emerald.
- **GIVEN** an emerald finished bubble, **WHEN** the user opens the dialog,
  **THEN** the bubble disappears and the results become seen.
- **GIVEN** the user opens and closes the dialog after all work settles,
  **WHEN** no new work occurs, **THEN** no bubble appears.
- **GIVEN** the user opens and closes the dialog while an agent still works,
  **WHEN** the dialog closes, **THEN** the blue running bubble appears again.
- **GIVEN** the dialog is open, **WHEN** a turn completes, **THEN** no finished
  bubble is stored for that completion.
- **GIVEN** activity belongs to workspace A, **WHEN** the user views workspace B,
  **THEN** workspace B does not show that activity.
- **GIVEN** a marked session, **WHEN** its tab or backing task is removed,
  **THEN** that session stops contributing to the bubble.
- **GIVEN** a Quick Chat session tab has never completed a turn, **WHEN** the
  page reloads, **THEN** no completion bubble shows because markers are
  ephemeral and not persisted.

## Out of scope

- No backend, database, or WebSocket contract changes.
- No persisted notification history.
- No new sound, toast, operating-system notification, or command-palette
  indicator.
- No activity bubble for Quick Terminal tabs or the config-chat floating panel.
- No change to Quick Chat session creation, tab sync, or dialog behavior.

## Implementation plan

See [the current implementation plan](../../../plans/quick-chat-activity-indicators/plan.md).
