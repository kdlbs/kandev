# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.

## Repo Layout

```
apps/
├── backend/          # Go backend (orchestrator, lifecycle, agentctl, WS gateway)
├── web/              # Next.js frontend (SSR + WS + Zustand)
├── cli/              # CLI tool (TypeScript)
└── packages/         # Shared packages/types
```

## Tooling

- **Package manager**: `pnpm` workspace (run from `apps/`, not repo root)
- **Backend**: Go with Make (`make -C apps/backend test|lint|build`)
- **Frontend**: Next.js (`cd apps && pnpm --filter @kandev/web dev|lint|typecheck`)
- **UI**: Shadcn components via `@kandev/ui`

---

## Backend Architecture

### Package Structure

```
apps/backend/
├── cmd/
│   ├── kandev/           # Main backend binary entry point
│   ├── agentctl/         # Agentctl binary (runs inside containers or standalone)
│   └── mock-agent/       # Mock agent for testing
├── internal/
│   ├── agent/
│   │   ├── lifecycle/    # Agent instance management (see below)
│   │   ├── agents/       # Agent type implementations
│   │   ├── controller/   # Agent control operations
│   │   ├── credentials/  # Agent credential management
│   │   ├── discovery/    # Agent discovery
│   │   ├── docker/       # Docker-specific agent logic
│   │   ├── dto/          # Agent data transfer objects
│   │   ├── handlers/     # Agent event handlers
│   │   ├── registry/     # Agent type registry and defaults
│   │   ├── runtime/      # Runtime name constants
│   │   ├── settings/     # Agent settings
│   │   ├── mcpconfig/    # MCP server configuration
│   │   └── remoteauth/   # Remote auth catalog and method IDs for remote executors/UI
│   ├── agentctl/
│   │   ├── client/       # HTTP client for talking to agentctl
│   │   └── server/       # agentctl HTTP server
│   │       ├── api/      # HTTP endpoints
│   │       ├── adapter/  # Protocol adapters (ACP, Codex, Copilot, Amp)
│   │       ├── instance/ # Multi-instance management
│   │       ├── mcp/      # MCP server integration
│   │       ├── process/  # Agent subprocess management
│   │       └── shell/    # Shell session management
│   ├── orchestrator/     # Task execution coordination
│   │   ├── executor/     # Launches agents via lifecycle manager
│   │   ├── handlers/     # Orchestrator event handlers
│   │   ├── messagequeue/ # Message queue for agent prompts
│   │   ├── queue/        # Task queue
│   │   ├── scheduler/    # Task scheduling
│   │   └── watcher/      # Event handlers
│   ├── task/
│   │   ├── controller/   # Task HTTP/WS controllers
│   │   ├── dto/          # Task data transfer objects
│   │   ├── events/       # Task event types
│   │   ├── handlers/     # Task event handlers
│   │   ├── models/       # Task, Session, Executor, Message models
│   │   ├── repository/   # Database access (SQLite)
│   │   └── service/      # Task business logic
│   ├── analytics/        # Usage analytics
│   ├── clarification/    # Agent clarification handling
│   ├── common/           # Shared utilities
│   ├── db/               # Database initialization
│   ├── debug/            # Debug tooling
│   ├── editors/          # Editor integration
│   ├── events/           # Event bus for internal pub/sub
│   ├── gateway/          # WebSocket gateway
│   ├── integration/      # External integrations
│   ├── lsp/              # LSP server
│   ├── mcp/              # MCP protocol support
│   ├── notifications/    # Notification system
│   ├── persistence/      # Persistence layer
│   ├── prompts/          # Prompt management
│   ├── sysprompt/        # System prompt injection
│   ├── user/             # User management
│   ├── workflow/         # Workflow engine
│   └── worktree/         # Git worktree management for workspace isolation
```

### Key Concepts

**Orchestrator** coordinates task execution:
- Receives task start/stop/resume requests via WebSocket
- Delegates to lifecycle manager for agent operations
- Handles event-driven state transitions
- Located in `internal/orchestrator/`

**Lifecycle Manager** (`internal/agent/lifecycle/`) manages agent instances:
- `Manager` (`manager.go`) - central coordinator for agent lifecycle
- `Runtime` interface (`runtime.go`) - abstracts execution environment (Docker, Standalone, Remote Docker)
- `ExecutionStore` (`execution_store.go`) - thread-safe in-memory execution tracking
- `session.go` - ACP session initialization and resume
- `streams.go` - WebSocket stream connections to agentctl
- `events.go` - publishes events to the event bus
- `container.go` - Docker container operations
- `process_runner.go` - agent process launch and management
- `profile_resolver.go` - resolves agent profiles/settings

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

