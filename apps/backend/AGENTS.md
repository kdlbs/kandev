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
│   ├── i18n/             # Localization for backend-rendered browser/share artifacts
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

**Cancellation progress projection:** `orchestrator.Service.CancellationPending(sessionID)` is a
runtime-only, session-scoped view of accepted cancellation work. Serialization that carries the
boolean with ordering identity uses the atomic `CancellationPendingSnapshot(sessionID)` provider,
whose process-local revision increments on first-begin and last-end transitions. The task DTO
package exposes both the compatibility boolean provider and snapshot seam; boot state, task-session
HTTP/WS lists and detail responses, and the session-scoped WebSocket notification must project
explicit `true`/`false` values plus the revision. Keep count, revision, and publication queue updates
in one critical section, drain event-bus sends outside it, and never persist this transient marker or
turn it into a coarse session lifecycle state.

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

**Convention:** only `internal/agent/runtime/` (and code that pre-dates Phase 1 migration) may import `runtime/lifecycle` or `runtime/agentctl` directly. New consumers — workflow engine actions, cron-driven trigger handlers, future task-tier callers — should depend on `runtime.Runtime` or narrow local interfaces for the lifecycle-owned capability they consume. Existing call sites are migrated through later phases of task-model-unification.

**Lifecycle Manager** (`internal/agent/runtime/lifecycle/`) manages agent instances under the runtime:
- `Manager` (`manager.go`, `manager_*.go`) - central coordinator for agent lifecycle
- `ExecutorBackend` interface (`executor_backend.go`) - abstracts execution environment (Docker, Standalone, Sprites, Remote Docker)
- `ExecutionStore` (`execution_store.go`) - thread-safe in-memory execution tracking
- `session.go` - ACP session initialization and resume
- `streams.go` - WebSocket stream connections to agentctl
- `process_runner.go` - agent process launch and management
- `profile_resolver.go` - resolves agent profiles/settings
**Lifecycle callback identity:** Process callbacks must carry the launched PID and generation; ignore delayed callbacks from replaced processes and duplicate callbacks, with tests for both old-generation and duplicate delivery.

**agentctl client** (`internal/agent/runtime/agentctl/`) is the HTTP/WS client used by the lifecycle manager to talk to a running agentctl instance. It is a runtime-tier package and should not be imported outside `internal/agent/runtime/`.

**Agent discovery vs. ACP probing:** discovery answers only whether an agent executable is available; authentication, protocol compatibility, and supported models or modes belong to the ACP probe path, not installation gates.

**agentctl** is an HTTP server that:
- Runs inside Docker containers or as standalone process
- Manages agent subprocess via stdin/stdout (ACP protocol)
- Exposes workspace operations (shell, git, files)
- Supports multiple concurrent instances on different ports

Standalone agentctl is launched in its own process group so terminal Ctrl+C is handled by the backend lifecycle manager first; do not share the backend's foreground group, which bypasses supervised shutdown and can leak ACP subprocesses.

**Executor Types** (database model):
- `local_pc` - Standalone process on host
- `local_docker` - Docker container on host
- `sprites` - Sprites cloud environment
- `remote_docker`, `remote_vps`, `k8s` - Planned

**Remote SSH executor platforms:** Treat supported remote OS/arch values as an end-to-end contract. Platform probe/normalization, lifecycle support checks, agentctl helper resolution, platform default shell, SSH readiness endpoints, frontend response types, and tests must stay aligned. Preserve raw unsupported platform details in user-facing errors, but use normalized values for supported-platform matching. Keep shell defaults platform-aware: Darwin defaults to `zsh`, Linux defaults to `bash`, unless an explicit shell is saved.

**Remote SSH lifecycle:** Resolve and run remote preparation, then verify the canonical checkout, origin, and `HEAD` before starting agentctl. Retain the remote task directory in session state for stop/resume; terminal archive/delete/cascade cleanup runs over the live SSH connection before teardown, while ordinary stop and backend shutdown preserve the workspace.

