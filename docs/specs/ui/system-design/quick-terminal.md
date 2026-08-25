---
status: draft
system: ui
requirements:
  - REQ-UI-QUICK-TERMINAL-001
created: 2026-08-03
updated: 2026-08-06
owners:
  - kandev
---
# Quick Chat and Terminal Tabs System Design

## Purpose and boundaries

This design preserves the technical source detail for `REQ-UI-QUICK-TERMINAL-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-QUICK-TERMINAL-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Quick Chat and Quick Terminal are both short-lived utilities reached from the same navigation
surfaces, but they currently open separate dialogs with different tab and lifecycle behavior. Users
should be able to keep several host terminals beside their utility conversations, switch between
them without losing work, and return to the most recent terminal without managing another window.

## What

- Quick Chat is the single responsive dialog for ordinary chats, configuration chats, and host
  terminal tabs. Quick Terminal no longer opens a separate dialog.
- Existing conversation launchers preserve their kind-specific behavior: generic Quick Chat
  shortcuts select an ordinary chat, configuration entry points select the workspace's
  configuration chat, and either opens its existing setup when no matching conversation exists.
- The existing Quick Terminal launchers use a reuse-or-create policy scoped to the active workspace:
  they open the most recently activated terminal tab when one exists, and create the first terminal
  tab otherwise.
- The tab-strip plus button opens a creation menu grouped like the task-detail Dockview add menu:
  an **Agents** section with **New Agent**, a separator, and a **Terminals** section with
  **New Terminal**. Existing tabs remain directly selectable in the tab strip rather than being
  duplicated in the creation menu. Because the plus button sits at the leading edge of the tab
  strip, its menu opens toward the trailing edge (aligned to the button's start) so it does not
  overhang the workspace edge.
- Choosing **New Agent** preserves the current ordinary/configuration setup flow. Choosing
  **New Terminal** always creates and activates a distinct host-shell terminal, even when another
  terminal exists.
- Chat and terminal tabs share one horizontal tab strip. Conversation ordering and configuration
  indicators remain unchanged; terminal tabs are ordered by creation and use a terminal icon with
  workspace-local labels such as `Terminal 1`, `Terminal 2`.
- Renameable conversation tabs expose **Rename** from a context menu on desktop right-click and
  the equivalent touch long-press gesture. The existing inline editor and backing-task rename
  persistence remain unchanged; terminal labels stay fixed.
- Multiple terminal tabs can run concurrently. Input, output, resize, exit, and error state belong
  to the selected terminal and must not affect sibling terminals.
- Switching to another tab or dismissing the shared dialog detaches the terminal presentation but
  leaves its host-shell session running. Reopening the dialog or reselecting the tab reconnects to
  that same session and replays the backend's available recent output.
- A quick-terminal host-shell session has no idle or wall-clock timeout. Once started it runs until
  the user closes its tab, the shell process exits, or the backend stops. A long-running or quiet
  process therefore keeps running while the dialog is closed and survives page reloads. This differs
  from the Agents-page agent-login sessions, which retain their existing idle and hard timeouts.
- Closing a terminal tab stops that tab's host-shell session and removes only that tab. Closing a
  chat tab retains the existing Quick Chat confirmation and task-deletion behavior.
- After the active tab is removed, the dialog selects the nearest remaining tab in the same
  rendered tab order; if the workspace has no remaining chat, setup, or terminal tab, the dialog
  closes.
- A terminal that exits naturally remains as an exited tab until the user closes it. The terminal
  shortcut reopens that most recent tab rather than silently replacing it; **New Terminal** is the
  explicit way to start another shell.
- Terminal tabs are associated with the workspace from which they were created and are visible,
  reusable, and selectable only while that workspace is active. They still run on the Kandev host
  and do not acquire a task workspace or repository working directory.
- The expanded desktop sidebar and the tablet/phone Home and Tasks headers retain separate Quick
  Terminal and Quick Chat buttons. Both open the shared dialog, select their respective content
  kind, and return focus to the launcher when the dialog closes.
- Tablet and desktop use the existing large Quick Chat floating geometry. Phone uses its existing
  full-height, dynamic-viewport surface. The tab strip and actions stay fixed while the selected
  terminal owns the remaining content region.
- The terminal on **Settings → Agents** retains its existing single-dialog presentation and
  stop-on-close behavior.

## Data model

Decision: [ADR-2026-08-05-server-owned-quick-terminal-descriptors](../../../decisions/2026-08-05-server-owned-quick-terminal-descriptors.md)

Persisted ordinary and configuration chat sessions keep their existing task/session-backed model.
The shared frontend tab state additionally holds backend-owned terminal descriptors:

| Field | Type | Meaning |
| --- | --- | --- |
| `tabId` | UUID string | Stable client identity and host-shell idempotency key for one terminal tab. |
| `workspaceId` | UUID string | Workspace whose shared dialog owns the tab. |
| `sessionId` | string, optional | Backend PTY session after startup succeeds. |
| `sequence` | positive integer | Workspace-local display order and default terminal label. |
| `status` | `connecting \| running \| exited \| error` | Last observable lifecycle state. |
| `exitCode` | integer, optional | Exit code received while the client was attached. |
| `error` | string, optional | Last start or stream failure displayed in the tab. |

The frontend tracks the last active chat and last active terminal separately so each launcher can
return to its own most recent tab. Terminal descriptors are backend-owned records scoped to the
authenticated user and workspace. They are returned in the Quick Chat boot payload and terminal
resync responses, but never enter task records, conversation rename/delete APIs, or conversation
membership reconciliation. The durable descriptor contains no terminal output and does not make
the PTY itself durable.

The descriptor store uses the following identity and lifecycle rules:

- `tabId` is a UUID generated by the browser and is the idempotency key for one logical terminal
  tab. The backend rejects malformed or cross-user updates and never treats it as a shell command,
  path, environment variable, or public agent identity.
- `sequence` is allocated by the backend per user and workspace. It is never reused after a tab is
  deleted, so labels remain stable across refreshes and devices.
- `sessionId` is the latest live host-shell session association. It is cleared when the backend
  can prove that the in-memory PTY no longer exists; a missing session never causes an implicit
  replacement.
- `status`, `exitCode`, and `error` are the latest bounded lifecycle snapshot. A live manager entry
  is authoritative for `running`; persisted status is used for errors and observed exits.

## API surface

`POST /api/v1/host-shell/start` accepts:

```json
{
  "cols": 120,
  "rows": 36,
  "client_id": "terminal-tab-uuid"
}
```

- `client_id` is an optional UUID. Without it, the endpoint retains the current
  singleton/idempotent behavior used by the Agents-page terminal; a present non-UUID value returns
  `400 Bad Request`.
- A Quick Terminal tab sends its stable `tabId` as `client_id`. Repeating a start for the same
  client ID returns the same running session; a different client ID creates an independent session.
- The response remains the existing host-shell session snapshot with `session_id`, `agent_id`,
  `cmd`, `running`, `started_at`, and optional exit fields.
- Stream, status, resize, and stop continue to use
  `/api/v1/agent-login/sessions/:sessionID/*`; their payloads do not change.

Quick Terminal descriptors use a separate authenticated API:

- `GET /api/v1/quick-terminal-tabs?workspace_id=<workspaceID>` returns the current user's
  descriptors for that workspace, ordered by `sequence`. The boot-state builder uses the same
  service so an initial page load and a later resync apply identical reconciliation rules.
- `POST /api/v1/quick-terminal-tabs` accepts `{ "tab_id": "<uuid>", "workspace_id": "<uuid>" }`,
  validates workspace visibility, allocates the next sequence, and is idempotent for the same
  user/tab ID.
- `PATCH /api/v1/quick-terminal-tabs/:tabID` records the latest session association and bounded
  lifecycle fields. A session ID is accepted only when it belongs to the host-shell manager entry
  keyed by that tab ID.
- `DELETE /api/v1/quick-terminal-tabs/:tabID` authorizes the row, stops its live host-shell session
  when present, and removes the descriptor. Repeating a delete for a missing row is treated as
  already closed; a different stop or persistence failure leaves the descriptor available for
  retry.

These descriptor routes are distinct from the low-level host-shell and agent-login routes so the
legacy singleton login surface retains its existing behavior and authorization contract.

## State machine

| State | Trigger | Result |
| --- | --- | --- |
| No terminal tab | Quick Terminal launcher | Create and activate one starting terminal tab. |
| Connecting descriptor | Page reload or another client | Reconcile its tab ID with the host-shell manager; reattach if the PTY is live, otherwise mark it unavailable without starting a replacement. |
| Existing terminal tabs | Quick Terminal launcher | Open the dialog on the most recently activated terminal tab. |
| Any shared-dialog state | **New Terminal** | Append and activate a new starting terminal tab. |
| Connecting | Host-shell start succeeds | Store the returned session ID, attach its stream, and mark the tab running. |
| Running | Chat/terminal tab switch or dialog dismissal | Detach the rendered terminal; keep the backend PTY and tab descriptor alive. |
| Detached | Tab selected or terminal launcher used | Reattach to the same session and replay available buffered output. |
| Running or detached | Quiet or long-running work | The session has no idle or hard timeout; it stays running across tab switches, dialog dismissal, and page reloads until closed, exited, or the backend stops. |
| Running or detached | PTY exits | Mark the tab exited when observed; retain it for inspection. |
| Connecting, running, exited, or error | Terminal tab close | Stop the session when one exists, remove the tab, select the nearest remaining same-workspace tab, or close the dialog when none remains. |

## Permissions

Quick terminals retain the existing host-shell permissions and environment of the Kandev backend
process. Workspace association controls descriptor ownership and frontend visibility; it does not
sandbox the shell or grant access to a task worktree. Existing API authentication, WebSocket origin
checks, and Agents-page authorization behavior remain unchanged.

## Failure modes

- A failed start or stream leaves the terminal tab visible with its existing error presentation and
  a usable close action. It does not activate or stop sibling tabs.
- Closing a terminal tab while its start request is pending removes the tab and stops the session if
  that request later succeeds. Development StrictMode replay uses the stable client ID and cannot
  create or stop a sibling session.
- A detached session may exit on its own (for example the user runs `exit` or the shell process
  crashes). It is not reaped by an idle or hard timeout. Reattaching to a session that no longer
  exists marks the tab exited or unavailable; it does not create a replacement implicitly.
- A descriptor whose PTY disappeared during a backend restart or while no client was attached is
  retained with an exited/unavailable status and a cleared session association. The user must
  choose **New Terminal** to create a replacement.
- A successful stop or an already-missing session removes the tab. Any other stop failure is
  surfaced and keeps the tab available so the user can retry.
- A descriptor create/update failure is visible on the affected tab and never causes a shell to be
  started without a durable tab record. Sibling tabs remain usable.
- Server Quick Chat reconciliation may add or remove persisted conversation tabs, but it must not
  discard server-owned terminal tabs or change the active terminal.
- Restoring focus after dialog dismissal must not reopen the sidebar terminal tooltip; pointer hover
  continues to show it.

## Persistence guarantees

- Ordinary and configuration chat persistence, cross-device synchronization, renaming, and
  expiration remain unchanged.
- Terminal tab descriptors survive page reloads, browser restarts, and access from another client
  for the same authenticated user and workspace. The backend is the durable source of descriptor
  membership, sequence, lifecycle snapshot, and latest session association.
- Host-shell processes and their rolling output buffers live only in backend memory. Dismissing the
  shared dialog does not stop them, and quick-terminal sessions have no idle or hard timeout, so
  only explicit tab close, the shell process exiting, or the backend stopping ends one. A backend
  restart leaves the descriptor behind and marks it unavailable until the user explicitly creates a
  new terminal. On graceful backend shutdown all live quick-terminal sessions are stopped rather
  than left to leak.
- Reconnection can replay only the backend's existing rolling output buffer; this feature does not
  add durable terminal history.

## Scenarios

- **GIVEN** no terminal tab exists in the active workspace, **WHEN** the user activates a Quick
  Terminal launcher, **THEN** the shared Quick Chat dialog opens with one starting terminal tab.
- **GIVEN** several terminal tabs exist, **WHEN** the user activates a Quick Terminal launcher,
  **THEN** the shared dialog selects the most recently activated terminal without creating another.
- **GIVEN** any chat or terminal tab is active, **WHEN** the user chooses **New Terminal** from the
  plus menu, **THEN** a distinct terminal tab and host-shell session are created and activated.
- **GIVEN** the plus menu is open, **WHEN** it renders, **THEN** it shows only **New Agent** under
  **Agents** and **New Terminal** under **Terminals**; existing tabs remain in the tab strip.
- **GIVEN** a renameable conversation tab is present, **WHEN** the user right-clicks it or
  long-presses it and chooses **Rename**, **THEN** the inline editor opens and a submitted name
  continues to persist through the existing conversation rename path.
- **GIVEN** two running terminal tabs, **WHEN** the user executes different marker commands in each
  and switches between them, **THEN** each tab displays only its own PTY output.
- **GIVEN** a running terminal tab, **WHEN** the user closes and later reopens the shared dialog,
  **THEN** the same tab reconnects to the same session and shows available recent output.
- **GIVEN** a running terminal tab with a persisted descriptor, **WHEN** the user refreshes the page,
  **THEN** the Quick Chat dialog restores one terminal tab with the same sequence and reattaches to
  the same live session without creating a duplicate PTY.
- **GIVEN** a running terminal tab whose process has produced no output and stayed detached for
  longer than the agent-login idle timeout, **WHEN** the user refreshes the page and reopens Quick
  Chat, **THEN** the tab reattaches to the same still-running session rather than showing exited and
  rather than starting a new numbered terminal.
- **GIVEN** the plus button at the leading edge of the tab strip, **WHEN** the user opens the
  creation menu on a pointer device, **THEN** the menu opens toward the trailing edge and stays
  within the viewport instead of overhanging the leading edge.
- **GIVEN** a persisted terminal descriptor whose PTY was lost during a backend restart, **WHEN** the
  user opens Quick Chat, **THEN** the tab remains visible as exited/unavailable and no replacement
  session starts until **New Terminal** is chosen.
- **GIVEN** the same authenticated user opens another client on the same workspace, **WHEN** its
  Quick Chat state loads, **THEN** it receives the persisted terminal descriptor and can reattach to
  the live session or see its authoritative exited/unavailable state.
- **GIVEN** a terminal and an ordinary chat both exist, **WHEN** the user alternates the Quick
  Terminal and generic Quick Chat launchers, **THEN** each launcher opens the most recently active
  matching tab without changing configuration-chat launcher behavior.
- **GIVEN** an active terminal tab with sibling tabs, **WHEN** the user closes it, **THEN** only its
  PTY is stopped and the nearest remaining same-workspace tab becomes active.
- **GIVEN** terminal tabs belong to two workspaces, **WHEN** the active workspace changes, **THEN**
  the shared dialog never displays or activates the other workspace's terminal tabs.
- **GIVEN** a phone viewport, **WHEN** the user opens the plus menu and creates a terminal, **THEN**
  the menu uses the existing touch-safe bottom-sheet treatment and the terminal fills the dialog's
  remaining dynamic-viewport region without document horizontal overflow.
- **GIVEN** terminal startup, reattachment, or stop fails, **WHEN** the failure settles, **THEN** the
  affected tab remains understandable and dismissible without disrupting chats or sibling terminals.
- **GIVEN** the user opens the terminal on **Settings → Agents**, **WHEN** that dialog closes,
  **THEN** its PTY still stops immediately and no Quick Chat terminal tab is created.
- **GIVEN** the shared dialog closes from a sidebar launcher, **WHEN** focus returns, **THEN** the
  launcher is focused without an automatically reopened tooltip.

## Out of scope

- Persisting terminal output or keeping a PTY process alive across a backend restart. The durable
  descriptor only makes the tab discoverable and reports that its old session is unavailable.
  Removing the idle/hard timeout keeps a quiet session alive while the backend runs; it does not
  make the process survive the backend stopping.
- Changing the Agents-page agent-login session lifecycle, which keeps its existing idle and hard
  timeouts.
- Synchronizing terminal input or output streams between multiple attached clients beyond the
  existing PTY subscription and rolling buffer behavior.
- Task-workspace terminals, repository working-directory selection, or moving terminals between
  workspaces.
- Terminal tab renaming, drag reordering, split panes, or a command-palette action.
- Changing Quick Chat task persistence, configuration-chat uniqueness, or ordinary-chat expiration.
- Changing the Agents-page terminal geometry or its stop-on-close lifecycle.

## Implementation plan

[Quick Chat and Terminal Tabs implementation](../../../plans/quick-terminal/plan.md)

[Quick Terminal refresh persistence repair](../../../plans/quick-terminal-refresh-persistence/plan.md)

[Quick Terminal durable session lifecycle and menu alignment](../../../plans/quick-terminal-durable-lifecycle/plan.md)
