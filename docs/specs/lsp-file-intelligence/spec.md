---
status: shipped
created: 2026-07-09
updated: 2026-08-12
owner: tbd
---

# LSP File Intelligence

## Why

Users need project-aware diagnostics and navigation inside Kandev without repeatedly paying a
language server's startup and project-analysis cost as they move between files, sessions, panels,
or browsers. Long Kotlin imports make a browser- or editor-owned lifecycle especially disruptive
and obscure the actual task-level work that is still running.

## What

- Kandev provides task-host language-server intelligence for TypeScript/JavaScript, Python, Go,
  Rust, and experimental Kotlin. Monaco uses the capabilities each initialized server advertises:
  diagnostics, completion, hover, definition, references, signature help, and semantic tokens.
- There is exactly one logical lifecycle per `(task_id, language)`. Every session, editor, tab,
  browser, and device authorized for that task observes and controls the same language server.
  No user-visible LSP contract is keyed by `session_id`.
- Each language has a task policy:
  - **Inherit default** follows the global auto-start default after bounded task-host discovery
    detects that language.
  - **Keep warm** starts or restores the server whenever the task environment is available,
    regardless of open files, and keeps it running until a task-level transition stops it.
  - **Disabled** keeps the language off and prevents editor mounts, reconnects, and global defaults
    from reacquiring it.
- Explicit **Start** sets **Keep warm** and converges toward a running server. Explicit **Stop**
  sets **Disabled**, cancels pending recovery, gracefully stops the server when possible, and
  reaps its process tree. **Restart** preserves the current policy, explains that project analysis
  may run again, stops the current generation completely, and creates exactly one new generation.
- Opening or closing a file, changing the active file or panel, switching sessions in the same
  task, unmounting an editor, closing a browser attachment, navigating, reconnecting, reloading,
  or passing the former two-minute idle boundary never changes task policy or stops a warm server.
- A backend or browser may reattach to an initialized task-host generation. If the task host no
  longer exists, Kandev reports recovery and starts at most one replacement generation when the
  effective policy requires it. Missing backend cache state is not absence: an existing-only
  physical reattachment that cannot launch or resume resources precedes every replacement.
- Supported project languages are discovered without opening a matching file. Discovery reads
  names and extensions only; it never starts or installs a language server, evaluates a manifest,
  or triggers a project import. Each scan is limited to two seconds, 10,000 filesystem entries,
  and depth six beneath the task root and ordered repository roots. It does not follow directory
  symlinks and skips `.git`, `.kandev`, `node_modules`, `vendor`, `target`, `build`, `dist`, and
  `.gradle`. A truncated scan is visible as partial rather than as a false complete result.
- The task control lists every status-visible supported language so a user can start one even when
  discovery is pending or finds no matching file. Every language is visible by default. A global
  user preference may hide individual languages from task aggregate/status surfaces without
  changing discovery, task policy, process state, or the current-language editor shortcut.
  Detected, explicitly configured, running, queued, importing, restart-required, and failed visible
  languages are considered relevant for the aggregate summary.
- Global editor settings remain defaults for auto-start, permission to auto-install, and the JSON
  configuration returned through `workspace/configuration`. Auto-start now reacts to task
  discovery and task-environment readiness, not to a file mount. Saving configuration updates one
  live task-host server through `workspace/didChangeConfiguration` without spawning another.
- The old LSP status-location preference no longer moves or hides lifecycle ownership. The editor
  toolbar is always a shortcut for its current supported language, while the task aggregate uses
  the application status surface. Stored legacy `lsp_status_location` values are ignored during
  compatibility migration and do not change either surface.
- With the Application status bar enabled, a stable compact task item remains visible whenever the
  task has a relevant language, even when another file, an unsupported file, or a non-file panel is
  active. One relevant language can read `Kotlin · Importing 4m`; multiple live languages can read
  `LSP · 2 running`. Server text and elapsed time are evidence, not inferred completion.
- Opening the task aggregate shows one compact collapsible row per visible language. The collapsed
  summary preserves language, lifecycle state, and detection evidence; expanding a row reveals
  policy, effective policy, server-reported work, elapsed time, generation, start time, last
  stop/restart reason, initiator, actionable errors, and Start/Stop/Restart controls. The task
  policy picker uses the shared select control. A Restart confirmation states that the current
  server and analysis are discarded and a project import may run again.
- When the Application status bar is disabled or no language is relevant enough for its compact
  item, an always-discoverable task/workspace action opens the same controller. This action remains
  available when every language is hidden and directs the user to visibility settings. The editor
  toolbar shortcut delegates to that controller, force-shows its current language for that
  disclosure, and never creates a separate lifecycle.
- Phone and coarse-pointer tablet layouts expose the same task value and actions through the
  existing Status drawer or task action. The language rows render inline in that drawer rather
  than opening another drawer. Touch targets are at least 44 px; the drawer has one scroll owner,
  honors safe areas and dynamic viewport height, remains contained, and causes no document-level
  horizontal overflow.
- The phone file viewer does not attach a protocol client or start a language server because a file
  opened. A user can still inspect policy and explicitly Start, Stop, or Restart the task server
  from the task-level mobile surface.
