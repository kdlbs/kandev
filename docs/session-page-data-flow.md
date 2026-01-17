# Task Session Page Data Flow (SSR + WS + Store)

This document maps the current data flow for `/task/:id/:sessionId` and highlights which components/hooks depend on which data. It also calls out places where hooks could be made more granular or shared.

## SSR Entry Point

Route: `apps/web/app/task/[id]/[sessionId]/page.tsx`

Server fetches:
- `fetchTask(taskId)`
- `fetchBoardSnapshot(task.board_id)`
- `listAgents()`
- `listRepositories(task.workspace_id)`
- `listTaskSessions(task.id)` for each task in board
- `listTaskSessionMessages(sessionId)` (best-effort)

Hydration:
- `snapshotToState(snapshot)` → kanban columns + tasks
- `taskToState(task, sessionId, messages?)` → `tasks.activeTaskId`, `tasks.activeSessionId`, `messages.bySession[sessionId]`
- Repositories + agent profiles are merged into store
- Render `<StateHydrator initialState={...} />` and `<TaskPageClient ... />`

## Client Composition

```
TaskSessionPage (SSR)
  └─ TaskPageClient
      ├─ TaskTopBar
      └─ TaskLayout
          ├─ TaskSessionSidebar (left)
          ├─ TaskCenterPanel (chat/changes/notes)
          └─ TaskRightPanel (files + terminal)
```

## Active Selection (Store)

Store slice: `tasks.activeTaskId`, `tasks.activeSessionId`
- Set on initial mount by `TaskPageClient` from URL + SSR task.
- Updated when selecting a task/session in sidebar.

## WebSocket Data Flow

WebSocket client:
- `apps/web/lib/ws/client.ts` with deduplicated subscriptions.
- Components call `client.subscribe(taskId)`; dedup ensures single WS subscription.
- On reconnect, re-subscribes to active task channels.

Handlers (global router):
- `message.added` / `message.updated` → `messages.bySession[sessionId]`
- `task_session.state_changed` → `taskSessionStatesByTaskId` + `taskSessions.items`
- `git.status` → `gitStatus` (keyed by task)
- `shell.output` → `shell.outputs[taskId]`

## Component Data Dependencies

### TaskPageClient (`apps/web/app/task/[id]/page-client.tsx`)
- Props: `task`, `sessionId`, `initialSessionsByTask`, `initialRepositories`, `initialAgentProfiles`
- Store:
  - `tasks.activeTaskId`, `tasks.activeSessionId` via `setActiveSession`
  - `kanban.tasks`, `kanban.columns`, `workspaces.items`
  - `agentProfiles.items`
- Hooks:
  - `useSessionResumption(task?.id, sessionId)`
  - `useTaskAgent(task)` (agent state, task.execution polling)
  - `useRepositories(workspaceId)` (fallback to SSR data)
- WS:
  - Subscribes to all tasks in board (deduped)
  - Listens for `task_session.state_changed` to refresh `sessionsByTask`
- Responsibilities:
  - URL sync via `history.replaceState`
  - Task selection triggers `fetchTask` + `listTaskSessions`

### TaskSessionSidebar (`apps/web/components/task/task-session-sidebar.tsx`)
- Store:
  - `tasks.activeTaskId`
  - `tasks.activeSessionId`
- Props:
  - tasks + columns + sessionsByTask from `TaskPageClient`
- Uses:
  - `TaskSessionSwitcher` (sessions list)
  - `TaskSwitcher` (tasks list)
- Calls back to parent for selection and session list reload.

### TaskCenterPanel (`apps/web/components/task/task-center-panel.tsx`)
- Props: `taskId`, `onSendMessage`, `selectedDiffPath`, `openFileRequest`
- Includes:
  - `TaskChatPanel`
  - `TaskChangesPanel`
  - `FileViewerContent`

### TaskChatPanel (`apps/web/components/task/task-chat-panel.tsx`)
- Store:
  - `tasks.activeSessionId` (fallback unless `sessionId` prop passed)
- Hooks:
  - `useTaskSession(sessionId)` → taskId + session details
  - `useTaskMessages(taskId, sessionId)` → messages + meta
  - `useLazyLoadMessages(sessionId)` → pagination
- Data:
  - Renders from `messages.bySession[sessionId]`

### TaskRightPanel (`apps/web/components/task/task-right-panel.tsx`)
- Store:
  - `tasks.activeSessionId`
- Uses:
  - `ShellTerminal taskId + activeSessionId`

### ShellTerminal (`apps/web/components/task/shell-terminal.tsx`)
- Store:
  - `shell.outputs[taskId]`
  - `taskSessions.items[sessionId]?.state` or `taskSessionStatesByTaskId[taskId]`
- WS:
  - Subscribes to shell only when session is RUNNING or WAITING_FOR_INPUT.

### TaskFilesPanel / TaskChangesPanel
- Store:
  - `gitStatus` keyed by active task
- No direct WS subscriptions.

## ASCII Diagram (Current)

```
SSR
┌──────────────────────────────────────────────────────────────┐
│ /task/:id/:sessionId                                          │
│  fetchTask + snapshot + agents + repos + sessions + messages  │
│  StateHydrator -> store (activeTaskId + activeSessionId)      │
└──────────────────────────────────────────────────────────────┘

Client
┌─────────────────────────────── TaskPageClient ──────────────────────────────┐
│ setActiveSession(taskId, sessionId)                                          │
│ subscribe(board tasks)  + listen task_session.state_changed                  │
│                                                                              │
│  ┌─ TaskSessionSidebar (sessions + tasks)                                    │
│  ├─ TaskCenterPanel                                                         │
│  │   └─ TaskChatPanel                                                       │
│  │       ├─ useTaskSession(sessionId)                                       │
│  │       ├─ useTaskMessages(taskId, sessionId)                              │
│  │       └─ useLazyLoadMessages(sessionId)                                  │
│  └─ TaskRightPanel                                                          │
│      └─ ShellTerminal (shell.output + session state)                         │
└──────────────────────────────────────────────────────────────────────────────┘

WS
  task.subscribe -> deduped in client
  message.added/updated -> messages.bySession
  task_session.state_changed -> taskSessions + taskSessionStatesByTaskId
  git.status -> gitStatus (task)
  shell.output -> shell.outputs[task]
```

## Hooks Review (Granularity + Reuse)

Current hooks:
- `useTaskMessages(taskId, sessionId)`
  - Fetches + sets loading meta + subscribes to task updates.
  - Could be split into:
    - `useSessionMessages(sessionId)` (only fetch + store)
    - `useTaskSubscription(taskId)` (subscription only)
- `useLazyLoadMessages(sessionId)` uses store meta; OK as a focused pagination hook.
- `useTaskSession(sessionId)` derives taskId from messages if session not in store.
  - Can be simplified once session list is guaranteed in store.
- `useTaskAgent(task)` + `useSessionResumption(taskId, sessionId)`
  - Both access execution state. Consider a combined `useTaskExecution(taskId, sessionId)` hook.
- `useRepositories(workspaceId)` currently handles fetch + loading; OK but could be generalized (e.g. `useListResource`).

Suggested split for sharing:
- `useTaskSubscription(taskId)` → handles `client.subscribe(taskId)` only.
- `useSessionMessages(sessionId)` → fetch + store; no taskId.
- `useActiveSession()` selector → centralizes active session usage.
- `useTaskGitStatus(taskId)` → pure selector + optional subscribe.
- `useShellOutput(taskId)` → pure selector + optional subscribe.

This would allow panels to mix and match only what they need without pulling in extra behavior.
