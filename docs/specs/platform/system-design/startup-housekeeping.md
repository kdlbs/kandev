---
status: current
system: platform
requirements:
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-001
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-002
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-003
  - REQ-PLATFORM-STARTUP-HOUSEKEEPING-004
---

# Startup housekeeping design

## Ownership and requirement mapping

Platform owns readiness and shared recovery orchestration. This design extends
the existing [startup lifecycle](startup-lifecycle.md) and
[startup progress](startup-progress-visibility.md) contracts. It does not replace
the [durable recovery](durable-agent-reattachment.md) identity or journal rules.

| Requirement | Mechanism |
| --- | --- |
| `REQ-PLATFORM-STARTUP-HOUSEKEEPING-001` | Preparation and activation boundary |
| `REQ-PLATFORM-STARTUP-HOUSEKEEPING-002` | Outstanding-state selection |
| `REQ-PLATFORM-STARTUP-HOUSEKEEPING-003` | Reporter history and existing surfaces |
| `REQ-PLATFORM-STARTUP-HOUSEKEEPING-004` | Session restoration scheduling |

## Verified starting point

At `72e9eec9b300918863f7d1da89f80303bf694335`, `backendapp/main.go`
constructs dependencies and starts some pollers before `orchestrator.Service.Start`.
That method synchronously calls `reconcileStartupState`, then activates the
scheduler, lifecycle sweep, and silent recovery. `lifecycle.Manager.Start` has
already attempted adoption before that reconciliation. Existing watcher and MCP
ordering fixes must survive this change.

`Repository.ListExecutorsRunning` reads the entire retained inventory.
`backfillPromptSeq` runs an UPDATE plus an unconditional historical counter
aggregation on each schema replay. `runSilentRestoreRecoveryPass` restores
candidates serially. `use-session-resumption.ts` awaits the open-time status
check when focus occurs; it does not prioritize that session at the backend.

Observed October 19 boot: 32.59 seconds to readiness, including 8.41 seconds in
prompt-sequence backfill. Survivor matching took 0.039 seconds. About 15 seconds
inside orchestrator preparation had no individual step timing. These observations
do not establish which task was focused or the cost of each uninstrumented query.

## Preparation and activation boundary

Keep the early bootstrap listener, `/health`, `/ready`, and Go-rendered startup
page. Keep application traffic behind the existing atomic handler switch.
Do not add another readiness endpoint or a second progress owner.

`backendapp` becomes the explicit coordinator of these stages:

1. Open the database, take an eligible upgrade backup, apply schemas, and admit
   required stores using the existing lifecycle.
2. Construct and wire passive services, event handlers, authorization, MCP,
   storage, and router dependencies. Register cleanup ownership during construction.
3. Run required **local** housekeeping under `recovering_sessions`, before any
   launch-capable producer or restoration worker activates. Name and time its
   independent operations rather than hiding them in `Service.Start`.
4. Adopt bounded local survivors after passive MCP/event wiring and local
   housekeeping, before publishing normal launch routes. This ownership discovery
   must precede new local launches: the startup orphan pass otherwise cannot
   distinguish a newly launched instance from an unowned survivor. Establish
   exact remote pending guards from SQL without contacting remote runtimes.
5. Validate required-store completeness and health, retain the existing version
   recording rule, publish the ready snapshot and application router atomically.
6. Activate normal runtime workers and session restoration through explicit
   process-owned start functions. Activation must not synchronously wait for a
   candidate conversation or a remote probe before admitting another session.

Split the preparation part of `Service.Start` into a
`PrepareStartup(context.Context) error` seam. Production composition invokes it
before activation. Preserve the existing `Start` entry point for other
compositions by making preparation idempotent for that service lifecycle; it
must not repeat work after successful preparation. A cancelled or failed
preparation must never publish success. Reset preparation state only when its
service lifecycle is stopped and restarted, with Stop joining admitted work.

Not everything in `reconcileStartupState` is local preparation. Split
`reconcileExecutorSessionsOnStartup` into local inventory classification and
per-session operations that need authenticated liveness/adoption. Do not move
the existing whole method before `Manager.Start`: `WasSessionRetracked` and
`HasPendingRemoteRecovery` are not valid dead-owner evidence before adoption.
Local bookkeeping may select pending work but must preserve a remote session's
turn and state until its owner is resolved by the existing guarded path.