**Executor default scripts:** Default prepare scripts are owned by executor lifecycle code (`internal/agent/lifecycle/default_scripts.go`), while `internal/scriptengine/` handles placeholder resolution and interpolation.

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
- `ACP` - ACP JSON-RPC over stdio (Claude Code)
- `Codex` - Codex-style JSON-RPC (OpenAI Codex)
- `CopilotAdapter` - GitHub Copilot SDK
- `AmpAdapter` - Sourcegraph Amp CLI
- `process.Manager` owns subprocess, wires stdio to adapter
- Factory pattern in `adapter/factory.go` selects adapter by agent type

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
├── store.ts                        # Root composition
├── slices/                         # Domain slices
│   ├── kanban/                    # boards, tasks, columns
│   ├── session/                   # sessions, messages, turns, worktrees
│   ├── session-runtime/           # shell, processes, git, context
│   ├── workspace/                 # workspaces, repos, branches
│   ├── settings/                  # executors, agents, editors, prompts
│   ├── diff-comments/             # code review diff comments
│   └── ui/                        # preview, connection, active state
├── hydration/                     # SSR merge strategies

hooks/domains/{kanban,session,workspace,settings}/  # Domain-organized hooks
lib/api/domains/                    # API clients
├── kanban-api, session-api, workspace-api, settings-api, process-api
├── plan-api, queue-api, workflow-api, stats-api
├── user-shell-api, debug-api
```

**Key State Paths:**
- `messages.bySession[sessionId]`, `shell.outputs[sessionId]`, `gitStatus.bySessionId[sessionId]`
- `tasks.activeTaskId`, `tasks.activeSessionId`, `workspaces.activeId`
- `repositories.byWorkspace`, `repositoryBranches.byRepository`

**Hydration:** `lib/state/hydration/merge-strategies.ts` has `deepMerge()`, `mergeSessionMap()`, `mergeLoadingState()` to avoid overwriting live client state. Pass `activeSessionId` to protect active sessions.

**Hooks Pattern:** Hooks in `hooks/domains/` encapsulate WS subscription + store selection. WS client deduplicates subscriptions automatically.

### WS

**Format:** `{id, type, action, payload, timestamp}`.

---

## Best Practices

### Code Quality (enforced by linters)

Static analysis runs in CI and pre-commit. New code **must** stay within these limits:

**Go** (`apps/backend/.golangci.yml` - errors on new code only):
- Functions: **≤80 lines**, **≤50 statements**
- Cyclomatic complexity: **≤15** · Cognitive complexity: **≤30**
- Nesting depth: **≤5** · Naked returns only in functions **≤30 lines**
- No duplicated blocks (**≥150 tokens**) · Repeated strings → constants (**≥3 occurrences**)

**TypeScript** (`apps/web/eslint.config.mjs` - warnings, will become errors):
- Files: **≤600 lines** · Functions: **≤100 lines**
- Cyclomatic complexity: **≤15** · Cognitive complexity: **≤20**
- Nesting depth: **≤4** · Parameters: **≤5**
- No duplicated strings (**≥4 occurrences**) · No identical functions · No unused imports
- No nested ternaries

**When you hit a limit:** extract a helper function, custom hook, or sub-component. Prefer composition over growing a single function.

### Backend
- Provider pattern for DI; stderr for logs, stdout for ACP only
- Pass context through chains; event bus for cross-component comm

### Frontend
- **Data:** SSR fetch → hydrate → read store. Never fetch in components
- **UI Components:**
  - Import shadcn components from `@kandev/ui`, NOT `@/components/ui/*`
  - **Always prefer native shadcn components** over custom implementations
  - Check `apps/packages/ui/src/` for available components (pagination, table, dialog, etc.)
  - For data tables, use `@kandev/ui/table` with TanStack Table; use shadcn Pagination components
  - Only create custom components when shadcn doesn't provide what's needed
- **Components:** <200 lines, extract to domain components, composition over props
- **Hooks:** Domain-organized in `hooks/domains/`, encapsulate subscription + selection
- **WS:** Use subscription hooks only; client auto-deduplicates
- **Interactivity:** All buttons and links with actions must have `cursor-pointer` class

### Plan Implementation
- After implementing a plan, run `make fmt` first to format code, then run `make typecheck test lint` to verify the changes. Formatting must come first because formatters may split lines, which can trigger complexity linter warnings.

---

## Maintaining This File

This file is read by AI coding agents (Claude Code via `CLAUDE.md` symlink, Codex via `AGENTS.md`). If your changes make any section of this file outdated or inaccurate - e.g., you add/remove/rename packages, change architectural patterns, add new adapters, modify store slices, or change conventions - **update the relevant sections of this file as part of the same PR**. Keep descriptions concise and factual. Do not add speculative or aspirational content.

---

**Last Updated**: 2026-02-22
