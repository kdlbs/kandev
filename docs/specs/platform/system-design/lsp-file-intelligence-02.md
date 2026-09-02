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
# LSP File Intelligence System Design Part 2

## Purpose and boundaries

This file continues the current technical design for
`REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001`. [Part 1](lsp-file-intelligence-01.md) defines the
product boundary, data model, and API surface; this part defines lifecycle state, authorization,
failure handling, persistence, and acceptance scenarios. Both parts and
[ADR-2026-08-05-task-scoped-lsp-ownership](../../decisions/2026-08-05-task-scoped-lsp-ownership.md)
are authoritative.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-LSP-FILE-INTELLIGENCE-001` | [State machine](#state-machine) and following sections |

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
Automatic task and settings reconciliation reloads the durable language row inside that same lane
before deciding effective policy or touching a runtime, so a queued stale snapshot cannot stop a
server enabled by a newer accepted user action.
Fired recovery callbacks also enter the lane before revalidating their recovery epoch, reloading
state, inspecting or recovering the task host, and deciding whether to relaunch. A canceled callback
therefore becomes a no-op, while an explicit control accepted after an in-flight recovery remains
the final transition and cannot be stopped by recovery's stale policy snapshot.
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
inspection begins. If cleanup marks that captured generation absent or deletion removes its row
after the inventory snapshot, the serialized reload releases only the exact stale or proven-absent
generation before runtime admission; it cannot resurrect a phantom slot or release a newer live
generation. If the serialized ledger already contains a newer queued generation for the same key,
the stale inventory generation cannot replace or remove it.

Capacity uses durable generation-scoped process-presence evidence rather than inferring ownership
from the current display error. Proven absence survives later task-host, watch, and reconciliation
errors and backend restart; allocating a new generation clears that proof. If Restart allocates a
replacement generation but task-host access fails before the replacement command runs, the
task/language keeps its reservation because the previous generation may still be alive. No queued
language may be promoted until absence is proved.

Startup registers watches from a post-reconciliation durable inventory. A watch failure may record
`task_host_watch_lost` only if the current row still has a server-bearing phase and nonzero
generation; a stale pre-reconcile row must not replace `off`, `queued`, `waiting_for_task`, or
`unsupported` state. If that controller-local error cannot be persisted, the still-current failed
watch leaves the normal bounded recovery sequence armed rather than disappearing without future
work. A replacement watch's initial snapshot may replay the current task-host revision; that equal
high-water snapshot is accepted only to heal `task_host_watch_lost`, while older observations and
equal observations in every other state remain stale.

An unexpected process or task-host stream failure keeps the desired policy and attempts automatic
recovery after 1, 5, and 30 seconds. A generation that remains ready for five minutes resets that
budget. After three failed recoveries, the phase stays `error` with explicit Restart available;
there is no unbounded crash loop. An initialize request that remains alive is not a crash and is
never auto-restarted or timed out. A transient durable-state read failure at the five-minute reset
does not consume a recovery attempt or discard the reset; the read retries after 1, 5, then at most
every 30 seconds until the current epoch can be confirmed or newer lifecycle evidence invalidates
it.

Recovery and ready-budget-reset timer callbacks acquire controller-lifecycle ownership before any
store or runtime access and validate their current timer epoch. Controller shutdown cancels
callback admission, stops timers that have not fired, and joins callbacks already in flight;
recovery command execution retains that owned lifecycle context. A recovery request received while
an attempt is still marked running is coalesced and retained for the next bounded backoff; releasing
the language command lane before callback bookkeeping completes cannot strand a keep-warm language
in `error` or `off` without its remaining retry. New crash evidence invalidates a ready-budget reset
even when its timer callback has already fired; a stale Ready read cannot reset attempts after the
new recovery has consumed its backoff. Fired ready-reset work and watch-loss state plus recovery
scheduling use the same owned per-language command lane as runtime observations and controls, so a
durable crash and its backoff cannot be split around a concurrent budget reset.

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
- **GIVEN** task or settings reconciliation captured Disabled before the user enabled a language,
  **WHEN** the enable completes before reconciliation enters that language lane, **THEN**
  reconciliation reloads Keep warm and leaves the newly ready generation running.

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
- **GIVEN** startup captured generation 1 before it surrendered capacity and generation 2 queued,
  **WHEN** stale inventory adoption resumes, **THEN** generation 2 remains queued, the retired
  generation is not made active, and no server starts above the configured cap.
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

[Task-scoped LSP lifecycle implementation plan](../../../plans/task-scoped-lsp-lifecycle/plan.md)
