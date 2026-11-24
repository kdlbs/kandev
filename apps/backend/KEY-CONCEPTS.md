## Key Concepts

### Orchestration and integrations

**Orchestrator** coordinates task execution:
- Receives task start/stop/resume requests via WebSocket
- Delegates to lifecycle manager for agent operations
- Handles event-driven transitions via workflow engine; capacity evidence is versioned, prompt-scoped, and separate from transport-loss restore
- Located in `internal/orchestrator/`; capacity continuation needs explicit provider evidence on the retained runtime and never restores after effects

**Cancellation progress projection:** `orchestrator.Service.CancellationPending(sessionID)` is a runtime-only, session-scoped view of accepted cancellation work. Serialization that carries the boolean with ordering identity uses the atomic `CancellationPendingSnapshot(sessionID)` provider, whose process-local revision increments on first-begin and last-end transitions.
The task DTO package exposes both the compatibility boolean provider and snapshot seam; boot state, task-session HTTP/WS lists and detail responses, and the session-scoped WebSocket notification must project explicit `true`/`false` values plus the revision.
Keep count, revision, and publication queue updates in one critical section, drain event-bus sends outside it, and never persist this transient marker or turn it into a coarse session lifecycle state.

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

### Workflow engine

`internal/workflow/engine/` provides typed state-machine evaluation:
- `Engine.HandleTrigger()` evaluates step actions for triggers (on_enter, on_turn_start, on_turn_complete, on_exit)
- `TransitionStore` interface abstracts persistence (implemented by `orchestrator.workflowStore`)
- `CallbackRegistry` maps action kinds to callbacks (plan mode, auto-start, context reset)
- First-transition-wins: multiple transition actions in one trigger, first eligible wins
- `EvaluateOnly` defers transition and data persistence to the caller, which orchestrates on_exit → DB → on_enter. Evaluation still runs callbacks and can mark an operation applied when no transition is deferred.
- `RequiresApproval` on actions: transitions requiring review gating are skipped
- Idempotent by `OperationID`; session-scoped data bag via `MachineState.Data`

### Agent runtime

`internal/agent/runtime/` owns launch, resume, stop, and observation through `runtime.Runtime`. Its facade delegates to the lifecycle manager. Run-owned executions use `runtime.LaunchSpec.Owner` (`kind=run`) with durable run-session identity. Admission fails closed before allocation and lifecycle registration. `runtime.Start` rolls back failed startup. Task launches retain task and session checks.

**Run scheduling ownership:** `internal/runs/` is generic; `internal/runs/models` owns the shared run-row and run-event data contracts. Only `internal/backendapp/` constructs and owns the single `internal/runs/scheduler` and its lifecycle. Office adapters may depend on runs, but generic runs must not import `internal/office` or its subpackages. Office retains launch, causation, routing, and backpressure policy. See ADR `2026-09-26-run-contract-ownership`.

**Runtime environment invariant:** `Agent.Runtime().Env` applies to every ACP subprocess entry point. Route new overrides through host-utility probes and sessionless prompts into agentctl child processes before sanitization; cover probe DTO, prompt DTO, and child-process boundaries. Host utility probes must use the same profile-resolved `HOME`, `GH_CONFIG_DIR`, and credential-selection inputs that the eventual launch receives; compose structured env blocks once and propagate them across create/configure/start/reconfigure, test both profile/host mismatch directions, and never emit secret or token values.
**System storage cleanup:** Install-wide cleanup providers live under `internal/system/storage/` and share the storage mutation gate for conflicting cache operations. Go-cache cleanup deletes eligible build data in place, preserving its root, ownership marker, and root `fuzz` corpus. It validates mount identity at traversal and removal boundaries; a missing mount identity must fail closed. Threshold discovery and deletion are bounded and resume across calls in the same process. The persisted `go_cache.allow_cleanup_while_busy` option bypasses activity and idle admission only for Go-cache cleanup; other providers retain their existing gates.
**Convention:** only `internal/agent/runtime/` (and code that pre-dates Phase 1 migration) may import `runtime/lifecycle` or `runtime/agentctl` directly. New consumers — workflow engine actions, cron-driven trigger handlers, future task-tier callers — should depend on `runtime.Runtime` or narrow local interfaces for the lifecycle-owned capability they consume. Existing call sites are migrated through later phases of task-model-unification.

