# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.

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
- Receives task start/stop/resume requests via WebSocket
- Delegates to lifecycle manager for agent operations
- Handles event-driven state transitions
- Located in `internal/orchestrator/`

**Lifecycle Manager** (`internal/agent/lifecycle/`) manages agent instances:
- `Manager` - central coordinator for agent lifecycle
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

**Executor Types** (database model):
- `local_pc` - Standalone process on host ✅
- `local_docker` - Docker container on host ✅
- `remote_docker`, `remote_vps`, `k8s` - Planned

**Runtime Interface** implements execution:
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
Client (WS)     Orchestrator        Lifecycle Manager       Runtime          agentctl
    |                |                      |                  |                |
    | task.start     |                      |                  |                |
    |--------------->| LaunchAgent()        |                  |                |
    |                |--------------------->| CreateInstance() |                |
    |                |                      |----------------->| (container)    |
    |                |                      |                  |--------------->|
    |                | StartAgentProcess()  |                  |                |
    |                |--------------------->| ConfigureAgent() |                |
    |                |                      |----------------------------->     |
    |                |                      | Start()          |                |
    |                |                      |----------------------------->     |
    |                |                      |                  |  agent proc    |
    |                |                      |<---- stream updates (WS) ---------|
    |<--- WS events -|                      |                  |                |
```

**Session Resume:**
- `TaskSession.ACPSessionID` - stored ACP session ID for resume
- `ExecutorRunning` - tracks active executor state (container ID, port, worktree)
- On backend restart: `RecoverInstances()` finds running containers, reconnects streams

**Provider Pattern:** Packages expose `Provide(cfg, log) (*impl, cleanup, error)` for DI. Returns implementation, cleanup function, and error. Cleanup called during graceful shutdown.

**Worktrees:** `internal/worktree/Manager` provides workspace isolation. Each session can have its own worktree (branch) to prevent conflicts between concurrent agents.

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

### UI Components

**Shadcn Components:** Import from `@kandev/ui` package:
```typescript
import { Badge } from '@kandev/ui/badge';
import { Button } from '@kandev/ui/button';
import { Dialog } from '@kandev/ui/dialog';
// etc...
```

**Do NOT** import from `@/components/ui/*` - always use `@kandev/ui` package.

### Data Flow Pattern (Critical)

```
SSR Fetch -> Hydrate Store -> Components Read Store -> Hooks Subscribe
```

**Never fetch data directly in components.**

### Store Structure (Domain Slices)

```
lib/state/
├── store.ts (~370 lines)          # Root composition
├── slices/                         # Domain slices
│   ├── kanban/                    # boards, tasks, columns
│   ├── session/                   # sessions, messages, turns, worktrees
│   ├── session-runtime/           # shell, processes, git, context
│   ├── workspace/                 # workspaces, repos, branches
│   ├── settings/                  # executors, agents, editors, prompts
│   └── ui/                        # preview, connection, active state
├── hydration/                     # SSR merge strategies
└── selectors/                     # Memoized selectors (future)

hooks/domains/{kanban,session,workspace,settings}/  # Domain-organized hooks
lib/api/domains/{kanban,session,workspace,settings,process}-api.ts  # API clients
```

**Key State Paths:**
- `messages.bySession[sessionId]`, `shell.outputs[sessionId]`, `gitStatus.bySessionId[sessionId]`
- `tasks.activeTaskId`, `tasks.activeSessionId`, `workspaces.activeId`
- `repositories.byWorkspace`, `repositoryBranches.byRepository`

**Hydration:** `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions.

**Hooks Pattern:** Hooks in `hooks/domains/` encapsulate WS subscription + store selection. WS client deduplicates subscriptions automatically.

### WS

**Format:** `{id, type, action, payload, timestamp}`. See `docs/asyncapi.yaml` for all actions.

---

## Best Practices

### Backend
- Provider pattern for DI; stderr for logs, stdout for ACP only
- Pass context through chains; event bus for cross-component comm

### Frontend
- **Data:** SSR fetch → hydrate → read store. Never fetch in components
- **UI Components:** Import shadcn components from `@kandev/ui`, NOT `@/components/ui/*`
- **Components:** <200 lines, extract to domain components, composition over props
- **Hooks:** Domain-organized in `hooks/domains/`, encapsulate subscription + selection
- **WS:** Use subscription hooks only; client auto-deduplicates

---

**Last Updated**: 2026-01-22