- Lifecycle presentation distinguishes admission, installation, process launch, process started,
  the LSP `initialize` request, server-reported work, initialized ready/idle, stopping, unsupported,
  unavailable, and error states. It never calls a started process ready before `initialize`
  succeeds.
- Initialization has no automatic timeout. After 60 seconds the UI calls out a long wait while
  retaining Stop. Kotlin may name Gradle import as a possible cause, but Kandev never invents an
  indexing percentage, completion signal, or ETA.
- Standard LSP work-done `begin`, `report`, and `end` events are shown with server title, optional
  message, optional clamped percentage, local elapsed time, and concurrent-work count. The oldest
  active item is primary; unrelated percentages are never averaged. Ending one item means only
  that the server ended that item.
- A task host stores one initialized JSON-RPC peer per language and multiplexes browser
  attachments. It remaps request IDs by attachment and generation, handles client-side protocol
  requests centrally, and caches capabilities, progress, and diagnostics for reconnecting
  attachments. A browser never sends `initialize`, `shutdown`, or `exit` as an ownership action.
- Shared document synchronization is deterministic. One canonical task-host URI is open upstream;
  duplicate attachment opens add references without rewinding text, accepted changes receive
  monotonically increasing upstream versions in arrival order, and the last accepted change is the
  server's analysis overlay. The final document attachment may send `didClose`, but that does not
  stop the server. Kandev does not overwrite another editor's unsaved buffer. Every browser-originated
  file URI is canonicalized and must resolve inside the backend-authorized task workspace or one of
  its repository roots; sibling paths, traversal, and symlink escapes fail before upstream traffic.
  Lexical task-root and authority/volume checks happen before filesystem resolution. Trusted
  task-host setup opens and pins one OS root handle per authorized root before any browser attaches,
  then derives and verifies the canonical identity from that same pinned handle. It never resolves
  one root lookup and reopens the resulting pathname as authority. Windows junction and mount-point
  roots therefore cannot defer their redirect until a browser child lookup. Browser authorization uses only
  handle-relative inspection, including across concurrent ancestor replacement. Symlink and Windows
  reparse targets are projected into the authorized root set before target lookup, with valid
  cross-root links switching pinned handles, so neither a direct UNC URI nor an in-root redirect to an
  untrusted share can trigger network access merely by being rejected. Missing document tails stop
  lookup at the first absent component. Since LSP servers consume pathnames rather than file handles,
  task-environment containment remains responsible for replacement after authorization completes.
- Successful saves first synchronize the newest live editor snapshot, then send `didSave` only
  when the server requested it. Stale persisted text is omitted if editing advanced while the save
  was in flight; failed saves send no save notification.
- TypeScript/JavaScript built-ins are suppressed only for task/session Monaco models whose active
  external server advertises the replacement capability. Other tasks, sessions, models, and
  unadvertised features keep Monaco's built-ins.
- Multi-repository tasks use one server. The task workspace is `rootUri`; ordered repository roots
  are `workspaceFolders`. A source-root change uses `workspace/didChangeWorkspaceFolders` when the
  server supports it; otherwise the current generation stays usable for its old scope and the task
  status becomes `restart_required` until the user restarts. A change to the physical task workspace
  root always requires an explicit restart because process working directory, `rootUri`, and
  attachment scope are initialize-time state; folder notifications never pretend to rebind them.
- The canonical `TaskEnvironment` is only a runtime target. All sessions of the task use it. An
  inherited-workspace task may share the physical environment and task-host process with another
  task, but never its language-server slot: agentctl keys runtime, progress, configuration, and
  workspace roots by `(task_id, language)`. Membership is proven from durable session state before
  runtime access. If an environment is replaced, Kandev proves the old generation is reaped before
  automatically starting one new generation in the replacement. An unresolved old runtime blocks
  the new launch with an actionable error instead of risking duplicate imports.
- Local PC/Worktree and Local Docker task environments are supported. Remote Docker, SSH, Sprites,
  missing, and unknown executors fail closed before Kandev acquires capacity or starts/resumes task
  resources. Backend capability data, not a frontend executor-name allowlist, owns that decision.
- Capacity counts task/language processes that are starting, live, or stopping. It does not count
  browser attachments, editor leases, detected languages, or queued desired state. The default is
  eight; `KANDEV_LSP_MAX_SERVERS` overrides it. The legacy `KANDEV_LSP_MAX_CONNECTIONS` value is a
  deprecated fallback only when the new variable is unset. Desired servers wait visibly for a slot
  without starting/resuming a task host and are reconsidered when a slot is released. Slot release
  reserves the next entry atomically and promotes it asynchronously under controller lifecycle, so
  stopping one task never waits for another task's installer or launch. A canceled promotion returns
  its reservation exactly once and advances the queue. Startup reserves every persisted generation
  that may still own a process before fallible runtime inspection; inspection frees it only after
  authoritative absence, while a confirmed stop releases the matching durable/runtime identity. A
  transient recovery error therefore cannot make admission exceed the configured cap.
