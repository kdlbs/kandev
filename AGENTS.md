# Kandev Engineering Guide

> **Purpose**: Architecture notes, key patterns, and conventions for LLM agents working on Kandev.

## Repo Layout

```
apps/
├── backend/          # Go backend (orchestrator, lifecycle, agentctl, WS gateway)
├── web/              # Next.js frontend (SSR + WS + Zustand)
├── cli/              # CLI tool (TypeScript)
├── landing/          # Landing page
└── packages/         # Shared packages/types
```

## Tooling

- **Package manager**: `pnpm` workspace (run from `apps/`, not repo root)
- **Backend**: Go with Make (`make -C apps/backend test|lint|build`)
- **Frontend**: Next.js (`cd apps && pnpm --filter @kandev/web dev|lint|typecheck`)
- **UI**: Shadcn components via `@kandev/ui`
- **GitHub repo**: `https://github.com/kdlbs/kandev`
- **Container image**: `ghcr.io/kdlbs/kandev` (GitHub Container Registry)

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
│   │   ├── executor/     # Executor types, checks, and service
│   │   ├── handlers/     # Agent event handlers
│   │   ├── registry/     # Agent type registry and defaults
│   │   ├── settings/     # Agent settings
│   │   ├── mcpconfig/    # MCP server configuration
│   │   └── remoteauth/   # Remote auth catalog and method IDs for remote executors/UI
│   ├── agentctl/
│   │   ├── client/       # HTTP client for talking to agentctl
│   │   └── server/       # agentctl HTTP server
│   │       ├── acp/      # ACP protocol implementation
│   │       ├── adapter/  # Protocol adapters + transport/ (ACP, Codex, OpenCode, Copilot, Amp)
│   │       ├── api/      # HTTP endpoints
│   │       ├── config/   # agentctl configuration
│   │       ├── instance/ # Multi-instance management
│   │       ├── mcp/      # MCP server integration
│   │       ├── process/  # Agent subprocess management
│   │       ├── shell/    # Shell session management
│   │       └── utility/  # agentctl utilities
│   ├── orchestrator/     # Task execution coordination
│   │   ├── dto/          # Orchestrator data transfer objects
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
│   ├── github/           # GitHub API integration (PRs, reviews, webhooks)
│   ├── integration/      # External integrations
│   ├── lsp/              # LSP server
│   ├── mcp/              # MCP protocol support
│   ├── health/           # Health check endpoints
│   ├── notifications/    # Notification system
│   ├── persistence/      # Persistence layer
│   ├── prompts/          # Prompt management
│   ├── repoclone/        # Repository cloning for remote executors
│   ├── scriptengine/     # Script placeholder resolution and interpolation
│   ├── secrets/          # Secret management
│   ├── sprites/          # Sprites AI integration
│   ├── sysprompt/        # System prompt injection
│   ├── task/
│   │   └── ...
│   ├── tools/            # Tool integrations
│   ├── user/             # User management
│   ├── utility/          # Shared utility functions
│   ├── workflow/         # Workflow engine
│   │   ├── engine/       # Typed state-machine engine (trigger evaluation, action callbacks, transition store)
│   │   ├── models/       # Workflow step, template, and history models
│   │   ├── repository/   # Workflow persistence (SQLite)
│   │   └── service/      # Workflow CRUD and step resolution
│   └── worktree/         # Git worktree management for workspace isolation
```

### Key Concepts

**Orchestrator** coordinates task execution:
- Receives task start/stop/resume requests via WebSocket
- Delegates to lifecycle manager for agent operations
- Handles event-driven state transitions via workflow engine
- Located in `internal/orchestrator/`

**Workflow Engine** (`internal/workflow/engine/`) provides typed state-machine evaluation:
- `Engine.HandleTrigger()` evaluates step actions for triggers (on_enter, on_turn_start, on_turn_complete, on_exit)
- `TransitionStore` interface abstracts persistence (implemented by `orchestrator.workflowStore`)
- `CallbackRegistry` maps action kinds to callbacks (plan mode, auto-start, context reset)
- First-transition-wins: multiple transition actions in one trigger, first eligible wins
- `EvaluateOnly` mode: engine evaluates without persisting, caller orchestrates on_exit → DB → on_enter
- `RequiresApproval` on actions: transitions requiring review gating are skipped
- Idempotent by `OperationID`; session-scoped data bag via `MachineState.Data`

**Lifecycle Manager** (`internal/agent/lifecycle/`) manages agent instances:
- `Manager` (`manager.go`, `manager_*.go`) - central coordinator for agent lifecycle
- `ExecutorBackend` interface (`executor_backend.go`) - abstracts execution environment (Docker, Standalone, Sprites, Remote Docker)
- `ExecutionStore` (`execution_store.go`) - thread-safe in-memory execution tracking
- `session.go` - ACP session initialization and resume
- `streams.go` - WebSocket stream connections to agentctl
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
- `sprites` - Sprites cloud environment ✅
- `remote_docker`, `remote_vps`, `k8s` - Planned

### Execution Flow

```
Client (WS) → Orchestrator → Lifecycle Manager → ExecutorBackend (container/process) → agentctl
                                                                                          ↓
