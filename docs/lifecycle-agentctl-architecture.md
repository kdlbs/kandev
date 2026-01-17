# Lifecycle Manager and Agentctl Architecture

This document describes the architecture, entities, and workflows involved in the agent lifecycle management system.

## Overview

The system consists of two main components:

1. **Lifecycle Manager** (`apps/backend/internal/agent/lifecycle/`) - Backend component that orchestrates agent execution lifecycles
2. **Agentctl** (`apps/backend/internal/agentctl/`) - Per-agent instance server providing workspace access and agent communication

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Backend                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      Lifecycle Manager                               │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │ExecutionStore│  │SessionManager│  │StreamManager │               │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘               │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │CommandBuilder│  │EventPublisher│  │   Runtime    │               │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘               │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                         │
│                                    │ HTTP/WebSocket                          │
│                                    ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                         Agentctl Instance                            │    │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐               │    │
│  │  │Process Manager│  │Shell Session │  │WorkspaceTracker│             │    │
│  │  └──────────────┘  └──────────────┘  └──────────────┘               │    │
│  │  ┌──────────────┐  ┌──────────────┐                                  │    │
│  │  │Protocol Adapter│ │ API Server  │                                  │    │
│  │  │ (ACP/Codex)   │  └──────────────┘                                  │    │
│  │  └──────────────┘                                                    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                    │                                         │
│                                    │ stdin/stdout (JSON-RPC 2.0)             │
│                                    ▼                                         │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                      Agent Subprocess                                │    │
│  │                    (auggie, codex, etc.)                             │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Lifecycle Manager Entities

### Manager (`manager.go`)

The central orchestrator for agent lifecycles. Coordinates all other components.

**Key Fields:**
- `runtime` - Runtime abstraction (Docker, Standalone, K8s)
- `executionStore` - Thread-safe execution tracking
- `commandBuilder` - Builds agent commands from registry config
- `sessionManager` - Handles ACP session initialization
- `streamManager` - Manages WebSocket streams to agents
- `eventPublisher` - Publishes lifecycle events to event bus

**Key Methods:**
- `Launch()` - Creates agentctl instance (agent not started)
- `StartAgentProcess()` - Starts the agent subprocess
- `StopAgent()` - Stops an agent execution
- `PromptAgent()` - Sends follow-up prompts
- `MarkReady()` / `MarkCompleted()` - Status transitions

### AgentExecution (`types.go`)

Represents a running agent instance.

**Key Fields:**
- `ID` - Unique execution identifier
- `TaskID` - Associated task
- `AgentProfileID` - Agent profile configuration
- `Status` - Current status (Running, Ready, Completed, Failed, Stopped)
- `ACPSessionID` - ACP session for resumption
- `agentctl` - Client for communicating with agentctl
- `ContainerID` / `ContainerIP` - Docker-specific (if applicable)
- `WorkspacePath` - Agent's working directory

### ExecutionStore (`execution_store.go`)

Thread-safe storage with multiple indexes for efficient lookup.

**Indexes:**
- By execution ID (primary)
- By task ID
- By container ID

### SessionManager (`session.go`)

Handles ACP protocol session initialization.

**Key Methods:**
- `InitializeSession()` - Sends initialize + session/new or session/load
- `InitializeAndPrompt()` - Full orchestration including initial prompt
- `SendPrompt()` - Sends prompts and waits for completion

### StreamManager (`streams.go`)

Manages WebSocket connections to agentctl for real-time updates.

**Streams Connected:**
- Updates stream (session updates from agent)
- Permission stream (permission requests)
- Git status stream
- File changes stream

### EventPublisher (`events.go`)

Publishes events to the event bus for WebSocket streaming to frontend.

**Event Types:**
- `agent.started`, `agent.stopped`, `agent.ready`, `agent.completed`, `agent.failed`
- `acp.session.created`
- `acp.message` (session updates)
- `prompt.complete`, `tool_call.started`, `tool_call.complete`
- `permission.request`
- `git.status.updated`, `file.change.notified`

### Runtime (`runtime.go`)

Interface abstracting the execution environment.

**Implementations:**
- Docker runtime
- Standalone runtime
- (Future: K8s, SSH)

**Key Methods:**
- `CreateInstance()` - Creates agentctl instance
- `StopInstance()` - Stops an instance
- `RecoverInstances()` - Recovers after backend restart

---

## Agentctl Entities