**Lifecycle Manager** (`internal/agent/runtime/lifecycle/`) manages agent instances under the runtime:
- `Manager` (`manager.go`, `manager_*.go`) - central coordinator for agent lifecycle
- `ExecutorBackend` interface (`executor_backend.go`) - abstracts execution environment (Docker, Standalone, Sprites, SSH, Kubernetes, Remote Docker)
- `ExecutionStore` (`execution_store.go`) - thread-safe in-memory execution tracking
- `session.go` - ACP session initialization and resume
- `streams.go` - WebSocket stream connections to agentctl
- `process_runner.go` - agent process launch and management
- `profile_resolver.go` - resolves agent profiles/settings
**Lifecycle callback identity:** Process callbacks must retain the launched PID and generation captured when scheduled, revalidate both before state/I/O, and ignore delayed callbacks from replaced processes and duplicates; test replacement before start and after waits. Prompt callbacks and asynchronous prompt errors must likewise carry immutable execution, session, prompt-generation, and turn evidence captured at the result boundary; never reread mutable execution snapshots later, because a successor prompt may already own them. Settle through the correlated terminal path and test both replacement-execution and same-execution successor-prompt races. Event handlers must not synchronously call lifecycle-manager methods that can reacquire a lock held by the event publisher; terminal event payloads must carry immutable identity/admission evidence for that boundary. Add a regression that blocks any manager generation read while publishing the terminal event. Retained agentctl adoption restores its authenticated descriptor, SQL generation, stream owner, projected cursor, and active submission before intake or prompt admission; replay uses bounded pages through the normal inbox, canonical projection, lifecycle, and ACK path. Do not call ACP initialize, load, resume, or new during adoption. Identity, journal, cursor, or projection errors fail closed and keep Stop available. `features.agentSurvival` controls process lifetime only.

**agentctl client** (`internal/agent/runtime/agentctl/`) is the HTTP/WS client used by the lifecycle manager to talk to a running agentctl instance. It is a runtime-tier package and should not be imported outside `internal/agent/runtime/`.
`RuntimeOwner` and `RecoveryCoordinator` in this package own local agentctl availability, runtime leases, and bounded in-process replacement. The system admin retry route fences requests by backend boot, runtime epoch, and snapshot revision. The child-kill route under `/api/v1/e2e/` is mounted only when `KANDEV_E2E_MOCK=true`.

**Agent discovery vs. ACP probing:** discovery answers only whether an agent executable is available; authentication, protocol compatibility, and supported models or modes belong to the ACP probe path, not installation gates. MiniMax uses native `mcode acp`, dynamic encoded model IDs and executor-owned `~/.minimax`; do not relocate its auth files because their identity includes the absolute auth-home path.

**agentctl** is an HTTP server that:
- Runs inside Docker containers or as standalone process
- Manages agent subprocess via stdin/stdout (ACP protocol)
- Exposes workspace operations (shell, git, files)
- Supports multiple concurrent instances on different ports

Standalone agentctl is launched in its own process group so terminal Ctrl+C is handled by the backend lifecycle manager first; do not share the backend's foreground group, which bypasses supervised shutdown and can leak ACP subprocesses.

### Executors

Executor types in the database model:
- `local_pc` - Standalone process on host
- `local_docker` - Docker container on host
- `sprites` - Sprites cloud environment
- `ssh` - Remote SSH host
- `k8s` - Namespaced Kubernetes Pod with optional PVC workspace
- `remote_docker` - Container on a Docker daemon reached over SSH; `remote_vps` - Planned

**Kubernetes lifecycle:** `task_environment_kubernetes` owns shared physical Pod/PVC inventory; `executors_running` records individual sessions and legacy session-owned pods. Persist the exact Pod/PVC names, UIDs, full `kandev.ai/*` identity, workload snapshot, and internal runtime-secret references before reporting a launch as durable. Session stop deletes only its agentctl instance, and backend shutdown preserves resources; task cleanup deletes the Pod and only a Kandev-created PVC after exact identity checks and confirmed absence. Reconnect uses the current executor connection config but the recorded workload/resource snapshot, and any ambiguity fails closed. Keep agentctl reachable only through a process-local loopback port-forward; never add a Service or place resolved credentials in a Pod spec.

**Remote SSH executor platforms:** Treat supported remote OS/arch values as an end-to-end contract. Platform probe/normalization, lifecycle support checks, agentctl helper resolution, platform default shell, SSH readiness endpoints, frontend response types, and tests must stay aligned. Preserve raw unsupported platform details in user-facing errors, but use normalized values for supported-platform matching. Keep shell defaults platform-aware: Darwin defaults to `zsh`, Linux defaults to `bash`, unless an explicit shell is saved.

**Remote SSH lifecycle:** Resolve and run remote preparation, then verify the canonical checkout, origin, and `HEAD` before starting agentctl. Retain the remote task directory in session state for stop/resume. On a terminal archive/delete/cascade stop the profile's `cleanup_script` runs over the live SSH connection before teardown; `StopInstance` never removes the task directory itself, on any stop reason. Ordinary stop and backend shutdown preserve the workspace. Reclaiming the task directory is an opt-in phase of the durable task-resource cleanup job, not part of the stop path (see `docs/specs/executors/system-design/remote-task-directory-reclamation.md`). Every stop reason except graceful backend shutdown kills the remote agentctl process and removes its per-session runtime directory.

For authentication, authorization, workspace scoping, and gateway security, read [authentication guidance](AUTHORIZATION.md).