Client (WS) ← Orchestrator ← Lifecycle Manager ←──── stream updates (WS) ──────── agent subprocess
```

1. Orchestrator receives `task.start` via WS
2. Lifecycle Manager creates executor instance (container or process)
3. agentctl starts inside the instance, agent subprocess is configured and started
4. Agent events stream back via WS through the chain

**Session Resume:** `TaskSession.ACPSessionID` stored for resume; `ExecutorRunning` tracks active state; on restart `RecoverInstances()` reconnects.

**Provider Pattern:** Packages expose `Provide(cfg, log) (*impl, cleanup, error)` for DI. Returns implementation, cleanup function, and error. Cleanup called during graceful shutdown.

**Worktrees:** `internal/worktree/Manager` provides workspace isolation. Each session can have its own worktree (branch) to prevent conflicts between concurrent agents.

**Executor default scripts:** Default prepare scripts are in `internal/agent/lifecycle/default_scripts.go`; `internal/scriptengine/` handles placeholder resolution.

---

## agentctl Server

### API Groups

agentctl exposes these route groups (see `internal/agentctl/server/api/`):
- `/health`, `/info`, `/status` - Health and status
- `/instances/*` - Multi-instance management
- `/processes/*` - Agent subprocess management (start/stop)
- `/agent/configure`, `/agent/stream` - Agent configuration and event streaming
- `/git/*` - Git operations (status, commit, push, pull, rebase, stage, etc.)
- `/shell/*` - Shell session management
- `/workspace/*` - File operations, search, tree
- `/vscode/*` - VS Code integration proxy

### Adapter Model

Protocol adapters in `adapter/transport/` normalize different agent CLIs:
- `AgentAdapter` interface defines `Start()`, `Stop()`, `Prompt()`, `Cancel()`
- Transports: `acp` (Claude Code), `codex` (OpenAI Codex), `opencode`, `shared`, `streamjson`
- Top-level adapters: `CopilotAdapter` (GitHub Copilot SDK), `AmpAdapter` (Sourcegraph Amp)
- `process.Manager` owns subprocess, wires stdio to adapter
- Factory pattern in `adapter/factory.go` selects adapter by agent type

### ACP Protocol

JSON-RPC 2.0 over stdin/stdout between agentctl and agent process. Requests: `initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`. Notifications: `session/update` with types `message_chunk`, `tool_call`, `tool_update`, `complete`, `error`, `permission_request`, `context_window`.

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
│   ├── comments/                  # code review diff comments
│   ├── github/                    # GitHub PRs, reviews
│   └── ui/                        # preview, connection, active state
├── hydration/                     # SSR merge strategies

hooks/domains/{kanban,session,workspace,settings,comments,github}/  # Domain-organized hooks
lib/api/domains/                    # API clients
├── kanban-api, session-api, workspace-api, settings-api, process-api
├── plan-api, queue-api, workflow-api, stats-api, github-api
├── user-shell-api, debug-api, secrets-api, sprites-api, vscode-api
├── health-api, utility-api
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

### Commit Conventions (enforced by CI)

Commits to `main` **must** follow [Conventional Commits](https://www.conventionalcommits.org/) (`type: description`). PRs are squash-merged — the PR title becomes the commit, validated by CI. Changelog is auto-generated from these via git-cliff (`cliff.toml`). See `.agents/skills/commit/SKILL.md` for allowed types and examples.

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

### Knowledge
- **Decisions:** Architecture decisions are recorded in `docs/decisions/`. Read `docs/decisions/INDEX.md` for an overview. When making significant architectural choices, create a new ADR via `/record decision`.
- **Plans:** Implementation plans are stored in `docs/plans/`. Save completed feature designs via `/record plan`.

### Backend
- Provider pattern for DI; stderr for logs, stdout for ACP only
- Pass context through chains; event bus for cross-component comm
- **Execution access:** Workspace-oriented handlers (files, shell, inference, ports, vscode, LSP) MUST use `GetOrEnsureExecution(ctx, sessionID)` — it recovers from backend restarts by creating executions on-demand. Only use `GetExecutionBySessionID` for operations that require a running agent process (prompt, cancel, mode).
- **Testing:** Prefer `testing/synctest` (Go 1.24+) over `time.Sleep` for time-dependent tests. Use `synctest.Test` to wrap tests with tickers or timeouts — it advances fake time instantly when all goroutines are idle. When `synctest` is not feasible (e.g., tests spawning external processes like `git`), use channel-based synchronization (`<-started`, non-blocking `select`) instead of sleep-based waits. Reserve `time.Sleep` only for integration tests that need real subprocess execution time.

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

**Last Updated**: 2026-03-05