- Binary discovery, managed installation, and execution retain existing task-host trust
  boundaries. Kandev never executes a server from a repository or relative `PATH` entry. Managed
  npm/release binaries live under the task host's `~/.kandev/lsp-servers`; `gopls` uses the task
  host's Go toolchain. Kotlin remains manual-install-only. Rust auto-install remains limited to
  supported macOS and Linux task hosts.
- Task stop, archive, and delete cancel pending starts/recovery, clear active progress, and
  terminate every LSP process descendant owned by that task. If another live task shares the
  departing owner's physical environment, Kandev transfers environment ownership to that borrower
  and preserves its task-host process and independent language slots. The surviving host purges the
  departing task's stopped slots, snapshots, subscriptions, diagnostic/capability caches, and
  workspace projection. With no live borrower, and on environment teardown, Kandev reaps the full
  task-host process tree. A temporary task stop or archive preserves policy and history for resume;
  task deletion cascades the task-language records.
- The task-scoped controller carries server-stamped initiator metadata (`user`, `agent`, or
  `automatic`) and reason codes. A future task-scoped MCP tool must call this controller with task
  ownership derived from its execution; it must not accept a caller-selected task/session ID or
  create a hidden language server. No MCP tool is included in this scope.

Decision: [ADR-2026-08-05-task-scoped-lsp-ownership](../../decisions/2026-08-05-task-scoped-lsp-ownership.md).

## Data model

`task_lsp_languages` stores durable policy and lifecycle evidence. Rows are created only when a
language is detected, explicitly configured, or has lifecycle evidence; task snapshots synthesize
default entries for the remaining supported registry languages.

| Field                      | Type               | Contract                                                                                                        |
| -------------------------- | ------------------ | --------------------------------------------------------------------------------------------------------------- |
| `task_id`                  | string             | Composite primary key; FK to `tasks.id` with cascade delete.                                                    |
| `language`                 | string             | Composite primary key; one of the registered LSP language IDs.                                                  |
| `policy`                   | enum               | `inherit`, `keep_warm`, or `disabled`; default `inherit`.                                                       |
| `detected`                 | boolean            | Result from the latest completed/partial bounded scan.                                                          |
| `detection_state`          | enum               | `unknown`, `scanning`, `complete`, `partial`, or `unavailable`.                                                 |
| `detection_scanned_at`     | timestamp nullable | Start time of the latest completed/partial scan.                                                                |
| `detection_truncated`      | boolean            | True when any scan budget stopped traversal.                                                                    |
| `phase`                    | enum               | Current durable lifecycle projection described below.                                                           |
| `generation`               | unsigned integer   | Monotonic task/language process-generation identity.                                                            |
| `revision`                 | unsigned integer   | Monotonic projection revision used to reject stale REST/WS data.                                                |
| `process_started_at`       | timestamp nullable | Time the current/last generation's OS process started.                                                          |
| `initialize_started_at`    | timestamp nullable | Time its `initialize` request began.                                                                            |
| `ready_at`                 | timestamp nullable | Time its initialize response succeeded.                                                                         |
| `last_transition_at`       | timestamp          | Time `phase` or activity last changed.                                                                          |
| `last_action`              | enum               | `start`, `stop`, `restart`, `set_policy`, or `reconcile`.                                                       |
| `last_action_at`           | timestamp nullable | Time the last controller action was accepted.                                                                   |
| `last_stop_reason`         | string             | Stable reason code for the last stop, including user, task, environment, or crash reasons.                      |
| `last_restart_reason`      | string             | Stable reason code for the last restart/recovery.                                                               |
| `last_initiator`           | enum               | `user`, `agent`, or `automatic`; stamped by a trusted adapter.                                                  |
| `restart_required`         | boolean            | True when workspace/runtime changes need an explained user restart while the current process may remain usable. |
| `restart_required_reason`  | string             | Stable reason code; empty when `restart_required` is false.                                                     |
| `error_code`               | string             | Empty outside an active failure; localized by the UI.                                                           |
| `error_message`            | string             | Optional authorized task-host detail; rendered as text.                                                         |
| `created_at`, `updated_at` | timestamp          | Row audit timestamps.                                                                                           |

Active progress tokens, server capabilities, cached diagnostics, open documents, downstream
attachment IDs, task-environment IDs, and execution IDs are runtime-only. They are keyed by
task-host language generation, never persisted as browser/session ownership, and never returned as
task ownership fields.

Global settings retain these meanings:

| JSON field                    | Type       | Meaning                                                                                |
| ----------------------------- | ---------- | -------------------------------------------------------------------------------------- |
| `lsp_auto_start_languages`    | `string[]` | Default languages enabled by an `inherit` policy after detection.                      |
| `lsp_auto_install_languages`  | `string[]` | Languages whose missing registered server Kandev may install on a supported task host. |
| `lsp_status_hidden_languages` | `string[]` | Languages omitted from aggregate task status surfaces; all are visible by default.     |
| `lsp_server_configs`          | object     | Per-language JSON used for `workspace/configuration` and live configuration changes.   |

## API surface

All browser routes are task-scoped:

| Method and path                                    | Request                | Result                                                                  |
| -------------------------------------------------- | ---------------------- | ----------------------------------------------------------------------- | ------------- | ----------------------------------------- |
| `GET /api/v1/tasks/:taskId/lsp`                    | none                   | Every supported language plus task aggregate metadata.                  |
| `PUT /api/v1/tasks/:taskId/lsp/:language/policy`   | `{ "policy": "inherit" | "keep_warm"                                                             | "disabled" }` | Accepted authoritative language snapshot. |
| `POST /api/v1/tasks/:taskId/lsp/:language/start`   | empty                  | Sets Keep warm and returns the accepted/queued/current snapshot.        |
| `POST /api/v1/tasks/:taskId/lsp/:language/stop`    | empty                  | Sets Disabled and returns the stopping/off snapshot.                    |
| `POST /api/v1/tasks/:taskId/lsp/:language/restart` | empty                  | Preserves policy and returns the single replacement operation snapshot. |
| `GET /lsp/tasks/:taskId/:language/attach`          | WebSocket upgrade      | Non-owning feature/document attachment to the current generation.       |

Mutation bodies never accept `task_id`, `session_id`, task-environment/execution identity,
generation, initiator, or reason. Invalid language/policy returns `400`; an invisible task returns
the normal not-found response; Restart of a disabled/off server returns `409`. A policy mutation
can succeed while its runtime snapshot is visibly `queued`, `unsupported`, `waiting_for_task`, or
`error`; those states do not cause a hidden execution attempt.

A language snapshot has this stable shape (nullable timestamps and empty error details omitted):

```json
{
  "task_id": "task-id",
  "language": "kotlin",
  "detected": true,
  "detection_state": "complete",
  "detection_truncated": false,
  "policy": "keep_warm",
  "effective_policy": "keep_warm",
  "phase": "initializing",
  "activity": "server_work",
  "generation": 4,
  "revision": 19,
  "process_started_at": "2026-08-05T10:00:00Z",
  "initialize_started_at": "2026-08-05T10:00:01Z",
  "progress": [
    {
      "token": "initialize-4",
      "title": "Importing Gradle project",
      "message": "Resolving dependencies",
      "percentage": 42,
      "started_at": "2026-08-05T10:00:01Z",
      "updated_at": "2026-08-05T10:04:00Z"
    }
  ],
  "last_action": "start",
  "last_initiator": "user",
  "restart_required": false
}
```

The backend publishes semantic `task.lsp_state_changed` events, mapped to browser
`task.lsp.changed` messages carrying one full language snapshot. Clients ignore a lower revision.
The first successful attachment frame is a non-LSP envelope containing `status: "attached"`, the
language, generation, workspace URI/folders, and initialized server capabilities. Raw LSP feature
traffic follows. Lifecycle/progress snapshots can arrive before attachment readiness; closing the
attachment releases only its request/document references.

The internal controller seam used by HTTP, lifecycle recovery, and a future MCP adapter exposes
task snapshot, policy mutation, `start|stop|restart` control, attachment, task teardown, and task
reconciliation operations. Every exported human-facing operation authorizes `taskID` first. Each
control call receives origin metadata from its trusted adapter; origin is not copied from request
arguments. A future in-session adapter resolves `taskID` from `AgentExecution` before invoking the
same seam.

## State machine

`phase` is one of:

| Phase              | Meaning                                                                      |
| ------------------ | ---------------------------------------------------------------------------- |
| `off`              | No process or pending start exists.                                          |
| `waiting_for_task` | Policy wants a server, but the canonical task environment is not ready.      |
| `queued`           | Runtime is supported and desired, but no process-capacity slot is available. |
| `installing`       | A permitted Kandev-managed server installation is active.                    |
| `starting`         | Capacity is held and the task host is launching the registered process.      |
| `process_started`  | The OS process and stdio bridge exist; `initialize` has not started.         |
| `initializing`     | The task host sent `initialize` and is awaiting its response.                |
| `ready`            | `initialize` succeeded; feature requests are accepted.                       |
| `stopping`         | New work is rejected while shutdown and process-tree reaping complete.       |
| `error`            | The desired operation failed or bounded automatic recovery was exhausted.    |
| `unsupported`      | The task executor is ineligible; no task resource was started or resumed.    |

`activity` is orthogonal: `server_work` exists only while one or more valid server-reported
work-done tokens are active; otherwise it is `idle`. A server can therefore be `initializing` with
reported work or `ready` with reported work without Kandev conflating that evidence with protocol
readiness. `restart_required` is also an overlay rather than a process phase: a ready generation can
continue serving its previous workspace scope while clearly requiring an explained restart.

The controller serializes commands per `(task_id, language)`. Concurrent duplicate commands while
one transition is active coalesce. Non-terminal different commands linearize in accepted order.
Explicit Stop, Disabled policy, and terminal task cleanup interrupt an accepted Start/install so
shutdown never waits indefinitely behind startup; the terminal transition is the final desired
state and process-tree proof. A restart first proves the old generation stopped, then admits one new
monotonic generation. Lifecycle-owned work queued behind a detached request is removed and releases
its reservation when controller shutdown cancels it; already-running lifecycle work remains joined.
Retrying the same task-host command after a transport timeout is idempotent.