Explicitly audit immediate-first-pass producers in `startServices`,
`startAgentInfrastructure`, `provideServices`, plugin startup, delivery workers,
and `startGatewayAndServe`. Defer their activation until the gate has passed.
The minimal listener and passive event subscriptions are allowed before the gate.
Move health checks or required initialization out of a worker's activation
method if they are actual readiness prerequisites. Preserve one start/stop pair,
reverse cleanup, restore quiescing, and subscriber-before-publisher ordering.
Worker starts registered while activation is in progress join its pending batch;
they cannot bypass an earlier failure or execute recursively ahead of that batch.

Office and automation schedules retain their policy after activation. They are
independent producers, not an excuse for restoration to dispatch instructions.
No global startup step may wait for all remote machines to become reachable.
Stable stopped plugin environments do not enter the live-agent attachment retry
queue. The owned plugin boot worker repairs only unfinished terminal allocation
or cleanup operations after active plugins have started; it is cancelled and
joined before persistence closes.

## Outstanding-state selection

### Prompt sequence replay

Keep replayable schemas and the exact existing ordinal definition, including
microsecond normalization and identifier tiebreaking. Avoid a new applied-marker
ledger that would conceal late legacy rows or failed partial repairs.

Use a dialect-supported partial index for user messages whose `prompt_seq = 0`.
Discover missing sequence counters from session/counter metadata and inspect
messages only for those sessions. Restrict counter aggregation to missing
counters instead of grouping the entire historical message table. Existing
counters remain authoritative even when higher-ordinal messages were deleted.
Run no ordinal UPDATE or message aggregation when neither outstanding rows nor
missing counters exist. A cheap indexed existence check is acceptable.

Legacy rows and missing counters remain repairable after interruption; fresh and
replay tests must use real repository initialization on SQLite and PostgreSQL.
Keep all SQL cancellable and do not change upgrade-backup eligibility.

### Runtime inventory

Keep `executors_running` authoritative under ADR 0025. Select active, uncertain,
pending-cleanup, or still-owned runtime records for reconciliation using their
runtime fields and retained evidence. Stable stopped resume-only records need
no runtime probe. Do not select solely from session state and do not delete
resume tokens to make the inventory smaller. Recheck exact execution identity
and row version before mutation, and preserve failed-inspection ownership.

## Reporter history and existing surfaces

Extend `internal/startup.Reporter`; do not add nested active steps. Retain the
current phase enum and add protocol-owned housekeeping steps in the existing
`recovering_sessions` phase. Proposed exact registry additions are:

| ID | Label key | Measure |
| --- | --- | --- |
| `startup.queue_reconciliation` | `startup.step.queue_reconciliation` | opaque |
| `startup.turn_reconciliation` | `startup.step.turn_reconciliation` | opaque |
| `startup.route_reconciliation` | `startup.step.route_reconciliation` | opaque |
| `startup.runtime_inventory` | `startup.step.runtime_inventory` | counted sessions |
| `startup.task_reconciliation` | `startup.step.task_reconciliation` | opaque |

For runtime inventory, the total is the selected local classification set;
advance only after each classification finishes. Do not count a session as
reattached or declare a remote owner dead. Keep `sessions.recovery` for its
existing meaning; post-ready restoration must not open startup steps.
Both dialects use these new steps. Opaque steps have no invented corpus or total.

Add an optional `completed_steps` list to `startup.Snapshot`. An entry contains
`id`, `label_key`, `activation_seq`, `phase`, `elapsed_ms`, and `completed`
(boolean). `activation_seq` is the active-step transition sequence captured at
BeginStep, so repeated identifiers have distinct list keys. Ordinary EndStep
records `completed: true`; ending due to phase change or replacement records
`completed: false` unless the caller already ended successfully. Such an entry
must not be rendered as a completed success. Timings alone never imply success.
A superseded parent timing can therefore be incomplete even when its enclosing
initialization succeeds; render it as “Timing incomplete”, not an operation failure.
Underlying failures remain fatal according to existing policy and logged at the
source; do not publish raw failure text through this unauthenticated endpoint.

Take one timestamp under the reporter mutex per transition. Freeze the startup
`elapsed_ms` at Ready and retain final history in memory for that process. Keep
at most 32 recent entries. The launcher uses a bounded 32 KiB reader for the
extended payload and tests the complete history through its HTTP probe. Clone slices in snapshots.
Reset history with the reporter's boot identity; no history goes into SQLite.
Existing consumers that ignore `completed_steps` retain their old behavior.
Malformed optional history must be ignored without losing a valid phase/step.

