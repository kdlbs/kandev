---
status: draft
created: 2026-08-16
owner: kandev
---

# Quick Chat Idle Dot

## Why

Quick chat sessions keep running after the user closes the dialog. A session's
turn can finish while the chat window is out of view, and nothing tells the
user the agent has answered. Replies are missed until the user happens to
reopen the dialog.

## What

- Each Quick Chat entry icon in the active workspace — the sidebar rail Quick
  Chat item, the sidebar Quick Chat shortcut, the tablet kanban header button,
  the mobile kanban header button, and the mobile task-switcher sheet button —
  shows a small red dot in the icon's corner while the Quick Chat dialog is
  closed and at least one quick chat session of that workspace has completed a
  turn (or settled to idle) since the dialog was last open.
- A turn that completes while the dialog is open does NOT raise a dot; the
  user is looking at the chat window.
- Opening the Quick Chat dialog clears every dot. Closing it re-arms tracking:
  a later turn completion while closed raises the dot again.
- The dot is per-workspace: only sessions belonging to the active workspace
  contribute to that workspace's entry icons.
- A session whose tab is closed or whose backing task is deleted stops
  contributing immediately.
- The dot is decorative (`aria-hidden`). Button labels, tooltips, and the
  dialog itself are unchanged.

## State machine

The unseen marker is a client-only, ephemeral flag keyed per (workspace,
session id): `unseenIdleByWorkspace[workspaceId][sessionId]`. Workspace
ownership lives in the key, so it survives hydration of a different
workspace's session list.

| State | Transition | Trigger | Actor |
|---|---|---|---|
| unmarked | marked | `session.state_changed` settles an active session (STARTING/RUNNING → IDLE/WAITING_FOR_INPUT/COMPLETED/FAILED/CANCELLED) or `session.turn.completed` arrives, while the dialog is closed, for a session known to `quickChat.sessions` (workspace taken from that tab entry) | WS handler |
| marked | unmarked | dialog opens (`openQuickChat`) — all workspaces | user |
| marked | unmarked | session becomes the active dialog tab | user |
| marked | unmarked | session tab closed or backing task deleted | user / sync |
| marked | unmarked | revision-guarded server resync removes the session from its workspace's list | resync |

## Failure modes

- **WS disconnected**: `session.state_changed` / `session.turn.completed` are
  live broadcasts; a turn that completes while the socket is down is never
  delivered and no dot is raised. There is no backfill. The dot is a
  best-effort cue, not a reliable delivery ledger.
- **Race: event before tab**: a settle event for a session whose tab is not
  yet in `quickChat.sessions` (e.g. the `task.created` tab arrives after the
  session already settled) does not raise a dot. The user never saw that chat
  window, so no dot is the conservative outcome.
- **Re-delivery**: a re-delivered `session.turn.completed` (reconnect replay,
  duplicate broadcast) for a turn already recorded as completed does not raise
  a dot — not after the tab arrives, and not after the dialog was opened and
  cleared the marker. Only a genuinely new turn completion raises the dot.
  For `session.state_changed` the same guarantee holds within the settle
  ledger's retention: the generation (`updated_at`) is kept per session for a
  60-minute window (max 500 sessions); a duplicate delivered after its
  generation was evicted (aged out or capped out) MAY raise a dot — accepted
  bounded-memory tradeoff, since practical replay windows are seconds; the dot
  self-heals on the next dialog open.
- **Abandoned turns**: `AbandonOpenTurns` (session resume/startup cleanup,
  rejected-message cleanup) publishes `session.turn.completed` for orphan
  turns whose `completed_at` equals `started_at` — the backend already
  recognizes this shape (`isAbandonedTurnCompletion` in
  `apps/backend/internal/backendapp/gateway.go`), so a discriminator EXISTS in
  the payload. The marker treats these uniformly with live completions and
  deliberately does NOT filter on it: the session did settle to idle, and the
  `session.state_changed` settle transition raises the same signal, so
  excluding abandoned closures would not prevent the dot anyway. This is an
  intentional product decision, pinned by a test; filtering is out of scope.
- **Receipt-time semantics**: the marker reflects events handled while the
  dialog is closed, not the instant the turn completed. `session.state_changed`
  and `session.turn.completed` travel over separate event-bus subscriptions
  and the gateway broadcasts both to every connected client (the payloads
  carry no `workspace_id`, so the workspace-broadcast path falls back to
  global delivery); per-workspace dots rely on session-tab membership and the
  selector, not transport scoping. Their relative order is not guaranteed; a
  completion notification handled while the dialog is open never marks, and
  one handled after the dialog closed marks, even if the turn itself finished
  earlier. The false-positive window is one notification round-trip and
  self-heals on the next dialog open.

## Scenarios

- **GIVEN** no quick chat session in the active workspace, **WHEN** the app
  renders, **THEN** no Quick Chat entry icon shows a dot.
- **GIVEN** a quick chat session with a running turn and the dialog closed,
  **WHEN** the turn completes (the session settles to an idle state), **THEN**
  a red dot appears on every Quick Chat entry icon of that workspace.
- **GIVEN** a quick chat session with a running turn, **WHEN** the turn
  completes while the dialog is open, **THEN** no dot appears for that turn.
- **GIVEN** a dot showing, **WHEN** the user opens the Quick Chat dialog,
  **THEN** the dot disappears.
- **GIVEN** a dot cleared by opening the dialog, **WHEN** the same session
  later completes another turn while the dialog is closed, **THEN** the dot
  reappears.
- **GIVEN** an unseen idle session in workspace A and none in workspace B,
  **WHEN** the user switches to workspace B, **THEN** workspace B's entry
  icons show no dot.
- **GIVEN** a dot contributed by one session, **WHEN** that session's tab is
  closed or its backing task is deleted, **THEN** the dot disappears.
- **GIVEN** a quick chat session tab that has never completed a turn,
  **WHEN** the page reloads, **THEN** no dot shows (markers are ephemeral and
  not persisted).

## Out of scope

- No persistence: markers reset on page reload (scenario above).
- No new copy, tooltips, sounds, or banner notifications.
- No dot on the command-palette Quick Chat item or the config-chat floating
  panel.
- No backend changes: the existing global broadcasts of
  `session.state_changed` and `session.turn.completed` are the only signals.
- No change to quick chat session creation, tab sync, or dialog behavior.