Controller startup must read the full durable task/language inventory before task-LSP operations
can pass their readiness gate. If that authoritative read fails, the gate returns the same sticky
startup error for every operation until backend restart; Kandev must not admit work against an
empty capacity ledger or start a background retry behind apparently usable controls. After a
successful inventory read, every possibly-live generation is reserved before fallible runtime
inspection begins.

Startup registers watches from a post-reconciliation durable inventory. A watch failure may record
`task_host_watch_lost` only if the current row still has a server-bearing phase and nonzero
generation; a stale pre-reconcile row must not replace `off`, `queued`, `waiting_for_task`, or
`unsupported` state.

An unexpected process or task-host stream failure keeps the desired policy and attempts automatic
recovery after 1, 5, and 30 seconds. A generation that remains ready for five minutes resets that
budget. After three failed recoveries, the phase stays `error` with explicit Restart available;
there is no unbounded crash loop. An initialize request that remains alive is not a crash and is
never auto-restarted or timed out.

Recovery and ready-budget-reset timer callbacks acquire controller-lifecycle ownership before any
store or runtime access and validate their current timer epoch. Controller shutdown cancels
callback admission, stops timers that have not fired, and joins callbacks already in flight;
recovery command execution retains that owned lifecycle context.

Concurrent and retried controller shutdown calls wait for the same lifecycle-generation join. If
one caller times out, a later caller must continue waiting rather than report success early. Timer
validity includes an immutable registration identity as well as its local epoch, preventing a
committed callback from a deleted recovery/discovery entry from consuming a replacement entry that
reused the same epoch value.

An explicit Start or Restart begins a new user-requested recovery epoch and therefore resets any
exhausted automatic retry budget for that task/language. A delayed task-host watch snapshot cannot
reacquire capacity or replace Off after Stop has cancelled that watch.

## Permissions

- Any authenticated human who can access the task can read and operate its LSP controls under the
  existing task/workspace ownership rules. Authorization runs before policy, environment,
  execution, installer, capacity, or attachment access, so hidden task IDs cannot be enumerated.
- Browser handlers derive `task_id` from the authorized route. They stamp initiator `user` and do
  not accept an origin override.
- Automatic discovery, recovery, capacity wakeups, source changes, and lifecycle cleanup use an
  internal trusted origin after resolving a real task record.
- A future in-session MCP caller derives task ownership from its trusted `AgentExecution` scope and
  reaches the same controller. Agent-provided task/session IDs and initiator values are ignored or
  rejected. No identity-free fallback is allowed when execution ownership cannot be resolved.
- The task-host control/watch/attachment APIs remain protected by the execution's agentctl secret;
  they are not public browser endpoints. The backend supplies task identity in an authenticated
  internal transport header after authorization; JSON bodies cannot supply or override it.

## Failure modes

- **Unsupported/unknown executor:** policy remains visible, phase is `unsupported`, and no capacity,
  task execution, agentctl instance, installer, or server is launched.
- **Task environment unavailable:** desired state becomes `waiting_for_task`; read-only status and
  discovery do not materialize a task environment. Resume/ready triggers one reconciliation.
- **Capacity exhausted:** desired state becomes `queued` before execution creation. Slot release
  wakes queued task/language keys deterministically by accepted action time, then task ID/language.
- **Missing binary:** an auto-installable language installs only when global permission allows it.
  Kotlin and platform-ineligible Rust installs show manual setup guidance. Installer failure keeps
  bounded task-host detail and does not fall back to a project binary.
- **Start failure or server crash:** the process tree is reaped, progress/capability/diagnostic
  state for that generation is cleared, and bounded recovery follows the state-machine policy.
- **Initialize rejection:** phase becomes `error` and preserves the server's JSON-RPC error message
  as text. A slow but live initialize remains `initializing` with increasing elapsed time.
- **Browser/attachment disconnect:** outstanding attachment requests are cancelled or forgotten,
  its document references are released, and the task-host server and progress remain alive. Live
  task/language/session attachment intent survives browser transport replacement, and all sessions
  rebind to the single replacement connection.
- **Backend watch disconnect/restart:** the task host retains the generation. Recovery adopts its
  snapshot and capacity before controls or background settings/discovery can consider a new start.
  A cache miss first probes for the existing physical task host without creating or resuming one.
  This existing-only probe uses stable task/environment identity and physical reconnect data without
  loading launch-time executor profiles, repository credentials, managed-cache state, or remote
  resume preflight. A genuinely new launch still resolves and validates every launch input. Stale
  backend progress is cleared only if no matching live generation exists.
- **Agentctl/task-host loss:** its process manager reaps descendants. The backend records the loss
  and uses bounded recovery only after the old runtime is known dead.
- **Environment replacement ambiguity:** Kandev blocks the replacement launch until the old
  generation is confirmed stopped or its execution is dead; it never optimistically runs both.
- **Shared physical environment:** stopping a borrowing task stops only that task's language slots.
  Stopping, archiving, or deleting the environment owner transfers the environment to one live
  borrower before mutation and preserves the physical task host plus every other task's independent
  slot. After stop proof, cleanup purges only the departing task's in-host slots, snapshots,
  subscriptions, caches, and workspace projection. When no live borrower remains, or the environment
  itself is torn down, cleanup reaps the physical task host and every descendant.
