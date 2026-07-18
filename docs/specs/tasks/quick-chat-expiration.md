---
status: shipped
created: 2026-07-02
owner: kandev
---

# Quick Chat Sessions, Persistence, and Expiration

## Why

Utility conversations need to remain available across page reloads without becoming detached
backend tasks. Configuration conversations need a compact Settings surface for in-context changes
and an explicit handoff to the larger Quick Chat dialog for reports and clarification questions,
while retaining the elevated tools that distinguish them from ordinary quick chats.

## What

- Quick Chat presents ordinary and configuration conversations as typed sessions in one session
  store and one tab model. A session has `kind: "chat" | "config"`.
- Configuration sessions are clearly identified with a sparkle/configuration indicator and an
  accessible label wherever their tab kind is presented.
- Settings Configuration Chat opens a floating configuration panel. The Command Palette
  Configuration Chat command opens the same typed setup/session in the Quick Chat modal.
- The Settings panel can transfer its current setup or session into the Quick Chat modal
  without creating a second task, losing messages, or replaying the initial prompt.
- Blank setup tabs use client-local workspace-and-kind-scoped identities, so ordinary and
  configuration setup can coexist without crossing workspace boundaries.
- Configuration setup retains its configuration-agent selection, introductory copy, suggestion
  prompts, and `Ask anything about your configuration...` prompt. It explains that the agent can
  manage workflows, agent profiles, and MCP configuration.
- Configuration setup never offers repository or branch selectors. Ordinary quick-chat setup
  retains the optional repository context defined by
  [Quick Chat Repository Context](quick-chat-repository-context.md).
- Creating an ordinary chat remains the default behavior when Quick Chat first opens. The tab-bar
  `+` action opens that setup directly. The setup always explains when to use Quick Chat instead of
  a tracked task and offers configuration mode in the form while the workspace has no existing or
  pending configuration session.
- A workspace currently presents one configuration session. Configuration entry points reopen that
  session when it exists, and the Settings floating panel shows it without a tab strip or new-session
  action. The larger Quick Chat dialog retains tabs so the configuration session can coexist with
  ordinary chats and be deleted through the established tab lifecycle.
- Desktop Settings uses the compact floating configuration panel until the user expands it. Mobile
  Settings uses a viewport-bounded floating panel and the existing full-screen Quick Chat layout
  after expansion, with the same creation, tab, clarification, and deletion capabilities.
- Pending clarification questions remain inline below a visible message history. The clarification
  region is scrollable, resizable on pointer-capable desktop and mobile devices, and collapsible so
  a user can recover conversation context without dismissing the question.
- Closing the modal or switching tabs preserves every real session. Closing a persisted tab asks
  for confirmation and deletes its backing ephemeral task through the existing task deletion path.
  Closing a blank setup tab only removes that client-side placeholder.
- When a setup start is superseded before its backend response arrives, the returned ephemeral task
  is deleted instead of reopening the abandoned tab.
- Restorable utility sessions are reconstructed from backend task/session state for the active
  workspace after a reload. They are available as tabs while the modal itself starts closed.
- Hydration resolves the workspace represented by the current route. On `/t/:id` and
  `/office/tasks/:id`, the task workspace takes precedence over a stale active-workspace setting.
- Ordinary quick chats expire after seven days of inactivity through the existing task deletion
  path. Configuration sessions do not use that idle expiration policy and remain until explicitly
  deleted or their workspace is deleted.
- Ordinary and configuration sessions can coexist without sharing setup state, initial prompts,
  repository choices, or workspace state.

Decision: [ADR-2026-07-14-typed-utility-chat-sessions](../../decisions/2026-07-14-typed-utility-chat-sessions.md). Repository-backed ordinary
chats additionally follow [ADR 0038](../../decisions/0038-quick-chat-repository-isolation.md).

## Data model

No database migration is required. Both session kinds use existing `tasks` and `task_sessions`
rows. A restorable utility-chat task satisfies all of these predicates:

```text
tasks.is_ephemeral = true
tasks.workflow_id = ""
tasks.origin != "automation_run"
tasks.archived_at IS NULL
primary task_session exists
```

The persisted discriminator is task metadata, not the task title:

```text
metadata.config_mode == true  -> kind = "config"
otherwise                     -> kind = "chat"
```

The unified frontend/boot session shape is:

```ts
type QuickChatSession = {
  kind: "chat" | "config";
  sessionId: string;
  workspaceId: string;
  name?: string;
  agentProfileId?: string;
};
```

For backward compatibility, a session object without `kind` is normalized to `kind: "chat"`.
Blank setup tabs use the same frontend shape with a generated workspace-and-kind-scoped
`sessionId`, but are never persisted or included in boot state.

Ordinary-chat last activity is the greater of `tasks.updated_at` and the newest associated
`task_sessions.updated_at`. An ordinary chat is eligible for deletion when that timestamp is more
than seven days old and none of its sessions is active. Configuration sessions are excluded from
this query by `metadata.config_mode`.

## API surface

