# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.
>
> **Related**: [ARCHITECTURE.md](ARCHITECTURE.md), [docs/asyncapi.yaml](docs/asyncapi.yaml)

## Repo Layout

```
kandev-3/
├── apps/
│   ├── backend/          # Go backend (orchestrator, lifecycle, agentctl, WS gateway)
│   ├── web/              # Next.js frontend (SSR + WS + Zustand)
│   ├── packages/         # Shared packages/types
│   └── landing/          # Marketing site (shares UI with web)
```

## Tooling

- **Package manager**: `pnpm` workspace (root)
- **Backend**: Go with Make (`make -C apps/backend test|lint|build`)
- **Frontend**: Next.js (`pnpm -C apps/web dev|lint|typecheck`)
- **UI**: Shadcn components via `@kandev/ui`

---

## Backend Architecture

### Package Structure

```
apps/backend/
├── cmd/
│   ├── kandev/           # Main backend binary entry point
│   └── agentctl/         # Agentctl binary (runs inside containers or standalone)
├── internal/
│   ├── agent/
│   │   ├── lifecycle/    # Agent instance management (see below)
│   │   ├── registry/     # Agent type registry and defaults
│   │   ├── runtime/      # Runtime name constants
│   │   └── mcpconfig/    # MCP server configuration
│   ├── agentctl/
│   │   ├── client/       # HTTP client for talking to agentctl
│   │   └── server/       # agentctl HTTP server
│   │       ├── api/      # HTTP endpoints
│   │       ├── instance/ # Multi-instance management
│   │       ├── process/  # Agent subprocess management
│   │       └── adapter/  # Protocol adapters (ACP, Codex, REST, MCP)
│   ├── orchestrator/     # Task execution coordination
│   │   ├── executor/     # Launches agents via lifecycle manager
│   │   ├── scheduler/    # Task scheduling
│   │   ├── queue/        # Task queue
│   │   └── watcher/      # Event handlers
│   ├── task/
│   │   ├── models/       # Task, Session, Executor, Message models
│   │   ├── service/      # Task business logic
│   │   └── repository/   # Database access (SQLite)
│   ├── worktree/         # Git worktree management for workspace isolation
│   └── events/           # Event bus for internal pub/sub
```

### Key Concepts

**Orchestrator** coordinates task execution:
- Receives task start/stop/resume requests
- Delegates to lifecycle manager for agent operations
- Handles event-driven state transitions

**Lifecycle Manager** (`internal/agent/lifecycle/`) manages agent instances:
- `Manager` - central coordinator (~900 lines after refactor)
- `Runtime` interface - abstracts execution environment (Docker, Standalone)
- `ExecutionStore` - thread-safe in-memory execution tracking
- `SessionManager` - ACP session initialization and resume
- `StreamManager` - WebSocket stream connections to agentctl
- `EventPublisher` - publishes events to the event bus
- `ContainerManager` - Docker container operations

**agentctl** is an HTTP server that:
- Runs inside Docker containers or as standalone process
- Manages agent subprocess via stdin/stdout (ACP protocol)
- Exposes workspace operations (shell, git, files)
- Supports multiple concurrent instances on different ports

### Executor and Runtime

**Executor** (database model) defines where agents run:

| Type | Description | Status |
|------|-------------|--------|
| `local_pc` | Standalone process on host | Implemented |
| `local_docker` | Docker container on host | Implemented |
| `remote_docker` | Remote Docker daemon | Planned |
| `remote_vps` | Remote server via SSH | Planned |
| `k8s` | Kubernetes pods | Planned |

**Runtime** (interface) implements execution:
- `DockerRuntime` - creates containers, mounts workspace
- `StandaloneRuntime` - uses shared agentctl process

```go
type Runtime interface {
    Name() runtime.Name
    HealthCheck(ctx context.Context) error
    CreateInstance(ctx context.Context, req *RuntimeCreateRequest) (*RuntimeInstance, error)
    StopInstance(ctx context.Context, instance *RuntimeInstance, force bool) error
    RecoverInstances(ctx context.Context) ([]*RuntimeInstance, error)
}
```

### Execution Flow

```
Client (WS)        Orchestrator       Lifecycle Manager      Runtime           agentctl
     |                  |                    |                  |                  |
     | task.start       |                    |                  |                  |
     |----------------->| LaunchAgent()      |                  |                  |
     |                  |------------------->| CreateInstance() |                  |
     |                  |                    |----------------->| (container/proc) |
     |                  |                    |                  |----------------->|
     |                  | StartAgentProcess()|                  |                  |
     |                  |------------------->| ConfigureAgent() |                  |
     |                  |                    |---------------------------------->|
     |                  |                    | Start()          |                  |
     |                  |                    |---------------------------------->|
     |                  |                    |                  |    agent proc   |
     |                  |                    |<---- stream updates (WS) ---------|
     |<---- WS events --|                    |                  |                  |
```

