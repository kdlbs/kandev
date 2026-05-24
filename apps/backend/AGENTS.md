# Backend (Go) — architecture and conventions

Scoped guidance for `apps/backend/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`.

## Package Structure

```text
apps/backend/
├── cmd/
│   ├── kandev/           # Main backend binary entry point
│   ├── agentctl/         # Agentctl binary (runs inside containers or standalone)
│   └── mock-agent/       # Mock agent for testing
├── internal/
│   ├── agent/
│   │   ├── runtime/      # Agent runtime: single seam for Launch/Resume/Stop/observe
│   │   │   ├── lifecycle/    # Agent instance management (moved from agent/lifecycle)
│   │   │   ├── agentctl/     # HTTP client for talking to agentctl (moved from agentctl/client)
│   │   │   └── routingerr/   # Provider error classifier + sanitizer + ProviderProber registry
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
│   ├── office/           # Autonomous agent management (Office feature)
│   │   ├── agents/       # Agent instance CRUD + auth guards
│   │   ├── approvals/    # Approval requests and decisions
│   │   ├── channels/     # External integration channels (webhooks)
│   │   ├── config/       # Config sync (DB ↔ filesystem)
│   │   ├── configloader/ # Filesystem config reader/writer
│   │   ├── costs/        # Cost tracking and budget policies
│   │   ├── dashboard/    # Dashboard API, issues, activity, live runs
│   │   ├── infra/        # GC, reconciliation
│   │   ├── labels/       # Task labels
│   │   ├── onboarding/   # Workspace onboarding wizard API
│   │   ├── projects/     # Project management
│   │   ├── repository/   # Office SQLite persistence
│   │   ├── runtime/      # Agent run context, capabilities, and runtime action surface
│   │   ├── routines/     # Scheduled recurring tasks
│   │   ├── routing/      # Provider routing: resolver, validators, catalogue, backoff, agent-overrides
│   │   ├── scheduler/    # Wakeup scheduler (duplicate of service scheduler features)
│   │   ├── service/      # Core office service (wakeups, event subscribers, execution policy)
│   │   ├── shared/       # Shared interfaces and activity logging
│   │   ├── skills/       # Skill injection and materialization
│   │   └── workspaces/   # Workspace deletion handler
│   ├── events/           # Event bus for internal pub/sub
│   ├── gateway/          # WebSocket gateway
│   ├── github/           # GitHub API integration (PRs, reviews, webhooks)
│   ├── common/           # Shared utilities, config, logger
│   ├── integration/      # External integrations
│   ├── integrations/     # Shared shapes for third-party integrations
│   │   ├── healthpoll/   # Reusable 90s auth-health Poller (used by jira, linear)
│   │   └── secretadapter/ # Upsert-style adapter over secrets.SecretStore
│   ├── jira/             # Jira/Atlassian Cloud integration (config, REST client, poller)
│   ├── linear/           # Linear integration (config, GraphQL client, poller)
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
│   ├── tools/            # Tool integrations
│   ├── user/             # User management
│   ├── utility/          # Shared utility functions
│   ├── workflow/         # Workflow engine
│   │   ├── engine/       # Typed state-machine engine
│   │   ├── models/       # Workflow step, template, and history models
│   │   ├── repository/   # Workflow persistence (SQLite)
│   │   └── service/      # Workflow CRUD and step resolution
│   └── worktree/         # Git worktree management for workspace isolation
```

## Key Concepts

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

**Agent Runtime** (`internal/agent/runtime/`) is the single seam for launching, resuming, stopping, and observing agent executions. ADR 0004 introduced this in Phase 1 of task-model-unification. The public surface is `runtime.Runtime` (`runtime.go`); a thin facade (`facade.go`) delegates to a `Backend` (satisfied by `*lifecycle.Manager`).

**Convention:** only `internal/agent/runtime/` (and code that pre-dates Phase 1 migration) may import `runtime/lifecycle` or `runtime/agentctl` directly. New consumers — workflow engine actions, cron-driven trigger handlers, future task-tier callers — should depend on `runtime.Runtime`. Existing call sites are migrated through later phases of task-model-unification.

**Lifecycle Manager** (`internal/agent/runtime/lifecycle/`) manages agent instances under the runtime:
- `Manager` (`manager.go`, `manager_*.go`) - central coordinator for agent lifecycle
- `ExecutorBackend` interface (`executor_backend.go`) - abstracts execution environment (Docker, Standalone, Sprites, Remote Docker)
- `ExecutionStore` (`execution_store.go`) - thread-safe in-memory execution tracking
- `session.go` - ACP session initialization and resume
- `streams.go` - WebSocket stream connections to agentctl
- `process_runner.go` - agent process launch and management
- `profile_resolver.go` - resolves agent profiles/settings

**agentctl client** (`internal/agent/runtime/agentctl/`) is the HTTP/WS client used by the lifecycle manager to talk to a running agentctl instance. It is a runtime-tier package and should not be imported outside `internal/agent/runtime/`.