- **Workspace roots change:** supported dynamic folder updates keep the generation; otherwise the
  task reports `restart_required` without silently discarding an expensive import. Refresh holds
  task/environment admission through runtime and durable-state updates, and config commit plus live
  application is serialized per task, so an older refresh cannot restore removed roots after a newer
  refresh. Terminal cleanup either follows the refresh and purges it or blocks the refresh before
  runtime access.
- **Task stop/archive/delete:** cleanup cancels starts/recovery before runtime teardown. Failure of
  graceful LSP control falls back to the task environment's full process-tree cleanup. Delete
  removes the persistent row only after cleanup ownership is registered. Direct and cascade
  terminal mutations publish exclusive admission intent and cancel active runtime readers before
  waiting, then persist a prepared cleanup barrier before inventory reads. Abandoned barriers are
  cancelled after a bounded interval even when transition-marker persistence failed.
- **Task-host startup:** the runtime registration remains internal until agentctl readiness and
  task-environment credential persistence both commit. A failed rollback keeps the cleanup handle
  private and recovery retries physical teardown before permitting replacement.
- **Malformed/stale protocol data:** oversized 16 MiB JSON-RPC frames, malformed progress, unknown
  tokens, lower revisions, old generations, and responses for detached request IDs are rejected
  without mutating current state.
- **Concurrent edits:** duplicate `didOpen` cannot rewind the canonical overlay. Later accepted
  changes are ordered for analysis, but Kandev does not claim collaborative editing or merge one
  browser's unsaved buffer into another.

## Persistence guarantees

Task policy, language detection summary, generation, revision, lifecycle timestamps, stable reason
codes, initiator category, and current error projection survive browser and backend restarts in
`task_lsp_languages`. Global defaults, configuration, and status visibility remain in user
settings. Hiding a language is presentational and never owns or overrides task policy. Browser
local storage does not own or override task policy; legacy session/language enablement keys are
ignored and removed when encountered.

A living task-host agentctl retains the process, initialized JSON-RPC peer, active progress,
capabilities, diagnostic cache, and open-document broker across browser and ordinary backend
reconnects. Those are runtime facts, not durable promises: agentctl or environment teardown clears
them. Backend recovery then increments generation and reports one replacement start if policy still
requires a server.

The browser retains task/language/session attachment leases independently of its current transport
generation. Task event registration is acknowledged before the LSP view performs its post-subscribe
authoritative refresh, so an event between initial HTTP hydration and subscription establishment
cannot leave stable stale state indefinitely.

A temporary task stop or archive stops runtime state but preserves task policy and historical
evidence. Unarchive/resume waits for a supported canonical environment, reruns bounded discovery,
and reconciles effective policy. Task delete cascades the task-language rows. No process survives
task-environment cleanup merely because a browser remains connected.

## Scenarios

### Ownership and controls

- **GIVEN** a Local PC task containing Kotlin sources and no open file, **WHEN** the user chooses
  Start for Kotlin from the task control, **THEN** policy becomes Keep warm and one Kotlin process
  begins initialization before any Kotlin editor mounts.
- **GIVEN** a warm Kotlin generation, **WHEN** the user switches to an unsupported file, chat,
  review, terminal, or another panel and advances beyond two minutes, **THEN** the same generation
  remains live and no new initialize/import occurs.
- **GIVEN** two sessions of one task, **WHEN** each opens a Kotlin file, **THEN** both use the same
  task/language generation and the task host has one Kotlin process.
- **GIVEN** two browser surfaces authorized for one task, **WHEN** both attach to Kotlin, **THEN**
  request/document traffic is isolated by attachment while the process and generation count stay
  one.
- **GIVEN** a running task server, **WHEN** the browser reloads or reconnects, **THEN** the task
  status and editor providers reattach without another process start or project import.
- **GIVEN** a running generation, **WHEN** the user confirms Restart once, **THEN** the old process
  is reaped before exactly one next generation initializes and the UI records user/restart reason.
- **GIVEN** Kotlin is running or queued, **WHEN** the user chooses Stop, **THEN** policy becomes
  Disabled, recovery is cancelled, the process is reaped, and later file/editor mounts do not
  reacquire it.
- **GIVEN** policy is Inherit and global Kotlin auto-start is off, **WHEN** Kotlin is detected,
  **THEN** Kotlin remains off; enabling the global default starts it once without opening a file.
- **GIVEN** concurrent Start, Stop, and Restart calls for one task/language, **WHEN** the controller
  accepts them, **THEN** snapshots expose monotonic revisions, duplicate commands coalesce, the
  final policy matches acceptance order, and at most one process exists.

### Status and progress

- **GIVEN** Kotlin reports Gradle import progress, **WHEN** another file or panel is active, **THEN**
  the application status item remains visible with server text/elapsed time and its disclosure
  retains Stop and Restart controls.
- **GIVEN** two task languages are live, **WHEN** the status bar renders, **THEN** it shows one
  aggregate item such as `LSP · 2 running`, not one item per active editor.
- **GIVEN** the user opens the task disclosure, **WHEN** its language rows render, **THEN** each row
  starts as a compact summary and independently expands to reveal the task policy and lifecycle
  actions through shared design-system controls.