**Opt-in authentication & per-user scoping** (`internal/auth/`): auth is OFF by default; the global middleware (`auth/httpmw`, installed after CORS in `backendapp.buildHTTPServer`) then injects a synthetic admin identity for the pre-auth single user, so behavior is unchanged. Enablement is the `features.auth` runtime flag (`KANDEV_FEATURES_AUTH`, Settings > System > Feature Toggles) — the auth service derives its mode from `cfg.Features.Auth` (`disabled` / `setup` = flag-on-no-admin / `enabled` = flag-on-admin-exists); there is no separate `auth.mode` setting. When enabled, requests authenticate via the session cookie (base name `kandev_session`; the effective name is derived from the request host — port-scoped on a ported host via `internal/common/httpcookie`, plain on a default-port host) or a `kandev_pat_*` bearer token. Scoping rules:
- **Identity travels in the request context** (`authn.IdentityFromContext`). No identity = internal caller (pollers, event bus, office schedulers) = unscoped. Synthetic identity = auth disabled = unscoped.
- **In-session agent MCP is scoped to the task owner.** The MCP tools an agent gets inside its own session are relayed over the agent's WebSocket stream, which carries no credential of its own. `internal/mcp/scope` resolves the stream's task → workspace → owner and attaches that user's real identity before dispatch (`lifecycle.Manager.SetMCPIdentityScoper`), so the same `authorize*` checks apply as for the PAT-authenticated `/mcp` endpoint. The owning task comes from the `AgentExecution`, never from the agent-supplied payload — do not "improve" this by reading `session_id`/`task_id` out of the request. Tool handlers stay identity-agnostic. Under enforced auth every dispatch is scoped to *somebody*: a resolvable active owner gets their identity; an unowned workspace gets a sentinel user ID that reaches unowned rows only; anything unresolvable (missing task/workspace row, or an owner whose account was deleted or disabled) is **denied**. Never return an identity-free context from this path — the task service reads that as an internal caller and grants everything.
- **Workspaces are per-user** (`workspaces.owner_id`); the task service filters/denies at the service layer with `*NotFound` sentinels (no existence leak). Unowned rows (`owner_id=''`) stay visible until the setup wizard claims them for the admin.
- **New user-facing service entry points must apply scoping** — call the `authorize*` helpers in `task/service/service_access.go` (or the same pattern) when adding routes that read or mutate workspace-scoped data. This includes **session-keyed** entry points: any service method taking a caller-supplied `sessionID` (or a `taskID` used to reach sessions) authorizes via `authorizeTaskID(session.TaskID)`. Authorize off the row you already read rather than calling `AuthorizeSessionAccess`, which re-reads the session. Batch helpers that take ID *lists* (`BatchGetSessionsForTasks`, `GetPrimarySessionInfoForTasks`, …) are exempt by convention: their callers derive the IDs from an already-authorized task list, so per-ID checks would add N queries to list views for no gain — keep it that way, and don't hand them caller-supplied IDs.
- **Two session paths bypass the task service and must guard themselves.** (1) Handlers that resolve an execution by a bare in-memory lookup (`GetExecutionBySessionID`, `*BySessionID`) skip the `GetOrEnsure*` chokepoint where the lifecycle check runs — call `service.AuthorizeSessionAccess` (see `ProcessHandlers.denySessionAccess`) or `lifecycleMgr.CheckSessionAccess` first. (2) The orchestrator reads sessions through its own repo handle, so it inherits nothing from the task service; session-keyed entry points there call `authorizeSession` / `authorizeTask`, wired by `SetSessionAccessChecker` / `SetTaskAccessChecker`. An entry point that accepts **both** a task and a session ID must use `authorizeTaskSessionPair`, never `authorizeSession` alone: the caller can satisfy a session-only check with one of their own sessions while pointing `taskID` at someone else's task, which the method then uses for its task-scoped work. Put the guard **first** in the method, before any repo/executor/agent-manager use — `TestSessionKeyedEntryPointsGuardBeforeDependencies` runs each entry point with nil dependencies and fails on a panic if a guard is placed too late.
- **Dispatched WS actions have a gateway backstop.** `Client.authorizeAction` (`internal/gateway/websocket/dispatch_scope.go`) checks any payload carrying `task_id` or `session_id` against the client's identity *before* dispatch, so a newly added action is scoped by default rather than only when its author remembers. Handler-level scoping is still required (defense in depth, and it covers non-WS callers) — do not delete a service-layer check because the backstop exists. The backstop reads `task_id`, `session_id` and `task_environment_id`, plus `id` on **top-level `task.<verb>` actions only** (`task.state`, `task.move`, `task.get`, … — `task.` and exactly one more segment). Keep using those names for task-scoped actions, because an action that invents a new name for the same kind of resource silently opts out. (`user_shell.stop` was exactly that: it keys off `task_environment_id` with `task_id` optional, so a `task_id`-only backstop missed it. `task.state` and `task.move` were the second instance: they name the task `id`, so the backstop parsed no refs and let one user mutate any other user's task — hence the `id` rule.) Deeper `task.*` namespaces are deliberately excluded: in `task.plan.revision.get` and `task.review.finding.update` the `id` names a revision or a finding, so those actions must carry `task_id` when they are task-scoped. Also avoid permissive types (`any`, `json.RawMessage`) for those fields: a payload that fails to unmarshal into the ref struct is treated as naming nothing.
- **WS**: clients carry their identity; dispatched actions and subscriptions are scoped; workspace-carrying events route via `Hub.BroadcastToWorkspace`. A new `hub.Broadcast` (global) call site needs a `//ws:global` justification comment. Subscription actions (`*.subscribe`) `return` before reaching the dispatch backstop (`Client.authorizeAction`), so scoping a new subscribe topic means adding a hook to `SubscriptionAccessPolicy` (`internal/gateway/websocket/access.go`) *and* calling it explicitly inside that action's own handler — the backstop's `scopedActionRefs` cannot reach it. `run.subscribe`'s `Subscriptions.Run` hook (added for WO-02, resolving the run's workspace via `runs.GetRunWorkspaceID`) is the worked example.
- **Task overview vs. session detail:** Keep cross-task state in the bounded, persisted `TaskStatusSummary` projection and `task.status_summary.updated`; keep unbounded transcript and execution data session-scoped. The [bounded task-status spec](../../docs/specs/platform/bounded-task-status-delivery.md) and [accepted ADR](../../docs/decisions/2026-08-01-separate-task-summary-session-stream-traffic.md) define the shared contract.
- **Port-proxy subtree capability:** with auth enforced, `/port-proxy/:sessionId/:port/*` needs a credential on *every* request, but browser subresource fetches cannot always carry one (the `<link rel="manifest">` fetch never sends cookies; sandboxed iframes and `?token=`-opened previews drop them elsewhere). `PortProxyHandler` mints a short-lived (15 min, sliding), path-scoped, HMAC-signed capability after the document authenticates and propagates it as a cookie scoped to the proxy subtree plus a `kandev_cap` query parameter appended to rewritten asset URLs. `requireConnectionAuth` (`internal/gateway/websocket/access.go`) accepts either form only for the exact session:port it was minted for and restores the issuing identity, so `CheckSessionAccess` still gates on the real owner. Synthetic identities (auth disabled) skip minting — proxied bodies stay byte-identical to the pre-auth behavior.
- **Self-authenticating webhooks** (automation and office channels) and `/health`, `/api/v1/features`, `/api/v1/app-state` stay public — the allowlist lives in `auth/httpmw/middleware.go` with a pinning test. Plugin webhooks are not allowlisted: `auth/httpmw` structurally defers only GET/POST relay paths because it cannot read manifests, and `internal/plugins.Controller.webhook` enforces each manifest's `access` policy.

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

