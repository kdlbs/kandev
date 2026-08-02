# Backend (Go) — architecture and conventions

Scoped guidance for `apps/backend/`. Repo-wide rules (commit format, code-quality limits, etc.) live in the root `AGENTS.md`. For plugin work, start at the [canonical plugin authoring guide](../../docs/public/plugins-authoring.md), follow choose recipe → edit `manifest.yaml` → implement → validate → package → smoke test, and treat `pkg/pluginsdk`, `proto/kandev/plugin/v1/plugin.proto`, `internal/plugins/manifest`, and `internal/plugins/pkgtar` as authoritative; the fixture is test support, and plugins must not access databases or `internal/...` packages.

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
│   ├── auth/             # Opt-in auth, per-user scoping, middleware, API, store
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
│   ├── office/           # Autonomous agent management (agents, approvals, channels, config, costs,
│   │                     # dashboard, infra, labels, onboarding, projects, repository, runtime,
│   │                     # routines, routing, scheduler, service, shared, skills, workspaces)
│   ├── events/           # Event bus for internal pub/sub
│   ├── gateway/          # WebSocket gateway
│   ├── github/           # GitHub API integration (PRs, reviews, webhooks)
│   ├── githubauth/       # Shared GitHub credential-broker environment contract
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
│   │   └── service/      # Workflow CRUD, step resolution, and sync apply
│   ├── workflowsync/     # GitHub workflow sync (per-workspace repo config, poller, force sync)
│   └── worktree/         # Git worktree management for workspace isolation
```

## Key Concepts

**Orchestrator** coordinates task execution:
- Receives task start/stop/resume requests via WebSocket
- Delegates to lifecycle manager for agent operations
- Handles event-driven state transitions via workflow engine
- Located in `internal/orchestrator/`

**Watcher Dispatch Coordinator** (`internal/orchestrator/watcher_dispatch.go`) is the single pipeline that turns a freshly-observed external issue (Linear, Jira, future) into a Kandev task. Bus subscribers for each integration forward the event to `WatcherDispatchCoordinator.Dispatch` with a per-integration `WatcherSource` implementation (`source_linear.go`, `source_jira.go`). Source methods carry the integration-specific bits (reserve dedup, build task request, attach task ID, release, auto-start params); the coordinator owns the cross-cutting pipeline (create task, decide auto-start, error/release handling). Add a new watcher = implement `WatcherSource` + register a one-line bus subscriber. Do NOT add another `createXIssueTask` mirror.

**GitHub App registration catalog** (`internal/github/`) stores zero or more managed/imported App
registrations; none is a global default. Workspace App connections must carry both registration ID
and installation ID. Runtime clients, token caches, broker leases, OAuth state, webhook delivery,
and service cache scopes must retain registration identity and credential generation. Reusing a
registration intentionally shares root App credentials and bot identity; installation grants and
workspace credentials remain isolated. Registration create/import/select/install belongs to the
workspace GitHub settings flow, and backend startup configuration is not an App credential source.
Registration-specific callback and webhook routes select one candidate registration but never
replace state verification, installation association, or HMAC verification.

**Workflow Engine** (`internal/workflow/engine/`) provides typed state-machine evaluation:
- `Engine.HandleTrigger()` evaluates step actions for triggers (on_enter, on_turn_start, on_turn_complete, on_exit)
- `TransitionStore` interface abstracts persistence (implemented by `orchestrator.workflowStore`)
- `CallbackRegistry` maps action kinds to callbacks (plan mode, auto-start, context reset)
- First-transition-wins: multiple transition actions in one trigger, first eligible wins
- `EvaluateOnly` mode: engine evaluates without persisting, caller orchestrates on_exit → DB → on_enter
- `RequiresApproval` on actions: transitions requiring review gating are skipped
- Idempotent by `OperationID`; session-scoped data bag via `MachineState.Data`

**Agent Runtime** (`internal/agent/runtime/`) is the single seam for launching, resuming, stopping, and observing agent executions. ADR 0004 introduced this in Phase 1 of task-model-unification. The public surface is `runtime.Runtime` (`runtime.go`); a thin facade (`facade.go`) delegates to a `Backend` (satisfied by `*lifecycle.Manager`).

**Runtime environment invariant:** `Agent.Runtime().Env` applies to every ACP subprocess entry point. Route new overrides through host-utility probes and sessionless prompts into agentctl child processes before sanitization; cover probe DTO, prompt DTO, and child-process boundaries.

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

**Agent discovery vs. ACP probing:** discovery answers only whether an agent
executable is available. Authentication, protocol compatibility, and supported
models or modes belong to the ACP probe path; do not reintroduce those checks
as installation gates.

**agentctl** is an HTTP server that:
- Runs inside Docker containers or as standalone process
- Manages agent subprocess via stdin/stdout (ACP protocol)
- Exposes workspace operations (shell, git, files)
- Supports multiple concurrent instances on different ports

Standalone agentctl is launched in its own process group so terminal Ctrl+C is
handled by the backend lifecycle manager first. Do not make standalone agentctl
share the backend's foreground process group; that bypasses supervised shutdown
and can leak ACP subprocesses.

**Executor Types** (database model):
- `local_pc` - Standalone process on host
- `local_docker` - Docker container on host
- `sprites` - Sprites cloud environment
- `remote_docker`, `remote_vps`, `k8s` - Planned

**Remote SSH executor platforms:** Treat supported remote OS/arch values as an end-to-end contract. Platform probe/normalization, lifecycle support checks, agentctl helper resolution, platform default shell, SSH readiness endpoints, frontend response types, and tests must stay aligned. Preserve raw unsupported platform details in user-facing errors, but use normalized values for supported-platform matching. Keep shell defaults platform-aware: Darwin defaults to `zsh`, Linux defaults to `bash`, unless an explicit shell is saved.

**Opt-in authentication & per-user scoping** (`internal/auth/`): auth is OFF by default; the global middleware (`auth/httpmw`, installed after CORS in `backendapp.buildHTTPServer`) then injects a synthetic admin identity for the pre-auth single user, so behavior is unchanged. Enablement is the `features.auth` runtime flag (`KANDEV_FEATURES_AUTH`, Settings > System > Feature Toggles) — the auth service derives its mode from `cfg.Features.Auth` (`disabled` / `setup` = flag-on-no-admin / `enabled` = flag-on-admin-exists); there is no separate `auth.mode` setting. When enabled, requests authenticate via a `kandev_session` cookie (opaque token, SHA-256 at rest, DB-backed) or a `kandev_pat_*` bearer token. Scoping rules:
- **Identity travels in the request context** (`authn.IdentityFromContext`). No identity = internal caller (pollers, event bus, office schedulers) = unscoped. Synthetic identity = auth disabled = unscoped.
- **In-session agent MCP is scoped to the task owner.** The MCP tools an agent gets inside its own session are relayed over the agent's WebSocket stream, which carries no credential of its own. `internal/mcp/scope` resolves the stream's task → workspace → owner and attaches that user's real identity before dispatch (`lifecycle.Manager.SetMCPIdentityScoper`), so the same `authorize*` checks apply as for the PAT-authenticated `/mcp` endpoint. The owning task comes from the `AgentExecution`, never from the agent-supplied payload — do not "improve" this by reading `session_id`/`task_id` out of the request. Tool handlers stay identity-agnostic. Under enforced auth every dispatch is scoped to *somebody*: a resolvable active owner gets their identity; an unowned workspace gets a sentinel user ID that reaches unowned rows only; anything unresolvable (missing task/workspace row, or an owner whose account was deleted or disabled) is **denied**. Never return an identity-free context from this path — the task service reads that as an internal caller and grants everything.
- **Workspaces are per-user** (`workspaces.owner_id`); the task service filters/denies at the service layer with `*NotFound` sentinels (no existence leak). Unowned rows (`owner_id=''`) stay visible until the setup wizard claims them for the admin.
- **New user-facing service entry points must apply scoping** — call the `authorize*` helpers in `task/service/service_access.go` (or the same pattern) when adding routes that read or mutate workspace-scoped data. This includes **session-keyed** entry points: any service method taking a caller-supplied `sessionID` (or a `taskID` used to reach sessions) authorizes via `authorizeTaskID(session.TaskID)`. Authorize off the row you already read rather than calling `AuthorizeSessionAccess`, which re-reads the session. Batch helpers that take ID *lists* (`BatchGetSessionsForTasks`, `GetPrimarySessionInfoForTasks`, …) are exempt by convention: their callers derive the IDs from an already-authorized task list, so per-ID checks would add N queries to list views for no gain — keep it that way, and don't hand them caller-supplied IDs.
- **Two session paths bypass the task service and must guard themselves.** (1) Handlers that resolve an execution by a bare in-memory lookup (`GetExecutionBySessionID`, `*BySessionID`) skip the `GetOrEnsure*` chokepoint where the lifecycle check runs — call `service.AuthorizeSessionAccess` (see `ProcessHandlers.denySessionAccess`) or `lifecycleMgr.CheckSessionAccess` first. (2) The orchestrator reads sessions through its own repo handle, so it inherits nothing from the task service; session-keyed entry points there call `authorizeSession` / `authorizeTask`, wired by `SetSessionAccessChecker` / `SetTaskAccessChecker`. An entry point that accepts **both** a task and a session ID must use `authorizeTaskSessionPair`, never `authorizeSession` alone: the caller can satisfy a session-only check with one of their own sessions while pointing `taskID` at someone else's task, which the method then uses for its task-scoped work. Put the guard **first** in the method, before any repo/executor/agent-manager use — `TestSessionKeyedEntryPointsGuardBeforeDependencies` runs each entry point with nil dependencies and fails on a panic if a guard is placed too late.
- **Dispatched WS actions have a gateway backstop.** `Client.authorizeAction` (`internal/gateway/websocket/dispatch_scope.go`) checks any payload carrying `task_id` or `session_id` against the client's identity *before* dispatch, so a newly added action is scoped by default rather than only when its author remembers. Handler-level scoping is still required (defense in depth, and it covers non-WS callers) — do not delete a service-layer check because the backstop exists. The backstop reads `task_id`, `session_id` and `task_environment_id` — keep using those names for task-scoped actions, because an action that invents a new name for the same kind of resource silently opts out. (`user_shell.stop` was exactly that: it keys off `task_environment_id` with `task_id` optional, so a `task_id`-only backstop missed it.) Also avoid permissive types (`any`, `json.RawMessage`) for those fields: a payload that fails to unmarshal into the ref struct is treated as naming nothing.
- **WS**: clients carry their identity; dispatched actions and subscriptions are scoped; workspace-carrying events route via `Hub.BroadcastToWorkspace`. A new `hub.Broadcast` (global) call site needs a `//ws:global` justification comment.
- **Self-authenticating webhooks** (automation, office channels, plugin webhooks) and `/health`, `/api/v1/features`, `/api/v1/app-state` stay public — the allowlist lives in `auth/httpmw/middleware.go` with a pinning test.

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