The creation endpoints remain distinct because they grant different backend capabilities:

- `POST /api/v1/workspaces/:id/quick-chat` creates an ordinary ephemeral chat and retains the
  optional repository contract from [Quick Chat Repository Context](quick-chat-repository-context.md).
- `POST /api/v1/workspaces/:id/config-chat` creates an ephemeral task with
  `metadata.config_mode=true`, selects the requested/workspace configuration agent profile, and
  prepares a session with configuration MCP tools.
- Both return `{ "task_id": string, "session_id": string }`.

The SPA boot payload includes the active workspace's sessions and primary session DTOs:

```json
{
  "quickChat": {
    "isOpen": false,
    "sessions": [
      {
        "kind": "config",
        "sessionId": "session-id",
        "workspaceId": "workspace-id",
        "name": "Config Chat",
        "agentProfileId": "profile-id"
      }
    ],
    "activeSessionId": null
  },
  "taskSessions": {
    "items": {
      "session-id": { "id": "session-id", "task_id": "task-id" }
    }
  }
}
```

Sessions are sorted by last activity, newest first. Boot classification always derives `kind` from
`metadata.config_mode`; titles are display values only.

## State machine

| State                                     | Trigger                                     | Result                                                                                                                                                                                                                             |
| ----------------------------------------- | ------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Closed, restored sessions available       | Quick Chat or command action                | Modal opens on an eligible session or typed setup tab in the active workspace.                                                                                                                                                     |
| Settings configuration panel closed       | Settings FAB                                | Floating panel opens on an eligible configuration session or configuration setup.                                                                                                                                                  |
| Floating setup/session                    | Open in Quick Chat                          | Floating panel closes and the same setup/session becomes active in the large dialog without creating a task.                                                                                                                       |
| Existing configuration session            | Configuration entry point                   | Existing session opens; no second configuration setup is created.                                                                                                                                                                  |
| Blank `chat` setup                        | Start succeeds                              | Placeholder is replaced by a persisted `chat` session.                                                                                                                                                                             |
| Blank `config` setup                      | Prompt/profile submit succeeds              | The task session is seeded and the typed persisted tab opens. ACP prompts are delivered once after the chat subscription is ready; passthrough prompts are delivered once by the backend launch path before the terminal attaches. |
| Persisted session open                    | Switch tab or close modal                   | Session remains persisted and restorable.                                                                                                                                                                                          |
| Persisted tab close requested             | User confirms                               | Backing task is deleted, then the tab is removed or reconciled by task deletion state.                                                                                                                                             |
| Blank setup tab close requested           | User closes tab/modal                       | Placeholder is removed without a backend request.                                                                                                                                                                                  |
| Ordinary chat idle longer than seven days | Expiration sweep                            | Existing task deletion path removes task, sessions, workspace materialization, and tab.                                                                                                                                            |
| Configuration chat idle                   | Expiration sweep                            | Session is retained.                                                                                                                                                                                                               |

## Permissions

Configuration capabilities continue to be granted by the backend's config-mode task/session
preparation. A frontend `kind: "config"` label does not itself grant tools. Workspace scoping and
the existing task/session authorization rules apply to creation, restoration, continuation, and
deletion.

## Failure modes

- A failed ordinary or configuration start leaves its setup visible with an error. Any task that
  was created before launch failed is deleted through the existing rollback path.
- A non-passthrough configuration prompt is held only until its created session is available to
  `QuickChatContent`; it is sent once after subscription and is not replayed by tab switches or
  rerenders. A passthrough prompt is included in the config-chat start request because that session
  renders a terminal instead of `QuickChatContent`.
- A boot-state query failure omits utility sessions for that response and logs the error. It does
  not alter or delete persisted tasks.
- A task without a primary session is not restored as a tab.
- A tab-delete failure surfaces an error and leaves the backend task eligible for restoration; it
  must not be converted into a permanently hidden client-only config session.
- A workspace change closes or re-scopes the visible modal before sessions from another workspace
  can be activated. Launch and tab actions reject or ignore cross-workspace session state.
- An expiration candidate query failure deletes nothing. A single ordinary-chat delete failure is
  logged and does not stop later candidates from being evaluated.

## Persistence guarantees

- Persisted ordinary and configuration tasks, their primary sessions, messages, agent profile
  identity, and display names available from task/local name state survive browser reloads and
  backend restarts.
- The modal starts closed after boot. Opening it reveals only the restored sessions for the active
  workspace.
- Blank setup tabs, unsent setup input, and a collapsed clarification presentation state do not
  survive reloads.
- Ordinary quick chats remain subject to the seven-day inactivity deletion policy.
- Configuration chats are not automatically expired. Explicit tab deletion and workspace deletion
  use established task cleanup so they do not leave unrecoverable ephemeral tasks.

## Scenarios

- **GIVEN** an ordinary quick chat and a configuration chat in one workspace, **WHEN** the user
  opens Quick Chat, **THEN** both appear as tabs and the configuration tab has a visible and
  accessible configuration indicator.