- **Launcher quoting:** Quote interpolated environment values for POSIX/Git Bash
  versus native Windows; when launch recipes change, extend
  `scripts/check-make-shells` with default, spaced, and quote-containing values.

## Conventions

- Provider pattern for DI; stderr for logs, stdout for ACP only.
- Pass context through chains; event bus for cross-component comm.
- **Dependency direction:** Shared admission limits used by task models and orchestrator/messagequeue belong in a neutral internal package; task/models must not import the higher-level orchestrator/messagequeue package.
- Production Git commands must use `subproc.NewGitCommand` with a classified `subproc.RunGit*` helper, or hold a classified admission slot across streaming `Start`/`Wait`. Do not construct raw Git commands outside `internal/common/subproc`; choose `interactive`, `lifecycle`, or `background`.
- **Cross-tier shared code belongs in `internal/common/`.** agentctl ships as a standalone binary uploaded into containers, so neither it nor the backend may own code the other imports — put shared logic in a neutral `internal/common/*` package instead of importing across the boundary or forking a copy (`ptyexec` is the worked example; `subproc`, `gitref`, `securityutil` are older ones). Duplicating to "avoid the dependency" is how the two PTY copies drifted.
- **Startup configuration ownership:** `internal/common/config/catalog.go` and `source.go` own the stable operator catalog, discovery, precedence, provenance, and typed values; managed agentctl children receive a private subset contract. Do not reparse stable env in executors or copy YAML into public child environments.
- **Pure computation that only matters on Windows should not carry `//go:build windows`.** One CI job runs on Windows and it tests a package allowlist, so tagged code is easily unverified — loginpty's copy of the Win32 quoting was compiled by no job at all. Keep platform API calls behind the tag, leave string/path helpers untagged, and add packages needing native coverage to the `test-windows` job in `.github/workflows/backend-tests.yml`.
- **Event-bus wildcard parity:** New NATS wildcard subscriptions must verify equivalent `MemoryEventBus` semantics in `go test ./internal/events/bus`.
- **Repository provider identity:** Provider-backed repositories are keyed by workspace, provider, normalized `provider_host` origin, full owner/namespace, and name. Persist `provider_host` when importing or resolving a remote; do not infer self-managed GitLab rows from owner/name alone. Legacy rows with an empty host have unknown identity and must fail closed for provider write/link operations.
- **Repository provider branches:** Native task branch pickers derive provider identity only from the persisted workspace repository. GitHub uses the first-party service; manifest-owned providers route through the owning active plugin's workspace-scoped `repositories.branches` action. Do not add provider-specific task-service branches or accept browser-supplied repository descriptors on this path.
- **PR status sync coverage:** A `github_pr_watches` row is unique per (session, repository, branch). A watch that has **found a PR** is never re-pointed: it is that PR's only sync handle, and both the poller and the on-demand sync iterate watches, so re-pointing it froze the PR at its last-observed checks/review state — which the UI renders as an open, mergeable PR long after it merged, and which silently disabled CI auto-fix and auto-merge for every branch but the one currently checked out. Branch switches (`resetPRWatchForBranchSwitch`) and the git-status branch sync (`syncPRWatchBranch`) therefore only re-point a **searching** watch (`pr_number=0`) of the same repository, and otherwise add a watch for the new branch — one searching watch per (session, repository) plus one per PR actually opened. Watches are still not a complete index of a task's PRs (sessions get deleted, PRs get linked by URL), so the orphan sweep stays. `service_pr_unwatched.go` reconciles the `github_task_prs` rows no watch points at, and deliberately writes **lifecycle fields only** (`state` / `merged_at` / `closed_at`) — check and review aggregates belong to the watch-driven sync that actually fetches reviews and check runs, and the per-PR REST status marks its counts populated even with nothing to count, so widening this path to aggregates persists 0/0 over good data (it broke six PR-popover e2e specs exactly that way). Terminal and detached rows are excluded so the set stays bounded. **The backend is the only writer of `github_task_prs`:** the PR-feedback read makes the same upstream calls the status sync does (`GetPR` + `ListPRReviews` + `ListCheckRuns`, plus comments), so it persists through `SyncTaskPR` (`service_pr_feedback_sync.go`) and lets `github.task_pr.updated` converge every surface. The frontend must never patch PR lifecycle fields into its own store copy — that write cannot reach the DB, so it desyncs the panel from the kanban card and evaporates on reload (it also masked the sync hole above for months). Both derivations share `newPRStatus` so they cannot drift; the persist rides *inside* the TTL cache's fetch closure, so a cache hit costs no write; failures are logged and swallowed because this is a read endpoint; and the row lookup is workspace-scoped (`ListTaskPRsByPRNumber`) to match the credential scope the read was authorized under. Finally, `merged` is the one state `SyncTaskPR` refuses to leave (`resolveTerminalMergeState`) since GitHub cannot un-merge a PR — do **not** generalize that into a `merged > closed > open` rank, because `closed -> open` is a real transition (a PR can be reopened) and ranking it as a regression pins reopened PRs closed forever. **The batched branch query's empty answer is authoritative:** an alias resolving with **zero** nodes has definitively said "no open PR on this branch", so `branchBatchResult.ResolvedEmpty` carries that to `applyBatchedSearchingWatch`, which must not re-ask it through a per-watch `client.FindPRByBranch`. That re-ask was one `gh pr list` (GraphQL) plus one `gh api /repos/...` per searching watch per cycle *forever*, since a branch with no PR keeps that shape — measured at 5,200-9,200 GraphQL points/hour against a 5,000/hour limit with 42 searching watches, parking the poller on `github rate-limited` ~40 min into every reset window. On a definitive negative only the fork **parent** (a repository the batch never asked about) is worth a look, and `forkParentRepositoryForLookup` caches fork identity per (credential scope, owner, repo) since that answer is constant. Two-or-more open PRs is **not** definitive — `selectBatchedBranchPRNode` refuses to guess — so that case and an unresolved alias keep the full fork-network fallback, throttled on the watch's `last_checked_at` (`PRSyncFreshnessWindow`) exactly as `triggerPRDetection` is; the batched path is the production path, so a throttle guarding only the legacy per-watch path guards nothing. Changes here need a **call-count** test (`service_pr_watch_batched_budget_test.go`) — an assertion on the resulting `PRStatus` passes with the whole regression restored.
- **Execution access:** Workspace-oriented handlers (files, shell, inference, ports, vscode, LSP) MUST use `GetOrEnsureExecution(ctx, sessionID)` — it recovers from backend restarts by creating executions on-demand. Only use `GetExecutionBySessionID` for operations that require a running agent process (prompt, cancel, mode).
- **Generated-title ownership:** Claim generated-title ownership only after task, config, and Office eligibility is known. Config tasks, Office/External modes, and workspace-only preparation (`StartAgent: false`) must never claim it; only an eligible agent-start task may claim the generated title.
- **Task lifecycle events:** Any code path that mutates a task row must publish via the event bus (`task.created` / `task.updated` / `task.deleted`) — either by going through `Service.CreateTask` / `UpdateTask` / `DeleteTask` / `ArchiveTask`, or by calling `publishTaskEvent` (or one of the `Publish*` helpers in `service_events.go`) directly. Walking `repository.TaskRepository` straight bypasses event publishing and breaks WS-driven UI like the All-Workflows kanban view. `HandoffService`'s cascade methods learned this the hard way — they now require a `TaskEventPublisher` wired via `SetTaskEventPublisher`. New cascade / bulk / cleanup paths must follow the same pattern. **Workflow steps** follow the same rule with their own publisher: step create/update/delete publish `workflow_step.created` / `.updated` / `.deleted` through `internal/workflow/stepevents.Publisher` (the WS gateway fans them out; the orchestrator re-evaluates queue admission off `.updated`). Both mutation surfaces — REST/WS in `internal/workflow/handlers`, MCP in `internal/mcp/handlers` — share that publisher and differ only in the source label they pass, so a new step-mutation path uses it rather than hand-rolling the payload; promoting a start step demotes the previous one, so also publish `.updated` for every entry in `DemotedStartSteps`.
- **Workspace deletion side tables:** Any workspace-scoped integration side table without a database foreign key/cascade must be deleted explicitly from its `WorkspaceDeleted` handler. Cover the create → delete lifecycle in a test, and include the same cleanup in E2E reset fixtures; do not assume deleting the workspace row removes orphaned integration metadata.
- **Testing:** Prefer `testing/synctest` (Go 1.24+) over `time.Sleep` for time-dependent tests. Use `synctest.Test` to wrap tests with tickers or timeouts — it advances fake time instantly when all goroutines are idle. When `synctest` is not feasible (e.g., tests spawning external processes like `git`), use channel-based synchronization (`<-started`, non-blocking `select`) instead of sleep-based waits. Reserve `time.Sleep` only for integration tests that need real subprocess execution time.
  - **Test cleanup:** Register `t.Cleanup` immediately after creating resources that need teardown (adapters, `io.Pipe` writers, background goroutines) — before any `t.Fatal`/`t.Fatalf` path. Late cleanup registration leaks pipes and goroutines on early failure.
  - **Joining production goroutines in tests:** When code spawns untracked goroutines (e.g. `fireWakeup`), don't rely on arbitrary sleeps. Join via an observable side effect — e.g. block on `EventTypeComplete` from `a.updatesCh` after unblocking the fake agent. Use short timeouts (~100ms) for in-process negative assertions; reserve multi-second waits for subprocess/integration tests only.
  - **Path/security tests:** Avoid using the real filesystem root as a fixture root. Build fake absolute roots under `t.TempDir()` with `filepath.Join`; this keeps tests portable across Windows, POSIX, and privileged cloud executors. For error assertions, do not compare raw `%q`/escaped strings with native `filepath` values; assert typed fields or normalize both sides with `filepath.Clean`/`filepath.FromSlash`.
  - **Filesystem permission tests:** Assert permission-denied behavior only after probing that the current executor enforces the permission bit change. Root-like Sprite executors may bypass `chmod` restrictions.
  - **Filesystem safety checks:** Carry the original `os.Lstat` `FileInfo` (or opened handle) through every decision; do not re-stat a validated path, which reopens a TOCTOU window before side effects. Test mismatches before writes or manager commands.
  - **Git indexed-environment tests:** When a test supplies `GIT_CONFIG_COUNT`, `GIT_CONFIG_KEY_n`, and `GIT_CONFIG_VALUE_n`, clear every inherited indexed key first and restore it with `t.Cleanup`. `internal/gitconfigenv` ignores indexes past the count; uncleared parent indexes silently alter the block instead of failing loudly.
  - **Derived environment contracts:** Drive derived values through the production producer/wiring seam instead of manually seeding keys; pair consumer/process coverage with a producer-boundary assertion so missing publication fails.
  - **GitLab host-trust tests:** `internal/gitlab`'s `TestMain` in `goleak_test.go` calls the scrub in `env_hermetic_test.go`; `internal/agentctl/server/process`'s `TestMain` clears ambient `KANDEV_GITLAB_HOST`/`GITLAB_HOST`/`GITLAB_TOKEN`. `test_ambient_env` runs both poisoned to catch a regression neither package's clean-env CI shard could catch.
  - **Full test output:** For local full-suite pass/fail validation, prefer plain `go test -race ./...`. `go test -json ./...` can emit very large JSONL streams; if a wrapper or tracing tool truncates the stream mid-record, downstream JSON parsing may fail even when Go tests passed. Use JSON output mainly for CI artifacts or test-report tooling that explicitly requires it.
  - **Private executable helpers:** When library code starts `os.Executable()` in a private helper mode, do not rely only on a command package's dispatch or package-specific `TestMain`: an importing package's test binary can be the executable and recursively run tests.
    Use either an explicitly marked private env-and-argument trampoline before normal test entry, or inject a dedicated helper executable. Cover it from a different importing package, not only the helper package.

