# Backend-Agentctl Connectivity

How the kandev backend communicates with agentctl instances (local, Docker, or remote).

## Overview

The backend maintains **3 independent connections** to each agentctl instance. They are not multiplexed over a single channel — each serves a distinct purpose with different traffic patterns.

```
                    ┌─────────────────────┐
                    │      agentctl       │
                    │  (container, local  │
                    │   process, or VM)   │
                    └──┬───┬───┬─────────┘
                       │   │   │
         HTTP ─────────┘   │   └──── Workspace WS
         (files, git,      │         (shell I/O, process
          vscode, procs)   │          output, git events)
                           │
                    Agent Updates WS
                    (ACP protocol:
                     prompts, responses,
                     tool calls)
```

---

## Connection 1: HTTP (Stateless, Request-Response)

**URL:** `http://host:port/api/v1/*`

Used for one-off operations that don't need streaming:

| Category | Endpoints |
|----------|-----------|
| Health | `GET /health` |
| Agent lifecycle | `POST /configure`, `POST /start`, `POST /stop` |
| Status | `GET /status` |
| File operations | `POST /workspace/file-tree`, `POST /workspace/file-content`, `POST /workspace/file-search`, `POST /workspace/file-create`, `POST /workspace/file-delete` |
| Git operations | `POST /workspace/git/stage`, `POST /workspace/git/commit`, `POST /workspace/git/push`, etc. |
| VS Code | `POST /vscode/start`, `POST /vscode/stop`, `GET /vscode/status` |
| Processes | `POST /process/start`, `POST /process/stop`, `GET /process/:id`, `GET /processes` |

Standard HTTP client with 60s timeout. Each call is independent.

**Key files:**
- `internal/agentctl/client/client.go` — HTTP client setup
- `internal/agentctl/client/client_files.go` — file operation methods
- `internal/agentctl/client/client_process.go` — process methods
- `internal/agentctl/client/git.go` — git methods

---

## Connection 2: Agent Updates WebSocket (Persistent)

**URL:** `ws://host:port/api/v1/agent/stream`

The ACP (Agent Control Protocol) channel. Carries the agent execution protocol — prompts, responses, tool calls, and streaming message chunks.

### Backend → Agentctl (Requests)

Requests are sent with a UUID and the caller blocks until a UUID-matched response arrives:

```go
// Simplified flow
sendStreamRequest(ctx, "agent.prompt", payload)
├── reqID = uuid.New()
├── pendingRequests[reqID] = make(chan *ws.Message)
├── conn.WriteMessage(JSON{id: reqID, action, payload})
└── <-pendingRequests[reqID]  // blocks until response
```

Request actions:
- `agent.initialize` — handshake
- `agent.session.new` — create new ACP session
- `agent.session.load` — resume existing session
- `agent.prompt` — send user message
- `agent.cancel` — cancel current operation
- `agent.permissions.respond` — respond to permission requests

### Agentctl → Backend (Events)

Async events streamed during agent execution:

| Event Type | Description |
|------------|-------------|
| `message_chunk` | Agent response text (streaming) |
| `reasoning` | Extended thinking output |
| `tool_call` | Agent invoked a tool |
| `tool_update` | Tool status changed |
| `complete` | Turn completed (success/error) |
| `permission_request` | Agent needs user permission |
| `context_window` | Token usage update |

**Key files:**
- `internal/agentctl/client/client_stream.go` — WebSocket RPC with request-ID tracking
- `internal/agentctl/client/agent.go` — ACP protocol methods (Initialize, NewSession, Prompt, etc.)
- `internal/agent/lifecycle/streams.go` — `StreamManager.connectUpdatesStream()`

---

## Connection 3: Workspace WebSocket (Persistent)

**URL:** `ws://host:port/api/v1/workspace/stream`

High-frequency streaming for workspace events. This is the channel through which shell PTY output, process output, git status changes, and file change notifications flow.

### Agentctl → Backend (Event Streaming)

| Message Type | Description | Frequency |
|-------------|-------------|-----------|
| `shell_output` | Shell PTY output | High (continuous) |
| `shell_exit` | Shell session ended | Low |
| `process_output` | Script process stdout/stderr | High |
| `process_status` | Process started/exited/failed | Low |
| `git_status` | Git working tree changed | Medium |
| `git_commit` | New commit made | Low |
| `git_reset` | HEAD reset | Low |
| `file_change` | File created/modified/deleted | Medium |
| `connected` | Stream ready | Once |

