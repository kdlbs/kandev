---
status: current
system: platform
requirements:
  - REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001
created: 2026-07-09
updated: 2026-09-02
owners:
  - tbd
---
# LSP File Intelligence System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
  browser attachments, editor leases, detected languages, or queued desired state. The canonical
  startup setting is `limits.lspMaxServers`, with a default of eight;
  `KANDEV_LSP_MAX_SERVERS` overrides YAML. The legacy `limits.lspMaxConnections` YAML key and
  `KANDEV_LSP_MAX_CONNECTIONS` environment variable are deprecated fallbacks only when their
  canonical replacements are unset. Desired servers wait visibly for a slot
  without starting/resuming a task host and are reconsidered when a slot is released. Slot release
  reserves the next entry atomically and promotes it asynchronously under controller lifecycle, so
  stopping one task never waits for another task's installer or launch. A canceled promotion returns
  its reservation exactly once and advances the queue. Startup reserves every persisted generation
  that may still own a process before fallible runtime inspection; inspection frees it only after
  authoritative absence, while a confirmed stop releases the matching durable/runtime identity. A
  transient recovery error therefore cannot make admission exceed the configured cap. A newer
  accepted queued generation for the same task/language is authoritative over an older startup
  inventory generation; stale adoption cannot erase that queue entry, resurrect the retired
  generation, or bypass the cap.
- Binary discovery, managed installation, and execution retain existing task-host trust
  boundaries. Kandev never executes a server from a repository or relative `PATH` entry. Managed
  npm/release binaries live under the task host's `~/.kandev/lsp-servers`; `gopls` uses the task
  host's Go toolchain. Kotlin remains manual-install-only. Rust auto-install remains limited to
  supported macOS and Linux task hosts.
- Task stop, archive, and delete cancel pending starts/recovery, clear active progress, terminate
  every LSP process descendant owned by that task, and delete both task-host and container-level
  encrypted runtime credential handles. Durable asynchronous cleanup snapshots retain those
  handles even after the task-environment row cascades. If another live task shares the
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


## Continued design

The lifecycle state machine, permissions, failure handling, persistence guarantees, acceptance
scenarios, exclusions, and implementation link continue in
[part 2](lsp-file-intelligence-02.md).