### Goroutine ownership and leak testing

Every long-running goroutine must have a single owner with explicit start and stop semantics:

- **Lifecycle:** the type that spawns the goroutine also exposes `Start(ctx)` / `Stop()` (or equivalent). `Start` registers on a `sync.WaitGroup`; `Stop` cancels the goroutine's context (or closes a `stopCh`) and `wg.Wait()`s for drain. Idempotent on both ends. Restartable services must reset stopped state and create a fresh cancellable context/cancel under the same mutex used by `Stop`; cover `Stop` → `Start`. `internal/integrations/healthpoll`, `internal/jira`, `internal/linear`, and `internal/github` pollers are the canonical shape.
- **E2E reset invariant:** `seedData`/backend are worker-scoped, so any workspace-scoped state a global poller reads (for example `github_review_watches`) must be deleted in `cmd/kandev/e2e_reset.go` before task deletion — otherwise the poller recreates rows mid-reset and later tests see duplicates. Add a `Delete...ByWorkspace` cascade when introducing a new poller-backed entity.
- **Cancellation:** the goroutine selects on `ctx.Done()` (or `stopCh`) in every long wait. Never use `time.Sleep` in a retry/backoff loop — use `time.NewTimer` inside a `select` that also watches the shutdown signal (see `lifecycle.StreamManager.sleepOrStop`).
- **Detached helpers:** event handlers and short-lived `go func()` calls in `internal/orchestrator/` and `internal/agent/runtime/lifecycle/` must accept a cancellable context (or check the owning type's shutdown signal) and return promptly when it fires.
- **Leak testing:** packages that spawn goroutines add `goleak.VerifyTestMain(m)` in a per-package `TestMain`. New packages of this kind must follow suit. When a third-party background goroutine genuinely can't be drained, suppress it with `goleak.IgnoreTopFunction(...)` and leave a comment explaining why. Currently instrumented: `internal/gateway/websocket/`, `internal/agent/runtime/lifecycle/`, `internal/agentctl/server/process/`, `internal/orchestrator/`, `internal/github/`, `internal/gitlab/`, `internal/jira/`, `internal/linear/`, `internal/integrations/healthpoll/`.

## Backups
- On every SQLite boot, `persistence.Provide` reads `kandev_meta.kandev_version`. If the stored version differs from the binary version (or any user tables exist but no version is recorded), it takes a `VACUUM INTO` snapshot into `backups/` beside the configured SQLite database file before running migrations. The default path is `<home>/data/kandev.db`, with backups in `<home>/data/backups/`.
- Retention: 2 backups kept (newest two by mtime); older ones are pruned after the snapshot succeeds.
- Postgres: backup is skipped with a log line. Use `pg_dump` for Postgres backups.
- Boot aborts if the backup fails — the pool is closed and `Provide` returns an error.
- Restore quiesces scheduling, active executions, and database-backed workers, validates the SQLite checkpoint result, and closes the shared pool. It quarantines the configured database and sidecars, installs the staged file, and restores the originals if replacement fails. The frontend requires a restart before database-backed work resumes; PostgreSQL restore is rejected.
- After all repos complete `initSchema`, `cmd/kandev/storage.go:recordSchemaVersion` writes the current binary version into `kandev_meta` (non-fatal; a failure just means the next boot will take a fresh snapshot).
- Migration logging: `db.MigrateLogger.Apply(name, stmt)` — success logs Info, "already exists" / "duplicate column name" is silently swallowed, anything else logs Warn but never returns an error (preserving the existing swallow-error contract).
- Schema replay handling: use `internal/db` helpers such as `IsDuplicateColumnError` / `IsAlreadyExistsError` instead of local error-string matching. When adding or changing startup schema code, include fresh-DB plus same-DB replay tests for SQLite; add the same env-gated Postgres replay coverage when the path supports Postgres. See `docs/decisions/0027-replayable-schema-migrations.md`.

## Schema & migrations (SQLite repository)

`internal/task/repository/sqlite` also runs against PostgreSQL: avoid unguarded SQLite-only `rowid`, JSON, or date syntax and add an environment-gated PostgreSQL behavior test for every changed dialect-sensitive method (schema replay is insufficient).

`initSchema()` in `internal/task/repository/sqlite/base_schema.go` runs the `init*Schema` (CREATE TABLE) steps **before** `runMigrations()`. The table-creation DDL uses `CREATE TABLE IF NOT EXISTS`, so on an **existing** database it is a no-op and never adds columns to a table that is already present.

**Rule:** when you add a column to an existing table, add it **only** via an idempotent `ADD COLUMN` migration in `runMigrations()` (`base_migrations.go`), never by editing the table's `CREATE TABLE` alone. Anything that *references* that new column — an index, a backfill `UPDATE`, a partial-index predicate — must live in `runMigrations()` **after** the `ADD COLUMN`, not in the `init*Schema` DDL. Putting a `CREATE INDEX ... (new_col)` in the schema-init block crashes existing DBs with `no such column: new_col`, because schema init runs before the migration that adds the column.

You may still list the column in the `CREATE TABLE` so fresh DBs get it inline, but the migration is the source of truth for evolution and must stand alone. New columns also need: the struct field in `models/`, the DTO field + `ToAPI` in `pkg/api/v1/`, and every `CreateX`/`UpdateX`/bulk write in the repo that should set it.

Built-in prompt content refreshes are seed-data migrations, not schema migrations. Match only known historical content hashes after applying the same normalization as the embedded prompt loader, require `created_at == updated_at` to preserve user edits, and use a conditional update over the original row values to avoid racing concurrent edits. Keep these refreshes with prompt seeding rather than `runMigrations()`.

## Internationalization

`internal/i18n` renders only browser-facing copy: SPA-unavailable pages and shared-task artifacts. Diagnostics, logs, agent/ACP output, and CLI output remain English. Use `i18n.T`/`i18n.Tf` with explicit locale threading (including interpolation/plurals); resolve artifact locale at creation. Catalogs are embedded in `internal/i18n/locales/`; regenerate `pseudo` with `pnpm run i18n:pseudo`.
Prefer stable error codes for new output so the frontend translates it. See `docs/i18n.md` and ADR `2026-08-01-share-artifact-locale.md`.

**Table-rebuild migrations:** When a legacy or constraint migration recreates a table, mirror every new column in the replacement `CREATE TABLE` and `INSERT ... SELECT` copy list; add a replay regression test proving values, including timestamps, survive. **Destructive cutover migrations:** Build a legacy schema with `NewWithDB` on a fresh database, replace final-shaped tables with legacy-shaped ones, inject a test-only failpoint after each cutover step, and assert byte-equivalent rollback; run the same matrix with `KANDEV_TEST_POSTGRES_DSN`, looking up PostgreSQL constraint names dynamically because they truncate at 63 bytes.

## Code-quality limits

Enforced by `apps/backend/.golangci.yml` (errors on new code only):
- Functions: ≤80 lines, ≤50 statements · Cyclomatic complexity: ≤15 · Cognitive complexity: ≤30
- Nesting depth: ≤5 · Naked returns only in functions ≤30 lines · No duplicated blocks (≥150 tokens) · Repeated strings → constants (≥3 occurrences) · Revive's 800-effective-line file limit also applies to test files; put new tests in a new file instead of appending to an already-large test file.

When a PR fixup touches backend code, run the CI-style changed-file linter locally from `apps/backend` with the PR base SHA before pushing, because CI enforces changed-file complexity thresholds:

```bash
golangci-lint run ./... --new-from-rev="<base-sha>" --timeout=5m
```
## Further scoped notes

- `internal/launcher/` — native launcher owning every entrypoint (`dev`, `start`, `run`, `service`); `dev` runs `make -C apps/backend dev` with Vite as a supervised child, state under `<repoRoot>/.kandev-dev/`. The root `make dev` prebuilds only the copied launcher; the backend dev target builds the native agentctl and a linux/amd64 helper when the host is not Linux/amd64 (`docs/plans/go-dev-launcher/`).
- `internal/agentctl/AGENTS.md` — agentctl server route groups, adapter model, ACP protocol
- `internal/agentctl/server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`), iframe-blocking header stripping
- `internal/integrations/AGENTS.md` — playbook for adding a new third-party integration (Jira/Linear pattern)
- `docs/i18n.md` ("Backend") — `internal/i18n` covers only what Go renders straight to a browser: the SPA-unavailable error pages and the shared-task artifacts (`share.html`, gist README and description). Both are complete. Everything else stays English by design; for new user-facing output prefer a stable error code the frontend translates. Use `i18n.Tf` for anything carrying a value — never `fmt.Sprintf` a translated string, and never build a plural in Go. A locale for output that outlives the request is resolved once at write time and threaded as an argument, not a context value (ADR `2026-08-01-share-artifact-locale.md`).
- `cmd/mock-agent/AGENTS.md` — predefined `/e2e:<name>` scenarios vs inline `e2e:...` scripts, recipe for adding a scenario, and the rebuild-before-e2e requirement
