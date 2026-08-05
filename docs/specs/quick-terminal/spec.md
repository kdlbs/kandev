---
status: shipped
created: 2026-08-03
updated: 2026-08-05
owner: kandev
---

# Quick Chat and Terminal Tabs

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
  duplicated in the creation menu.
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

Persisted ordinary and configuration chat sessions keep their existing task/session-backed model.
The shared frontend tab state additionally holds browser-local terminal descriptors:

| Field | Type | Meaning |
| --- | --- | --- |
| `tabId` | UUID string | Stable client identity and host-shell idempotency key for one terminal tab. |
| `workspaceId` | UUID string | Workspace whose shared dialog owns the tab. |
| `sessionId` | string, optional | Backend PTY session after startup succeeds. |
| `sequence` | positive integer | Workspace-local display order and default terminal label. |
| `status` | `starting \| running \| exited \| error` | Last observable lifecycle state. |
| `exitCode` | integer, optional | Exit code received while the client was attached. |
| `error` | string, optional | Last start or stream failure displayed in the tab. |

The frontend tracks the last active chat and last active terminal separately so each launcher can
return to its own most recent tab. Terminal descriptors never enter the backend boot payload,
SQLite, task records, or Quick Chat cross-device reconciliation.

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

## State machine

| State | Trigger | Result |
| --- | --- | --- |
| No terminal tab | Quick Terminal launcher | Create and activate one starting terminal tab. |
| Existing terminal tabs | Quick Terminal launcher | Open the dialog on the most recently activated terminal tab. |
| Any shared-dialog state | **New Terminal** | Append and activate a new starting terminal tab. |
| Starting | Host-shell start succeeds | Store the returned session ID, attach its stream, and mark the tab running. |
| Running | Chat/terminal tab switch or dialog dismissal | Detach the rendered terminal; keep the backend PTY and tab descriptor alive. |
| Detached | Tab selected or terminal launcher used | Reattach to the same session and replay available buffered output. |
| Running or detached | PTY exits | Mark the tab exited when observed; retain it for inspection. |
| Starting, running, exited, or error | Terminal tab close | Stop the session when one exists, remove the tab, select the nearest remaining same-workspace tab, or close the dialog when none remains. |

## Permissions

Quick terminals retain the existing host-shell permissions and environment of the Kandev backend
process. Workspace association controls frontend visibility only; it does not sandbox the shell or
grant access to a task worktree. Existing API authentication, WebSocket origin checks, and
Agents-page authorization behavior remain unchanged.

## Failure modes

- A failed start or stream leaves the terminal tab visible with its existing error presentation and
  a usable close action. It does not activate or stop sibling tabs.
- Closing a terminal tab while its start request is pending removes the tab and stops the session if
  that request later succeeds. Development StrictMode replay uses the stable client ID and cannot
  create or stop a sibling session.
- A detached session may exit or reach the existing backend idle/hard timeout. Reattaching to a
  session that no longer exists marks the tab exited or unavailable; it does not create a
  replacement implicitly.
- A successful stop or an already-missing session removes the tab. Any other stop failure is
  surfaced and keeps the tab available so the user can retry.
- Server Quick Chat reconciliation may add or remove persisted conversation tabs, but it must not
  discard browser-local terminal tabs or change the active terminal.
- Restoring focus after dialog dismissal must not reopen the sidebar terminal tooltip; pointer hover
  continues to show it.

## Persistence guarantees

- Ordinary and configuration chat persistence, cross-device synchronization, renaming, and
  expiration remain unchanged.
- Terminal tab descriptors live only in the current browser page's frontend store. They do not
  survive a page reload, browser restart, or another device.
- Host-shell processes and their rolling output buffers live only in backend memory. Dismissing the
  shared dialog does not stop them, but explicit tab close, process exit, the existing idle/hard
  timeout, or a backend restart does.
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

- Persisting or synchronizing terminal tabs, terminal names, or terminal output across reloads,
  backend restarts, browser tabs, or devices.
- Task-workspace terminals, repository working-directory selection, or moving terminals between
  workspaces.
- Terminal tab renaming, drag reordering, split panes, or a command-palette action.
- Changing Quick Chat task persistence, configuration-chat uniqueness, or ordinary-chat expiration.
- Changing the Agents-page terminal geometry or its stop-on-close lifecycle.

## Implementation plan

[Quick Chat and Terminal Tabs implementation](../../plans/quick-terminal/plan.md)