- **GIVEN** Go is hidden in editor settings, **WHEN** task status renders or reloads, **THEN** Go is
  omitted from aggregate counts and rows while its discovery, policy, generation, and process state
  are unchanged.
- **GIVEN** Go is hidden from task status, **WHEN** a Go Monaco editor opens its toolbar shortcut,
  **THEN** that disclosure includes and focuses Go so the editor shortcut remains functional.
- **GIVEN** every language is hidden, **WHEN** a task view renders, **THEN** the task/workspace status
  action stays discoverable and explains how to manage language visibility in settings.
- **GIVEN** the Application status bar is disabled, **WHEN** any task view is open, **THEN** a task
  or workspace action can open every visible supported language and its controls without a
  supported file.
- **GIVEN** the server process started but has not received an initialize response, **WHEN** status
  renders, **THEN** it distinguishes process start from initialization and shows elapsed time with
  no ETA.
- **GIVEN** Kotlin initialize remains live for 60 seconds, **WHEN** status renders, **THEN** it names
  Gradle import only as a possible cause, retains Stop, and does not time out or restart.
- **GIVEN** a server reports title/message/percentage through work-done progress, **WHEN** another
  attachment or browser opens status, **THEN** it sees the same generation-scoped work item with a
  clamped percentage and elapsed time.
- **GIVEN** a server reports no work-done events, **WHEN** initialize succeeds, **THEN** status says
  ready/idle rather than inventing indexing work or completion.
- **GIVEN** a prior Stop or Restart, **WHEN** disclosure opens later, **THEN** it shows generation,
  started-at/elapsed evidence, stable last reason, and initiator category.

### Discovery, environments, and recovery

- **GIVEN** a large repository, symlink loop, or ignored dependency tree, **WHEN** language
  discovery runs, **THEN** it remains inside the entry/depth/time budgets, never follows the loop,
  starts no LSP, and reports partial results when truncated.
- **GIVEN** a task has multiple repositories, **WHEN** one language starts, **THEN** one initialize
  request uses the task root and the ordered repository roots as workspace folders.
- **GIVEN** repository roots change and the server cannot update workspace folders dynamically,
  **WHEN** discovery refreshes, **THEN** the running generation stays alive and status requires an
  explained Restart rather than silently reimporting.
- **GIVEN** a task environment is stopped while policy is Keep warm, **WHEN** task cleanup runs,
  **THEN** the process tree is reaped and policy persists; one new generation starts only after the
  task environment later becomes ready.
- **GIVEN** a task is archived, **WHEN** archive cleanup commits, **THEN** every task LSP is stopped
  and progress cleared; unarchive does not start anything until a supported environment is ready.
- **GIVEN** a task is deleted, **WHEN** deletion completes, **THEN** all task LSP process trees are
  reaped and `task_lsp_languages` rows cascade away.
- **GIVEN** a backend restart while agentctl and Kotlin remain alive, **WHEN** recovery reconciles,
  **THEN** it adopts the same generation/progress and does not repeat initialize/import.
- **GIVEN** backend memory loses the task-host handle while its process remains alive, **WHEN** a
  read or Ensure occurs, **THEN** Kandev reattaches to that stable physical host before permitting
  any replacement process.
- **GIVEN** a task host may still exist after its launch-time executor profile was deleted, **WHEN**
  cleanup probes the physical runtime, **THEN** Kandev performs an existing-only inspection without
  reloading that profile, stops the process tree when present, and still requires the profile before
  any later new launch.
- **GIVEN** Start is blocked in install or task-host launch, **WHEN** Stop, Disabled policy, or task
  teardown is accepted, **THEN** the blocked work is cancelled and the terminal transition proceeds
  without waiting for startup to finish.
- **GIVEN** agentctl died during a backend restart, **WHEN** policy still wants Kotlin, **THEN**
  recovery proves the old runtime dead and starts exactly one next generation.
- **GIVEN** a language server crashes repeatedly, **WHEN** Keep warm remains effective, **THEN**
  Kandev retries at 1, 5, and 30 seconds and then leaves an actionable error without an unbounded
  process loop.
- **GIVEN** the task/language server cap is full, **WHEN** another task requests Start, **THEN** it
  queues before task-host resume, counts no browser lease, and starts once when a real server slot
  is released.
- **GIVEN** task A releases a capacity slot while queued task B has a slow installer, **WHEN** A's
  Stop completes, **THEN** A returns after its own durable stop proof without waiting for B; B's
  promotion remains controller-owned and cancellation cannot leak its reserved slot.
- **GIVEN** startup cannot inspect a persisted generation that may still be alive, **WHEN** another
  task requests Start, **THEN** the ambiguous generation keeps its capacity reservation until
  authoritative absence or a confirmed stop prevents over-admission.
- **GIVEN** an SSH, Sprites, Remote Docker, unknown, or missing environment, **WHEN** Start is
  requested, **THEN** status is unsupported and no capacity, execution, installer, or process is
  acquired.