**Worktree file materialization:** `copy_files` (`Repository.CopyFiles`) is a comma-separated spec of repository-relative gitignored paths/doublestar globs seeded into each new worktree. Copy is the default; the exact terminal `:symlink` suffix (for example `.env.local:symlink`) creates a relative host-worktree link so source changes propagate live. Other colons stay literal (`config:dev`, `.env:`); use `::symlink` to copy a literal path ending in `:symlink`. Malformed reserved syntax is rejected at save time. Parsing and materialization live in `internal/worktree/copyfiles/` (`ParseSpecs`, `ValidateSpec`, `Copy`); duplicates and overlapping matches are first-entry-wins. `Manager.copyConfiguredFiles` runs before setup during worktree creation. Source and destination containment checks reject traversal and symlinked destination parents. Failures are non-fatal warnings. Windows link creation is best-effort. Remote executors cannot link to the host, so `Parse`/`Plan` preserve literal colon paths but turn symlink-mode entries into copied bytes delivered through `WriteEntries`.

**Executor default scripts:** Default prepare scripts are in `internal/agent/runtime/lifecycle/default_scripts.go`; `internal/scriptengine/` handles placeholder resolution.

## Conventions

- Provider pattern for DI; stderr for logs, stdout for ACP only.
- Pass context through chains; event bus for cross-component comm.
- **Event-bus wildcard parity:** New NATS wildcard subscriptions must verify equivalent `MemoryEventBus` semantics in `go test ./internal/events/bus`.
- **Repository provider identity:** Provider-backed repositories are keyed by workspace, provider, normalized `provider_host` origin, full owner/namespace, and name. Persist `provider_host` when importing or resolving a remote; do not infer self-managed GitLab rows from owner/name alone. Legacy rows with an empty host have unknown identity and must fail closed for provider write/link operations.
- **Execution access:** Workspace-oriented handlers (files, shell, inference, ports, vscode, LSP) MUST use `GetOrEnsureExecution(ctx, sessionID)` — it recovers from backend restarts by creating executions on-demand. Only use `GetExecutionBySessionID` for operations that require a running agent process (prompt, cancel, mode).
- **Task lifecycle events:** Any code path that mutates a task row must publish via the event bus (`task.created` / `task.updated` / `task.deleted`) — either by going through `Service.CreateTask` / `UpdateTask` / `DeleteTask` / `ArchiveTask`, or by calling `publishTaskEvent` (or one of the `Publish*` helpers in `service_events.go`) directly. Walking `repository.TaskRepository` straight bypasses event publishing and breaks WS-driven UI like the All-Workflows kanban view. `HandoffService`'s cascade methods learned this the hard way — they now require a `TaskEventPublisher` wired via `SetTaskEventPublisher`. New cascade / bulk / cleanup paths must follow the same pattern.
- **Testing:** Prefer `testing/synctest` (Go 1.24+) over `time.Sleep` for time-dependent tests. Use `synctest.Test` to wrap tests with tickers or timeouts — it advances fake time instantly when all goroutines are idle. When `synctest` is not feasible (e.g., tests spawning external processes like `git`), use channel-based synchronization (`<-started`, non-blocking `select`) instead of sleep-based waits. Reserve `time.Sleep` only for integration tests that need real subprocess execution time.
  - **Test cleanup:** Register `t.Cleanup` immediately after creating resources that need teardown (adapters, `io.Pipe` writers, background goroutines) — before any `t.Fatal`/`t.Fatalf` path. Late cleanup registration leaks pipes and goroutines on early failure.
  - **Joining production goroutines in tests:** When code spawns untracked goroutines (e.g. `fireWakeup`), don't rely on arbitrary sleeps. Join via an observable side effect — e.g. block on `EventTypeComplete` from `a.updatesCh` after unblocking the fake agent. Use short timeouts (~100ms) for in-process negative assertions; reserve multi-second waits for subprocess/integration tests only.
  - **Path/security tests:** Avoid using the real filesystem root as a fixture root. Build fake absolute roots under `t.TempDir()` with `filepath.Join`; this keeps tests portable across Windows, POSIX, and privileged cloud executors.
  - **Filesystem permission tests:** Assert permission-denied behavior only after probing that the current executor enforces the permission bit change. Root-like Sprite executors may bypass `chmod` restrictions.
  - **Filesystem safety checks:** Carry the original `os.Lstat` `FileInfo` (or opened handle) through every decision; do not re-stat a validated path, which reopens a TOCTOU window before side effects. Test mismatches before writes or manager commands.
  - **Git indexed-environment tests:** When a test supplies `GIT_CONFIG_COUNT`,
    `GIT_CONFIG_KEY_n`, and `GIT_CONFIG_VALUE_n`, clear every inherited indexed
    key first and restore it with `t.Cleanup`. Changing only the count can leave
    higher parent indexes behind and create a malformed Git config block.
  - **Full test output:** For local full-suite pass/fail validation, prefer plain `go test -race ./...`. `go test -json ./...` can emit very large JSONL streams; if a wrapper or tracing tool truncates the stream mid-record, downstream JSON parsing may fail even when Go tests passed. Use JSON output mainly for CI artifacts or test-report tooling that explicitly requires it.
  - **Private executable helpers:** When library code starts `os.Executable()` in
    a private helper mode, do not rely only on a command package's dispatch or
    package-specific `TestMain`: an importing package's test binary can be the
    executable and recursively run tests. Use either an explicitly marked
    private env-and-argument trampoline that runs before normal test entry, or
    inject a dedicated helper executable. Cover it from a different importing
    package, not only the helper package.

