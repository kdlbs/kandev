# Agent Orchestration + Session UI Data Flow

This document describes how a task session is created, how agentctl/agent lifecycle events flow through the backend, and how the session details page consumes those events.

## Overview (Actors)

- **Client UI**: Next.js app with session details page and panels (chat, terminal, git, files).
- **Backend**: Orchestrator + WebSocket gateway + task/session store.
- **agentctl**: Sidecar HTTP/WS server inside agent container (or standalone process) that manages the agent subprocess and exposes ACP + workspace streams.
- **Agent**: ACP-speaking process (JSON-RPC over stdin/stdout) that performs work.

## Orchestration Flow (Task Session Creation)

```
Client UI
  |  HTTP: POST /api/v1/tasks { start_agent: true, agent_profile_id }
  v
Backend Task Service
  |  create task + first session
  |  start agentctl orchestration
  |  create task_session (STARTING)
  |  launch agentctl (container or standalone)
  |  wait for /health
  |  POST /api/v1/start
  |  POST /api/v1/acp/call initialize
  |  POST /api/v1/acp/call session/new
  |  POST /api/v1/acp/call session/prompt
  v
agentctl
  |  spawns agent process
  |  streams ACP updates + workspace streams
  v
Agent
  |  ACP notifications (session/update)
  v
agentctl -> Backend
  |  ACP update stream -> backend
  |  backend updates task_session state + messages
  |  backend publishes WS notifications
  v
Client UI
  |  session.subscribe
  |  receive session-scoped events
```

## Backend Event Routing (Session Scoped)

Events are now routed primarily by **session_id**. The WS gateway maintains **session subscriber lists**. It broadcasts session events only to clients subscribed to that session.

```
agentctl WS /api/v1/acp/stream
  -> backend ACP handler
     -> task_session state update
     -> message persisted
     -> WS broadcast: session.channel

agentctl WS /api/v1/workspace/git-status/stream
  -> backend git-status handler
     -> attach to session
     -> WS broadcast: session.channel

agentctl WS /api/v1/output/stream
  -> backend shell handler
     -> session resolve (session_id -> task_id)
     -> WS broadcast: session.channel
```

Session notifications:
- `session.agentctl_starting`
- `session.agentctl_ready`
- `session.agentctl_error`
- `session.state_changed`
- `session.message.added`, `session.message.updated`
- `session.shell.output`, `session.shell.status`
- `session.git.status`
- `session.workspace.file.changes`

## Frontend Session Page Flow (SSR + WS)

```
Server (SSR)
  - fetch session by id
  - fetch parent task + workspace context
  - hydrate store (tasks, sessions, repos, agent profiles)

Client (Hydrated)
  - read sessionId from URL (/s/:sessionId)
  - set activeSessionId in store
  - subscribe to session events
  - child components read from store + hooks
```

### Component Subscriptions

Each component subscribes to the data it needs via hooks. The **WS client deduplicates subscriptions** so multiple components can safely request the same channel.

```
TaskSessionPage (/s/:sessionId)
  -> useSession(sessionId)
     -> subscribe: session.{sessionId}

  TaskChatPanel
    -> useSessionMessages(sessionId)
       -> subscribe: session.{sessionId}

  ShellTerminal
    -> useSessionAgentctl(sessionId)
       -> subscribe: session.{sessionId}
    -> useShellOutput(sessionId)
       -> subscribe: session.{sessionId}

  TaskChangesPanel (Git)
    -> useSessionGitStatus(sessionId)
       -> subscribe: session.{sessionId}

  TaskFilesPanel
    -> useSessionAgentctl(sessionId)
       -> wait for readiness
    -> useSessionWorktrees(sessionId)
       -> subscribe: session.{sessionId}
```

## Event + State Flow Diagram

```
Agent
  | ACP session/update
  v
agentctl
  | WS /api/v1/acp/stream
  v
Backend ACP handler
  | update session state/messages
  | emit session WS events
  v
WS Gateway (session subscribers)
  | broadcast to session.{sessionId}
  v
WebSocket Client (browser)
  | dedupe subscription
  | route to store slice
  v
Zustand Store
  | sessions, messages, shell, git, worktrees
  v
Session UI Panels
  | hook selectors
  | render
```

## Concrete Example: New Session -> Message Appears In Chat

Scenario: user starts a task session and the agent replies with a completion message that appears in the chat panel.

### Sequence (Calls + Events)

1) Client -> Backend (HTTP request)
```
POST /api/v1/tasks
body:
  title: "Example task"
  workspace_id: "workspace-1"
  repository_id: "repo-9"
  agent_profile_id: "profile-abc"
  start_agent: true
```

2) Backend (internal)
- creates task
- creates first task session
- starts agentctl orchestration for the new session
- returns session_id in the HTTP response

3) Backend -> agentctl (HTTP)
- `POST /api/v1/start`
- `POST /api/v1/acp/call` initialize
- `POST /api/v1/acp/call` session/new
- `POST /api/v1/acp/call` session/prompt

4) Backend -> Client (WS notification)
```
action: session.agentctl_starting
payload:
  task_id: "task-123"
  session_id: "session-789"
```

5) Client subscribes to session
```
action: session.subscribe
payload:
  session_id: "session-789"
```

6) agentctl -> Backend (ACP notification)
```
jsonrpc: "2.0"
method: "session/update"
params:
  type: "content"
  data:
    text: "Working on your request..."
    session_id: "session-789"
```

7) Backend -> Client (WS notification)
```
action: session.message.added
payload:
  session_id: "session-789"
  message:
    id: "msg-1"
    author_type: "agent"
    content: "Working on your request..."
```

8) Client store update (message handler)
```
messages.bySession["session-789"] += message
```

9) Chat panel render
```
TaskChatPanel -> useSessionMessages("session-789") -> messages list -> render
```

### Full Event List (Typical Session Start)

- HTTP request: `POST /api/v1/tasks` with `start_agent: true`
- WS notification: `session.agentctl_starting`
- WS request: `session.subscribe`
- WS notification: `session.agentctl_ready`
- ACP notifications from agent:
  - `session/update` (content/toolCall/complete/error)
- WS notifications:
  - `session.message.added`
  - `session.message.updated`
  - `session.state_changed`
  - `session.shell.status` / `session.shell.output` (if shell active)
  - `session.git.status` (if git watcher active)
  - `session.workspace.file.changes` (if watcher active)

## Notes / Guardrails

- Session is the source of truth; task id is derived from session when needed.
- No fallback to “active task” or “first message” for session ownership.
- Agentctl readiness is explicit; panels that require a live agent should wait for `agentctl_ready`.
- Store slices are keyed by session id (messages, shell output, git status, worktrees).
- WS subscriptions are **session-scoped** and **deduplicated** at the client level.