### Instance Manager (`server/instance/manager.go`)

Manages multiple agent instances, each with dedicated HTTP server.

**Key Methods:**
- `CreateInstance()` - Allocates port, creates process manager, starts HTTP server
- `StopInstance()` - Stops process, shuts down HTTP server, releases port
- `GetInstance()` / `ListInstances()`

### Instance (`server/instance/instance.go`)

Represents a single agent instance.

**Key Fields:**
- `ID`, `Port`, `Status`
- `WorkspacePath`, `AgentCommand`, `Env`
- `manager` - Process manager (unexported)
- `server` - HTTP server (unexported)

### Process Manager (`server/process/manager.go`)

Manages the agent subprocess lifecycle.

**Key Fields:**
- `cmd` - The exec.Cmd for the agent process
- `stdin`, `stdout`, `stderr` - Process pipes
- `adapter` - Protocol adapter (ACP or Codex)
- `workspaceTracker` - Git/file monitoring
- `shell` - Embedded shell session
- `pendingPermissions` - Permission requests awaiting response

**Key Methods:**
- `Start()` - Starts agent subprocess, creates adapter, starts shell
- `Stop()` - Graceful shutdown
- `Configure()` - Sets agent command before start
- `GetUpdates()` - Returns session update channel
- `RespondToPermission()` - Responds to permission requests

### Protocol Adapter (`server/adapter/adapter.go`)

Interface for agent communication protocols.

**Implementations:**
- `ACPAdapter` - ACP JSON-RPC 2.0 over stdin/stdout
- `CodexAdapter` - Codex-style JSON-RPC

**Key Methods:**
- `Initialize()` - Handshake with agent
- `NewSession()` / `LoadSession()` - Session management
- `Prompt()` - Send prompt to agent
- `Cancel()` - Cancel current operation
- `Updates()` - Channel for session updates

### WorkspaceTracker (`server/process/workspace_tracker.go`)

Monitors workspace for git status and file changes.

**Features:**
- Periodic git status polling
- File system watching (fsnotify)
- Subscriber pattern for updates

### Shell Session (`server/shell/session.go`)

PTY-based shell access for the workspace.

**Features:**
- Interactive shell via PTY
- Output streaming to subscribers
- Input writing
- Resize support

---

## Workflows

### 1. Agent Launch Flow

```
User Request → Orchestrator → Lifecycle Manager
                                    │
                                    ▼
                            1. Resolve agent profile
                            2. Get agent config from registry
                            3. Create worktree (if enabled)
                            4. Build environment variables
                                    │
                                    ▼
                            Runtime.CreateInstance()
                                    │
                                    ▼
                            Agentctl instance starts
                            (HTTP server on dedicated port)
                                    │
                                    ▼
                            Wait for agentctl ready
                            (shell/workspace access available)
                                    │
                                    ▼
                            Publish agent.started event
```

### 2. Agent Process Start Flow

```
StartAgentProcess(executionID)
            │
            ▼
    Wait for agentctl ready
            │
            ▼
    agentctl.ConfigureAgent(command, env)
            │
            ▼
    agentctl.Start()
            │
            ▼
    Process Manager starts subprocess
    Creates protocol adapter (ACP/Codex)
    Starts shell session
    Starts workspace tracker
            │
            ▼
    SessionManager.InitializeAndPrompt()
            │
            ├─► ACP initialize handshake
            ├─► session/new or session/load
            ├─► Connect WebSocket streams
            ├─► Send initial task prompt
            └─► Mark execution as READY
```

### 3. Prompt Flow

```
PromptAgent(executionID, prompt)
            │
            ▼
    Validate execution status (RUNNING or READY)
            │
            ▼
    Update status to RUNNING
            │
            ▼
    agentctl.Prompt(prompt)
            │
            ▼
    Agent processes prompt
    Streams updates via WebSocket
            │
            ▼
    Accumulate message content
    Handle tool calls
    Publish events to frontend
            │
            ▼
    Prompt completes
    Mark execution as READY
```

### 4. Session Update Flow

```
Agent Subprocess
      │
      │ stdout (JSON-RPC notifications)
      ▼
Protocol Adapter
      │
      │ Normalized SessionUpdate
      ▼
Process Manager.updatesCh
      │
      │ WebSocket stream
      ▼
Lifecycle Manager.StreamManager
      │
      │ Callback
      ▼
handleSessionUpdate()
      │
      ├─► Update progress
      ├─► Accumulate message content
      ├─► Handle tool calls
      └─► Publish to event bus
            │
            ▼
      Frontend via WebSocket
```