### Goroutine ownership and leak testing

Every long-running goroutine must have a single owner with explicit start and stop semantics:

- **Lifecycle:** the type that spawns the goroutine also exposes `Start(ctx)` / `Stop()` (or equivalent). `Start` registers on a `sync.WaitGroup`; `Stop` cancels the goroutine's context (or closes a `stopCh`) and `wg.Wait()`s for drain. Idempotent on both ends. `internal/integrations/healthpoll`, `internal/jira`, `internal/linear`, and `internal/github` pollers are the canonical shape.
- **E2E reset invariant:** `seedData`/backend are worker-scoped, so any workspace-scoped state a global poller reads (for example `github_review_watches`) must be deleted in `cmd/kandev/e2e_reset.go` before task deletion — otherwise the poller recreates rows mid-reset and later tests see duplicates. Add a `Delete...ByWorkspace` cascade when introducing a new poller-backed entity.
- **Cancellation:** the goroutine selects on `ctx.Done()` (or `stopCh`) in every long wait. Never use `time.Sleep` in a retry/backoff loop — use `time.NewTimer` inside a `select` that also watches the shutdown signal (see `lifecycle.StreamManager.sleepOrStop`).
- **Detached helpers:** event handlers and short-lived `go func()` calls in `internal/orchestrator/` and `internal/agent/runtime/lifecycle/` must accept a cancellable context (or check the owning type's shutdown signal) and return promptly when it fires.
- **Leak testing:** packages that spawn goroutines add `goleak.VerifyTestMain(m)` in a per-package `TestMain`. New packages of this kind must follow suit. When a third-party background goroutine genuinely can't be drained, suppress it with `goleak.IgnoreTopFunction(...)` and leave a comment explaining why. Currently instrumented: `internal/gateway/websocket/`, `internal/agent/runtime/lifecycle/`, `internal/agentctl/server/process/`, `internal/orchestrator/`, `internal/github/`, `internal/jira/`, `internal/linear/`, `internal/integrations/healthpoll/`.

## Backups

- On every SQLite boot, `persistence.Provide` reads `kandev_meta.kandev_version`. If the stored version differs from the binary version (or any user tables exist but no version is recorded), it takes a `VACUUM INTO` snapshot into `<data-dir>/backups/` before running migrations.
- Retention: 2 backups kept (newest two by mtime); older ones are pruned after the snapshot succeeds.
- Postgres: backup is skipped with a log line. Use `pg_dump` for Postgres backups.
- Boot aborts if the backup fails — the pool is closed and `Provide` returns an error.
- After all repos complete `initSchema`, `cmd/kandev/storage.go:recordSchemaVersion` writes the current binary version into `kandev_meta` (non-fatal; a failure just means the next boot will take a fresh snapshot).
- Migration logging: `db.MigrateLogger.Apply(name, stmt)` — success logs Info, "already exists" / "duplicate column name" is silently swallowed, anything else logs Warn but never returns an error (preserving the existing swallow-error contract).
- Schema replay handling: use `internal/db` helpers such as `IsDuplicateColumnError` / `IsAlreadyExistsError` instead of local error-string matching. When adding or changing startup schema code, include fresh-DB plus same-DB replay tests for SQLite; add the same env-gated Postgres replay coverage when the path supports Postgres. See `docs/decisions/0027-replayable-schema-migrations.md`.

## Schema & migrations (SQLite repository)

`internal/task/repository/sqlite` also runs against PostgreSQL: avoid unguarded SQLite-only `rowid`, JSON, or date syntax and add an environment-gated PostgreSQL behavior test for every changed dialect-sensitive method (schema replay is insufficient).

`initSchema()` in `internal/task/repository/sqlite/base_schema.go` runs the `init*Schema` (CREATE TABLE) steps **before** `runMigrations()`. The table-creation DDL uses `CREATE TABLE IF NOT EXISTS`, so on an **existing** database it is a no-op and never adds columns to a table that is already present.

**Rule:** when you add a column to an existing table, add it **only** via an idempotent `ADD COLUMN` migration in `runMigrations()` (`base_migrations.go`), never by editing the table's `CREATE TABLE` alone. Anything that *references* that new column — an index, a backfill `UPDATE`, a partial-index predicate — must live in `runMigrations()` **after** the `ADD COLUMN`, not in the `init*Schema` DDL. Putting a `CREATE INDEX ... (new_col)` in the schema-init block crashes existing DBs with `no such column: new_col`, because schema init runs before the migration that adds the column.

You may still list the column in the `CREATE TABLE` so fresh DBs get it inline, but the migration is the source of truth for evolution and must stand alone. New columns also need: the struct field in `models/`, the DTO field + `ToAPI` in `pkg/api/v1/`, and every `CreateX`/`UpdateX`/bulk write in the repo that should set it.

## Code-quality limits

Enforced by `apps/backend/.golangci.yml` (errors on new code only):
- Functions: ≤80 lines, ≤50 statements
- Cyclomatic complexity: ≤15 · Cognitive complexity: ≤30
- Nesting depth: ≤5 · Naked returns only in functions ≤30 lines
- No duplicated blocks (≥150 tokens) · Repeated strings → constants (≥3 occurrences)

When you hit a limit, extract a helper function. Prefer composition over growing a single function.

When a PR fixup touches backend code, run the CI-style changed-file linter locally from `apps/backend` with the PR base SHA before pushing, because CI enforces changed-file complexity thresholds:

```bash
golangci-lint run ./... --new-from-rev="<base-sha>" --timeout=5m
```

## Further scoped notes

- `internal/agentctl/AGENTS.md` — agentctl server route groups, adapter model, ACP protocol
- `internal/agentctl/server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`), iframe-blocking header stripping
- `internal/integrations/AGENTS.md` — playbook for adding a new third-party integration (Jira/Linear pattern)
- `docs/i18n.md` ("Backend") — `internal/i18n` covers only what Go renders straight to a browser: the SPA-unavailable error pages and the shared-task artifacts (`share.html`, gist README and description). Both are complete. Everything else stays English by design; for new user-facing output prefer a stable error code the frontend translates. Use `i18n.Tf` for anything carrying a value — never `fmt.Sprintf` a translated string, and never build a plural in Go. A locale for output that outlives the request is resolved once at write time and threaded as an argument, not a context value (ADR `2026-08-01-share-artifact-locale.md`).
- `cmd/mock-agent/AGENTS.md` — predefined `/e2e:<name>` scenarios vs inline `e2e:...` scripts, recipe for adding a scenario, and the rebuild-before-e2e requirement