- **GIVEN** a configuration agent is selected in Settings, **WHEN** the user starts Configuration
  Chat from the Settings FAB, **THEN** a floating configuration setup/session opens and the created
  task retains config-mode MCP tools.
- **GIVEN** the floating configuration setup is open, **WHEN** its empty state renders, **THEN** the
  panel header is the only Configuration Chat title, the composer action remains visible, and no
  separate cancel footer is rendered.
- **GIVEN** a floating configuration conversation, **WHEN** the user chooses Open in Quick Chat,
  **THEN** the floating panel closes and the large dialog shows the same session, history, task, and
  pending initial prompt.
- **GIVEN** the Quick Chat modal has no configuration session, **WHEN** the user starts a new chat,
  **THEN** its setup explains Quick Chat versus a tracked task and offers configuration mode inside
  the panel.
- **GIVEN** a workspace already has a configuration session, **WHEN** the user starts a new Quick
  Chat, **THEN** configuration mode is not offered and configuration entry points reopen the
  existing session.
- **GIVEN** Settings Configuration Chat is open, **WHEN** setup or the existing conversation is
  shown, **THEN** the floating panel has no session tabs or new-configuration-session action.
- **GIVEN** any application route, **WHEN** the user chooses Configuration Chat from the Command
  Palette, **THEN** the same unified configuration setup/session opens in Quick Chat.
- **GIVEN** a configuration setup tab, **WHEN** it renders, **THEN** it shows configuration copy,
  suggestions, the configuration profile choice, and no repository controls.
- **GIVEN** an ordinary setup tab, **WHEN** it renders, **THEN** optional repository/branch controls
  and the existing isolated repository behavior remain available.
- **GIVEN** a configuration prompt starts an ACP session, **WHEN** the frontend subscribes to the
  created task session, **THEN** the prompt is delivered exactly once and its response appears in
  that tab.
- **GIVEN** a passthrough configuration profile, **WHEN** its session starts, **THEN** the backend
  launch receives the initial prompt exactly once and the unified tab renders the passthrough
  terminal.
- **GIVEN** a completed configuration conversation, **WHEN** the browser reloads and Quick Chat is
  reopened, **THEN** the configuration tab and prior messages are restored and the conversation can
  continue.
- **GIVEN** Quick Chat is closed with a configuration tab active, **WHEN** the generic Quick Chat
  action reopens the dialog, **THEN** that configuration tab remains active regardless of chat kind.
- **GIVEN** restored sessions with different activity times, **WHEN** boot state is built, **THEN**
  eligible ordinary and configuration sessions are ordered newest first with the correct kind.
- **GIVEN** config-mode, automation-run, workflow-bound ephemeral, and ordinary quick-chat tasks,
  **WHEN** boot state is built, **THEN** only the config-mode and ordinary utility chats are restored.
- **GIVEN** sessions belonging to two workspaces, **WHEN** either workspace is active, **THEN** only
  that workspace's sessions can be shown or activated.
- **GIVEN** a task route whose workspace differs from the persisted active-workspace setting,
  **WHEN** the page reloads, **THEN** the task workspace's utility sessions are restored.
- **GIVEN** an agent clarification is pending, **WHEN** the Quick Chat tab is visible, **THEN** the
  question remains inline in a scrollable region, meaningful preceding message context remains
  visible, and the user can resize or collapse the question before answering it.
- **GIVEN** a narrow mobile viewport and a pending clarification, **WHEN** the user expands, scrolls,
  collapses, or answers it, **THEN** the primary question actions remain visible and usable without
  horizontal clipping.
- **GIVEN** a real ordinary or configuration session, **WHEN** the user switches tabs or closes the
  modal, **THEN** no backing task is deleted.
- **GIVEN** a real configuration tab, **WHEN** the user confirms its close action, **THEN** its
  backing ephemeral task is deleted through the Quick Chat deletion path and does not return after
  reload.
- **GIVEN** a blank typed setup tab, **WHEN** it is closed or the modal is dismissed, **THEN** only
  that placeholder is removed and no task deletion request is issued.
- **GIVEN** a quick-chat session object from an older boot/client shape without `kind`, **WHEN** it is
  hydrated, **THEN** it behaves as an ordinary `chat` session.
- **GIVEN** a configuration chat idle for more than seven days, **WHEN** the expiration sweep runs,
  **THEN** it is retained.
- **GIVEN** an ordinary quick chat idle for more than seven days with no active session, **WHEN** the
  expiration sweep runs, **THEN** its task, sessions, and workspace materialization are deleted.

## Out of scope

- Redesigning the agent protocol, configuration MCP tools, or config-chat creation endpoint.
- A new persistence table for UI tabs or duplicating task/session history in frontend storage.
- A separate configuration session store, message renderer, or backend conversation.
- A general task/chat navigation redesign.
- Multiple configuration sessions per workspace.
- Repository context for configuration sessions.
- A user-configurable ordinary-chat retention window or automatic expiration for config sessions.