### 5. Permission Request Flow

```
Agent requests permission
      │
      │ JSON-RPC notification
      ▼
Protocol Adapter
      │
      │ PermissionHandler callback
      ▼
Process Manager.handlePermissionRequest()
      │
      ├─► If auto-approve: return immediately
      │
      └─► Store pending permission
          Send notification to backend
          Wait for response (with timeout)
                │
                ▼
          Frontend shows permission dialog
                │
                ▼
          User responds
                │
                ▼
          RespondToPermission(pendingID, optionID)
                │
                ▼
          Response sent to waiting goroutine
                │
                ▼
          Agent continues execution
```

### 6. Agent Stop Flow

```
StopAgent(executionID, force)
            │
            ▼
    Get execution from store
            │
            ▼
    If not force:
        agentctl.Stop() (graceful)
            │
            ▼
    Runtime.StopInstance()
            │
            ▼
    Update status to STOPPED
    Set FinishedAt timestamp
            │
            ▼
    Remove from ExecutionStore
            │
            ▼
    Publish agent.stopped event
```

---

## Communication Patterns

### Backend ↔ Agentctl

- **Protocol**: HTTP + WebSocket
- **Base URL**: `http://localhost:{port}` (port allocated per instance)

**HTTP Endpoints:**
- `GET /health` - Health check
- `GET /api/v1/status` - Process status
- `POST /api/v1/start` - Start agent process
- `POST /api/v1/stop` - Stop agent process
- `POST /api/v1/configure` - Configure agent command
- `POST /api/v1/acp/initialize` - ACP initialize
- `POST /api/v1/acp/session/new` - Create session
- `POST /api/v1/acp/session/load` - Load session
- `POST /api/v1/acp/prompt` - Send prompt
- `POST /api/v1/permissions/{id}/respond` - Respond to permission

**WebSocket Endpoints:**
- `/api/v1/acp/stream` - Session updates
- `/api/v1/permissions/stream` - Permission notifications
- `/api/v1/workspace/stream` - Unified workspace stream (shell, git, files)

### Agentctl ↔ Agent

- **Protocol**: ACP (Agent Communication Protocol)
- **Transport**: JSON-RPC 2.0 over stdin/stdout
- **Format**: Newline-delimited JSON

**Backend → Agent (Requests):**
- `initialize` - Handshake
- `session/new` - Create session
- `session/load` - Resume session
- `session/prompt` - Send prompt
- `session/cancel` - Cancel operation

**Agent → Backend (Notifications):**
- `session/update` - Progress, content, tool calls, completion, errors

---

## Status Transitions

```
                    ┌─────────────┐
                    │   RUNNING   │◄──────────────────┐
                    └──────┬──────┘                   │
                           │                          │
              ┌────────────┼────────────┐             │
              │            │            │             │
              ▼            ▼            ▼             │
        ┌─────────┐  ┌─────────┐  ┌─────────┐        │
        │  READY  │  │ FAILED  │  │ STOPPED │        │
        └────┬────┘  └─────────┘  └─────────┘        │
             │                                        │
             │ (follow-up prompt)                     │
             └────────────────────────────────────────┘
                           │
                           ▼
                    ┌───────────┐
                    │ COMPLETED │
                    └───────────┘
```

**Status Meanings:**
- `RUNNING` - Agent is processing a prompt
- `READY` - Agent completed a turn, waiting for next prompt
- `COMPLETED` - Agent finished all work successfully
- `FAILED` - Agent encountered an error
- `STOPPED` - Agent was manually stopped

---

## Key Design Decisions

1. **Two-Phase Launch**: `Launch()` creates agentctl instance for workspace access, `StartAgentProcess()` starts the actual agent. This allows shell/git access before agent starts.

2. **Protocol Adapter Pattern**: Abstracts agent communication protocol (ACP, Codex) behind a common interface, enabling support for different agent types.

3. **Runtime Abstraction**: Supports multiple execution environments (Docker, Standalone, K8s) through a common interface.

4. **Event-Driven Architecture**: All state changes published to event bus for real-time frontend updates.

5. **Session Resumption**: ACP session IDs stored for resuming conversations after backend restart.

6. **Worktree Isolation**: Git worktrees provide isolated workspaces per task, preventing conflicts.