Events are dispatched via callbacks:

```go
stream, err := client.StreamWorkspace(ctx, WorkspaceStreamCallbacks{
    OnShellOutput:   func(data string) { ... },
    OnProcessOutput: func(output *ProcessOutput) { ... },
    OnGitStatus:     func(update *GitStatusUpdate) { ... },
    OnFileChange:    func(notification *FileChangeNotification) { ... },
})
```

### Backend → Agentctl (Commands)

```go
stream.WriteShellInput(data string)    // Send stdin to shell PTY
stream.ResizeShell(cols, rows int)     // Resize terminal
stream.Ping()                          // Keep-alive
```

**Key files:**
- `internal/agentctl/client/workspace_stream.go` — WebSocket connection + dispatch
- `internal/agentctl/client/workspace_dispatch.go` — message type routing
- `internal/agent/lifecycle/streams.go` — `StreamManager.connectWorkspaceStream()`

---

## Why Three Connections?

| Aspect | HTTP | Agent Updates WS | Workspace WS |
|--------|------|-----------------|--------------|
| **Lifetime** | Per-request (60s) | Persistent (session) | Persistent (session) |
| **Multiplexing** | Standard HTTP | Request ID (UUID) | Message type dispatch |
| **Traffic** | Low | Medium | High |
| **Pattern** | Request-response | Sync RPC + async events | Async streaming |

Separating them provides:

- **Isolation** — high-frequency shell output doesn't block ACP prompt/response cycles
- **Independent reconnection** — workspace stream failures don't kill the agent session
- **Protocol clarity** — agent stream is JSON-RPC with request tracking; workspace stream is pure event streaming

---

## Data Flow: Agentctl → Backend → Frontend

Events flow through the event bus to reach frontend WebSocket clients:

```
agentctl
  │
  ├─ Agent Updates WS ──→ StreamManager callback
  │                         └─→ Manager.handleMessageChunkEvent()
  │                             └─→ EventPublisher.PublishAgentStreamEvent()
  │                                 └─→ EventBus.Publish("agent/{sessionID}", ...)
  │
  └─ Workspace WS ──────→ StreamManager callback
                            └─→ Manager.handleShellOutput()
                                └─→ EventPublisher.PublishShellOutput()
                                    └─→ EventBus.Publish("session/{sessionID}/shell/output", ...)
                                                │
                                                ▼
                                    SessionStreamBroadcaster (gateway)
                                        └─→ hub.BroadcastToSession(sessionID, wsMsg)
                                            └─→ Frontend WebSocket clients
```

**Key files:**
- `internal/agent/lifecycle/events.go` — EventPublisher (publishes to event bus)
- `internal/agent/lifecycle/manager_events.go` — event handler callbacks
- `internal/gateway/websocket/` — event bus subscribers that forward to frontend

---

## Remote Executors (Sprites)

For remote executors like Sprites, the architecture doesn't change. The Sprites SDK sets up a TCP proxy tunnel:

```go
// Sprites proxy: maps a local port to the remote agentctl port
session, err := sprite.ProxyPort(ctx, localPort, remoteAgentctlPort)
```

From the backend's perspective, it's still `http://127.0.0.1:{localPort}` — all 3 connections go through the same tunnel transparently. The Sprites proxy handles TCP-level multiplexing.

**Key file:** `internal/agent/lifecycle/executor_sprites.go`

---

## Stream Lifecycle

### Startup

```go
StreamManager.ConnectAll(execution, readyCh)
├── go connectUpdatesStream(execution)   // Agent Updates WS
│   └── client.StreamUpdates(ctx, handler, mcpHandler, onDisconnect)
└── go connectWorkspaceStream(execution) // Workspace WS
    └── client.StreamWorkspace(ctx, callbacks)
```

Both streams connect in parallel. `readyCh` is closed when both are established.

### Reconnection

On disconnect, the workspace stream retries with exponential backoff (up to 5 attempts). On backend restart:

1. `RecoverInstances()` finds running containers/processes
2. Rebuilds `ExecutorInstance` with client pointing to same port
3. `ReconnectAll()` waits for agentctl health, then reconnects both streams
4. ACP session resumed via stored `ACPSessionID`

### Shutdown

```go
client.Close()
├── agentStreamConn.Close()
├── workspaceStreamConn.Close()
└── httpClient (GC'd)
```

**Key file:** `internal/agent/lifecycle/streams.go`