**agentctl** is an HTTP server that:
- Runs inside Docker containers or as standalone process
- Manages agent subprocess via stdin/stdout (ACP protocol)
- Exposes workspace operations (shell, git, files)
- Supports multiple concurrent instances on different ports

**Executor Types** (database model):
- `local_pc` - Standalone process on host
- `local_docker` - Docker container on host
- `sprites` - Sprites cloud environment
- `remote_docker`, `remote_vps`, `k8s` - Planned

## Execution Flow

```text
Client (WS) → Orchestrator → Lifecycle Manager → ExecutorBackend (container/process) → agentctl
                                                                                          ↓
Client (WS) ← Orchestrator ← Lifecycle Manager ←──── stream updates (WS) ──────── agent subprocess
```

1. Orchestrator receives `session.launch` via WS
2. Lifecycle Manager creates executor instance (container or process)
3. agentctl starts inside the instance, agent subprocess is configured and started
4. Agent events stream back via WS through the chain

**Session Resume:** `TaskSession.ACPSessionID` stored for resume; `ExecutorRunning` tracks active state; on restart `RecoverInstances()` reconnects.

**Provider Pattern:** Packages expose `Provide(cfg, log) (*impl, cleanup, error)` for DI. Returns implementation, cleanup function, and error. Cleanup called during graceful shutdown.

**Worktrees:** `internal/worktree/Manager` provides workspace isolation. Each session can have its own worktree (branch) to prevent conflicts between concurrent agents.

**Executor default scripts:** Default prepare scripts are in `internal/agent/runtime/lifecycle/default_scripts.go`; `internal/scriptengine/` handles placeholder resolution.

## Conventions

- Provider pattern for DI; stderr for logs, stdout for ACP only.
- Pass context through chains; event bus for cross-component comm.
- **Execution access:** Workspace-oriented handlers (files, shell, inference, ports, vscode, LSP) MUST use `GetOrEnsureExecution(ctx, sessionID)` — it recovers from backend restarts by creating executions on-demand. Only use `GetExecutionBySessionID` for operations that require a running agent process (prompt, cancel, mode).
- **Task lifecycle events:** Any code path that mutates a task row must publish via the event bus (`task.created` / `task.updated` / `task.deleted`) — either by going through `Service.CreateTask` / `UpdateTask` / `DeleteTask` / `ArchiveTask`, or by calling `publishTaskEvent` (or one of the `Publish*` helpers in `service_events.go`) directly. Walking `repository.TaskRepository` straight bypasses event publishing and breaks WS-driven UI like the All-Workflows kanban view. `HandoffService`'s cascade methods learned this the hard way — they now require a `TaskEventPublisher` wired via `SetTaskEventPublisher`. New cascade / bulk / cleanup paths must follow the same pattern.
- **Testing:** Prefer `testing/synctest` (Go 1.24+) over `time.Sleep` for time-dependent tests. Use `synctest.Test` to wrap tests with tickers or timeouts — it advances fake time instantly when all goroutines are idle. When `synctest` is not feasible (e.g., tests spawning external processes like `git`), use channel-based synchronization (`<-started`, non-blocking `select`) instead of sleep-based waits. Reserve `time.Sleep` only for integration tests that need real subprocess execution time.

## Backups

- On every SQLite boot, `persistence.Provide` reads `kandev_meta.kandev_version`. If the stored version differs from the binary version (or any user tables exist but no version is recorded), it takes a `VACUUM INTO` snapshot into `<data-dir>/backups/` before running migrations.
- Retention: 2 backups kept (newest two by mtime); older ones are pruned after the snapshot succeeds.
- Postgres: backup is skipped with a log line. Use `pg_dump` for Postgres backups.
- Boot aborts if the backup fails — the pool is closed and `Provide` returns an error.
- After all repos complete `initSchema`, `cmd/kandev/storage.go:recordSchemaVersion` writes the current binary version into `kandev_meta` (non-fatal; a failure just means the next boot will take a fresh snapshot).
- Migration logging: `db.MigrateLogger.Apply(name, stmt)` — success logs Info, "already exists" / "duplicate column name" is silently swallowed, anything else logs Warn but never returns an error (preserving the existing swallow-error contract).

## Code-quality limits

Enforced by `apps/backend/.golangci.yml` (errors on new code only):
- Functions: ≤80 lines, ≤50 statements
- Cyclomatic complexity: ≤15 · Cognitive complexity: ≤30
- Nesting depth: ≤5 · Naked returns only in functions ≤30 lines
- No duplicated blocks (≥150 tokens) · Repeated strings → constants (≥3 occurrences)

When you hit a limit, extract a helper function. Prefer composition over growing a single function.

## Further scoped notes

- `internal/agentctl/AGENTS.md` — agentctl server route groups, adapter model, ACP protocol
- `internal/agentctl/server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`), iframe-blocking header stripping
- `internal/integrations/AGENTS.md` — playbook for adding a new third-party integration (Jira/Linear pattern)