Open one queue step around the existing transfer, settlement, pending dispatch,
and Send Now preparation; record suboperation durations in structured logs so
the previous unexplained interval can be assigned. Open turn, route, inventory,
and task reconciliation steps around their real owning operations. No active
step may span asynchronous post-ready work. Extend registry completeness and
locale tests, reconciling AC-PLATFORM-STARTUP-PROGRESS-005.3 in the same change.

The Go bootstrap page and React `restart-progress-dialog.tsx` show completed
rows and durations under the active step. Labels resolve through existing
backend/web catalogs, with the same English and six translated locales plus
pseudo. The launcher prints step completion duration when observed and handles
the extended bounded payload. No new native desktop shell bridge is included.

Phone exemplar: the existing Go startup page and mobile restart-progress dialog.
Use their single-column information hierarchy, placing the active operation and
overall time before the timing list. Keep a single scrolling body inside a
safe-area-aware viewport, wrap long translated labels, and keep any existing
phone actions at least 44px. Desktop retains its compact dialog geometry.
Accessibility uses the current status region; ticking elapsed time must not
reannounce the whole history every second. See UI-01 in the implementation plan.

## Session restoration scheduling

After preparation, schedule existing eligible candidates through a bounded
manager-owned dispatcher with two concurrent operations. Reserve one admission
slot for the currently focused eligible session; background scanning uses the
other. If no focused session is pending, that slot can accept another background
candidate, but an already running operation is never cancelled to reclaim it.
Focus has priority at the next available admission. Keep per-session deduplication
and existing ceiling reservations; the bound does not override configured capacity.

Use the existing `session.launch` activation source `session_focus` to signal
priority. The frontend must signal the selected visible session without waiting
for its existing open-time check to finish; the backend joins any matching
attempt and remains authoritative on eligibility. Do not mount recovery effects
for hidden conversations. Other open-time checks still populate recovery UI.

Reuse native restore and authenticated reattachment, journal receipt projection,
and the current pinned checkpoint/CAS guards. Never resend a stored submission.
An uncertain previous outcome remains uncertain. Queue pause, recovery guards,
and readiness notifications must prevent recovery itself from draining queued
instructions, including the already-resumed and legacy-native branches.
Keep explicit user resume/submit and independently scheduled Office work distinct.

Stop, archive, task switch, and process shutdown must cancel or fence pending
work; a stale completion cannot affect a successor. Unsupported provider shapes
continue to produce their existing explicit blocked outcome. Pending remote
ownership goes to its existing retry mechanism rather than consuming a global
readiness gate. Do not weaken process-tree liveness proofs.

## Bounded inventory and completion snapshots

Background native restore inventory retains one bounded page at a time and drains
that page before reading the next. Focused requests still join the shared two-slot
dispatcher independently of inventory pagination.

Restart boot-id completion keeps startup-detail polling active until the ready
snapshot arrives, then freezes that final detail. A successful final response
without compatible snapshot data clears an earlier in-progress projection.

Persisted ownership inventory is required local evidence even when its rows name
remote runtimes. A read failure prevents readiness; remote endpoint availability
remains session-scoped after readiness. This preserves fail-closed owner fencing.
Malformed terminal plugin metadata is logged and skipped during deferred operation
repair so other unfinished operations can progress.

Runtime classification joins executor evidence with one batched active-session
inventory. Stopped rows with created, starting or running sessions still reach
normal guarded settlement; stopped, execution-less waiting, idle or terminal conversations
incur no process or endpoint probe. Resume identity remains retained.

## Validation and operational evidence

Use deterministic barriers to hold housekeeping and assert that producers and
restoration have not started. Exercise the production composition seam rather
than only callback arrays. Test cancellation, failed preparation, final health
failure, replay, and listener/MCP ordering.

Use disposable populated SQLite and isolated PostgreSQL fixtures to prove the
second startup does not aggregate historical messages. Query-plan evidence and
statement observation establish this without brittle wall-clock assertions.
Keep a before/after populated-startup timing record for human comparison.

Block one restoration and prove a second eligible session can become ready.
Assert unchanged prompt, user-turn, and queued-dispatch counts across startup,
focus, and reconnection. Browser tests cover the Go page and existing React
dialog on Chromium and mobile-chrome, including history, outage, boot reset,
long translations, and transition to the requested application route.

## Decisions and delivery

- [Visible preparation before runtime activation](../../../decisions/2026-10-19-startup-housekeeping-before-activation.md).
- [Replayable schemas](../../../decisions/0027-replayable-schema-migrations.md).
- [Authoritative runtime inventory](../../../decisions/0025-runtime-cleanup-uses-executors-running.md).
- [Combined implementation plan](../../../plans/startup-housekeeping/plan.md).