- **GIVEN** a Local Docker task, **WHEN** its task-level control starts Kotlin, **THEN** discovery,
  binary lookup, initialization, and process execution occur inside that container and remain
  shared by all task sessions.

### Editor and protocol behavior

- **GIVEN** two attachments open the same canonical file, **WHEN** the second sends `didOpen`,
  **THEN** the task host adds a reference without sending a duplicate upstream open or rewinding
  the first attachment's newer text.
- **GIVEN** two attachments edit one open file, **WHEN** changes arrive, **THEN** upstream versions
  increase in arrival order and the latest accepted text is analyzed without overwriting either
  browser's local buffer.
- **GIVEN** two sessions share one task/language connection and its browser transport closes,
  **WHEN** either session establishes the replacement transport, **THEN** both live session leases
  rebind and continue routing through one upstream generation.
- **GIVEN** two sessions reference one document, **WHEN** one session releases its final attachment
  lease before its editor cleanup runs, **THEN** that session's document references are drained
  before routing membership and the remaining session can close the final upstream document.
- **GIVEN** initial task-LSP HTTP hydration completes before the task WebSocket subscription ACK,
  **WHEN** lifecycle state changes in that interval, **THEN** a post-ACK authoritative refresh
  converges the view even if the event itself was not delivered.
- **GIVEN** the last editor closes a document while policy is Keep warm, **WHEN** `didClose` is sent,
  **THEN** document diagnostics may clear but the language-server generation remains running.
- **GIVEN** a successful file save races with later typing, **WHEN** persistence returns, **THEN**
  the server stays synchronized to the newer buffer and stale optional `didSave.text` is omitted.
- **GIVEN** TypeScript LSP is active for one task model, **WHEN** Monaco requests intelligence for
  another task/session model, **THEN** built-ins are suppressed only for the model and capabilities
  covered by the active external server.
- **GIVEN** an LSP definition targets another file under any task repository root, **WHEN** the user
  invokes navigation, **THEN** Kandev opens or focuses that file in the initiating task/session
  model and does not expose the task-host path as a browser ownership key.
- **GIVEN** a server sends an oversized frame, stale generation response, malformed progress, or
  response for a detached request, **WHEN** the task host processes it, **THEN** it rejects that
  data without corrupting the live task/language state.

### Mobile and security

- **GIVEN** a phone task view, **WHEN** the user opens Status, **THEN** task language policy, state,
  progress, errors, and 44 px controls appear in independently collapsible inline rows in the one
  safe-area-aware drawer.
- **GIVEN** a phone opens a Kotlin file while Kotlin policy is Disabled, **WHEN** the viewer mounts,
  **THEN** no LSP attachment or process starts; the separate task Status control can still Start it.
- **GIVEN** a coarse-pointer tablet, **WHEN** the task control opens, **THEN** the same shared rows
  use one contained drawer scroll owner with no document-level horizontal overflow.
- **GIVEN** a repository contains `.kandev/lsp-servers/kotlin-lsp` or a relative executable path,
  **WHEN** Kotlin starts, **THEN** Kandev ignores it and uses only the trusted registry/cache/PATH
  boundaries.
- **GIVEN** an unauthorized browser requests another task's LSP snapshot, action, or attachment,
  **WHEN** the route handles it, **THEN** authorization fails before runtime lookup and leaks no
  task, environment, generation, or binary evidence.
- **GIVEN** an authorized attachment sends a document URI that traverses, names a sibling path, or
  resolves through a symlink outside the task's canonical roots, **WHEN** the task host handles it,
  **THEN** it rejects the message before the language server receives any request or notification.
- **GIVEN** an in-root Windows reparse point targets an untrusted UNC share, **WHEN** an attachment
  names a document beneath that redirect, **THEN** the task host rejects the target before any DNS,
  SMB, or target-filesystem lookup.
- **GIVEN** a backend-selected Windows workspace root is itself a junction or mount point, **WHEN**
  the task host configures the generation, **THEN** it records the canonical identity from the same
  root handle it pins before browser attachment, and later document traffic cannot reopen or rebind
  that root.
- **GIVEN** a checked document ancestor is replaced by a symlink or reparse point while authorization
  is walking the path, **WHEN** the next component is inspected, **THEN** handle-relative traversal
  fails closed without touching an outside root or untrusted network authority.
- **GIVEN** overlapping authoritative browser refreshes for the same task, **WHEN** an older refresh
  fails after a newer refresh has begun and the newer refresh succeeds, **THEN** the accepted snapshot
  is not paired with the older transport error, while newer failed controls remain protected from
  older successful refreshes.

## Out of scope

- Implementing a task-scoped LSP MCP tool or exposing LSP intelligence directly to agents.
- Language intelligence inside the phone CodeMirror viewer; mobile receives lifecycle controls and
  status only.
- SSH, Sprites, Remote Docker, or other remote-executor LSP support.
- Collaborative editing, cross-browser buffer merging, or remote cursor presence.
- Invented indexing percentages, project-completion guarantees, or ETAs when the server does not
  report them.
- More than one language-server process for the same task/language, including per-repository or
  per-session processes.

## Implementation plan

[Task-scoped LSP lifecycle implementation plan](../../plans/task-scoped-lsp-lifecycle/plan.md)