### Session Resume

Sessions can be resumed after interruption:
- `TaskSession.ACPSessionID` - stored ACP session ID for resume
- `ExecutorRunning` - tracks active executor state (container ID, port, worktree)
- On backend restart: `RecoverInstances()` finds running containers, reconnects streams

### Provider Pattern

Packages expose `Provide(...)` functions for dependency injection:

```go
func Provide(cfg *config.Config, log *logger.Logger) (*impl, func() error, error)
```

- Returns concrete implementation, cleanup function, and error
- Cleanup called during graceful shutdown
- Keeps `cmd/kandev/main.go` thin

### Worktrees

Git worktrees provide workspace isolation:
- Each session can have its own worktree (branch)
- Prevents conflicts between concurrent agents
- Managed by `internal/worktree/Manager`

---

## agentctl Server

### HTTP Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check |
| `/api/v1/status` | GET | Agent status |
| `/api/v1/configure` | POST | Configure agent before start |
| `/api/v1/start` | POST | Start agent subprocess |
| `/api/v1/stop` | POST | Stop agent |
| `/api/v1/acp/*` | Various | ACP protocol (initialize, session/*, prompt) |
| `/api/v1/shell/*` | Various | Shell operations |
| `/api/v1/workspace/*` | Various | Git status, file changes |
| `/ws/updates` | WS | Agent event stream |
| `/ws/workspace` | WS | Workspace event stream (git, files, shell) |

### Adapter Model

Protocol adapters normalize different agent CLIs:

- `AgentAdapter` interface defines `Start()`, `Stop()`, `Prompt()`, `Cancel()`
- `ACPAdapter` - ACP JSON-RPC over stdio
- `CodexAdapter` - Codex-style JSON-RPC
- `process.Manager` owns subprocess, wires stdio to adapter

---

## ACP Protocol

JSON-RPC 2.0 over stdin/stdout between agentctl and agent process.

**Backend -> Agent (Requests)**
- `initialize` - handshake
- `session/new` - create new session
- `session/load` - resume existing session
- `session/prompt` - send user message
- `session/cancel` - cancel current operation

**Agent -> Backend (Notifications)**
- `session/update` with types: `message_chunk`, `tool_call`, `tool_update`, `complete`, `error`, `permission_request`, `context_window`

---

## Frontend Architecture

### Data Flow Pattern (Critical)

```
SSR Fetch -> Hydrate Store -> Components Read Store -> Hooks Subscribe
```

**This pattern must be followed. Do not fetch data directly in components.**

1. **SSR fetches** in layout/page server components
2. **Hydrate store** via `StateHydrator` component
3. **Components read from store only** (Zustand selectors)
4. **Hooks subscribe to WS channels** for real-time updates
5. **WS events update store** via event handlers

### Store Structure

```typescript
// Task/session data keyed by ID
messages.bySession[sessionId]
gitStatus[sessionId]
shell.outputs[sessionId]

// Active selection
tasks.activeTaskId
tasks.activeSessionId
```

### Custom Hooks Pattern

Hooks encapsulate subscription + store access:

```typescript
// Hook subscribes to WS channel, returns data from store
function useSessionMessages(sessionId: string) {
  useSessionSubscription(sessionId)  // Subscribe on mount, cleanup on unmount
  return useStore(state => state.messages.bySession[sessionId])
}
```

### WebSocket Subscription

- Components call subscription hooks (e.g., `useTaskSubscription(taskId)`)
- WS client **deduplicates** subscriptions with reference counting
- Reconnects automatically re-subscribe all active channels
- Event handlers route payloads to store slices

**Why This Matters**: Without this pattern, you get duplicate subscriptions, stale data, and race conditions.

### WS Message Format

```json
{
  "id": "uuid",
  "type": "request|response|notification|error",
  "action": "action.name",
  "payload": { ... },
  "timestamp": "2026-01-10T12:00:00Z"
}
```

See `docs/asyncapi.yaml` for all WS actions.

---

## Best Practices

### Backend
- Use provider pattern for dependency injection
- Keep agents logging to stderr; stdout only for ACP
- Pass context through call chains (but detach for background work)
- Use event bus for cross-component communication

### Frontend
- Never fetch in components; use SSR + store hydration
- Always use subscription hooks; never raw WS calls in components
- Keep components reading from store only
- Split large pages into smaller components

---

**Last Updated**: 2026-01-22
