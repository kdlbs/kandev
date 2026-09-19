---
status: draft
system: office
requirements:
  - REQ-OFFICE-TASKLESS-001
---

# Taskless Office Run Sessions

## Purpose and boundaries

Office owns run scheduling and durable run-session records. The shared agent
runtime owns processes and executor resources. Task services retain strict task
ownership. Follow [ADR](../../../decisions/2026-09-17-office-taskless-run-sessions.md).
This design completes the taskless behavior in scheduler-01/02; it does not
replace their queue, continuation or idle-skip rules.

## Requirement mapping

| Criteria | Sections |
| --- | --- |
| .1, .2, .8 | Persistence; launch flow |
| .3 | Scheduling and routing |
| .4 | Events and observation |
| .5, .6 | Cancellation and recovery |
| .7 | Runtime admission and security |

All criteria belong to REQ-OFFICE-TASKLESS-001.

## Evidence and existing seams

`office/service/scheduler_integration.go:launchAgent` rejects empty task IDs;
`office/scheduler/dispatch_routing.go:launchCandidate` independently does so.
`agent/runtime.Runtime.Launch` currently prepares an execution; it does not
itself start the agent process and deliver the first prompt. Lifecycle
`ensureLaunchSessionStillActive` reads task sessions. These are all required
integration points, not optional follow-up work.

`office/service/event_subscribers.go` already has `handleTasklessAgentCompleted`,
continuation summaries and run-terminal compare-and-set behavior. Its legacy
agent-only run lookup cannot attribute new attempts safely.
`office/pause/sweep.go` currently inventories runs but stops processes through a
set of task IDs. Existing taskless-failure tests verify the capability gap;
replace their unsupported-launch expectation while preserving unrelated
failure, race and inbox guarantees.

## Persistence

Add `office_run_sessions` to the Office repository migrations, using portable
SQLite/Postgres SQL. Proposed columns: `id`, `workspace_id`, `agent_profile_id`
(the Office identity), `run_id`, `attempt`, `state`, `execution_id`,
`execution_profile_id`, `adapter`, `model`, `acp_session_id`, `created_at`,
`started_at`, `finished_at`, `cancel_requested_at`, `error_message` and a version
for compare-and-set updates. States are preparing, running, finished, failed,
cancelled and interrupted. Unique `(run_id, attempt)` prevents duplicate launch
reservation. All provider attempts also have distinct session IDs; record their
route-attempt association rather than sharing an ACP session.

Reserve the session and bind `runs.session_id` atomically while the run is
claimed. Runtime registration persists `execution_id` before process startup.
A failed bind must stop/rollback the prepared execution. Existing run history
retention deletes associated terminal session records only after confirming no
live resources; workspace/agent deletion performs stop-before-delete and retains
cleanup evidence when stopping fails. Do not store JWTs or environment secrets.
No changes to task-session nullability or synthetic task creation are permitted.

Shared runtime inventory uses the run-session ID as `executors_running.session_id`
and executor correlation identity, with an empty task ID and the typed owner
snapshot in inventory metadata. This inventory key has no task-session foreign
key. `AgentExecution.SessionID` remains empty for run owners, so task consumers
cannot mistake the execution for a task session. Run workspaces live under
`office-runs/<workspace>/<run-session>` and do not carry task ownership markers.
Normal stop persists terminal inventory. Recovery validates the owner snapshot
and stops the recorded predecessor before allowing a retry; failed stops and
unknown inventory retain the claim.

## Runtime admission and security

Proposed runtime APIs (not current symbols): typed `ExecutionOwner` on
`LaunchSpec`/execution/events, with `kind=task|run`, workspace/session identity,
and run/attempt identity for run owners; an owner admission provider; and
`Runtime.Start` to prepare, register, start the process and dispatch the initial
prompt. Existing `Launch` callers retain their preparation semantics. Keep all
lifecycle imports inside `internal/agent/runtime`; backendapp only composes
interfaces. Runtime supplies process mechanics; Office supplies admission and
persistent record callbacks through interfaces with no reverse Office import.

Task owner admission retains current task cleanup and terminal-session checks.
Run owner admission checks exact durable session, run claim/attempt, agent
eligibility, workspace existence and pause/cancel state before allocation and
again around runtime registration/start. Unknown owner, mismatched identity or
read error fails closed. A durable cancellation mark participates in registration
so a pause racing allocation either observes/stops the new execution or causes
registration to roll it back. Do not implement this as an unchecked metadata flag
that skips `GetTaskSession`.

Supply resolved executor profile, workspace, environment, prompt, skills,
permissions and Office MCP mode to the runtime. Allocate a session-specific
workspace under the existing managed runtime workspace mechanism; never reuse
an empty-task scratch key or assume a repository is required. Executor backend
contracts remain shared. A task-only executor configuration must produce an
explicit unsupported-configuration error, not a fake task ID or silent fallback.

Mint runtime credentials after reservation, binding exact workspace, Office
agent, run and session. Task ID remains empty. Existing workspace/capability
checks and task-decision rejection remain authoritative. Routing may change the
execution profile but never Office identity or tool authority.

## Scheduling and routing

Introduce a proposed Office `RunSessionLauncher` interface alongside TaskStarter,
implemented through the shared runtime. Branch on task presence after normal
queue claim, budget, idle-skip, executor and context preparation. Both the concrete
path and `launchCandidate` use the same run-session launcher; candidate model,
provider, flags and environment must reach it without losing the Office prompt
or skills. Preserve route ledgers, provider classification, parking and backoff.
No new scheduler, background heartbeat producer or default toggle is needed.

## Launch flow

1. Claim the existing run and apply admission/idle gates.
2. Reserve a fresh attempt/session; build runtime context and scoped credentials.
3. Resolve provider/executor, prepare the runtime with run ownership and register
   the exact execution ID durably. Recheck cancellation and pause admission.
4. Start the agent process, wait for readiness and dispatch the assembled prompt
   exactly once. Persist actual adapter/model and expose the session on run detail.
5. Consume events for the exact attempt. A successful turn finishes the session
   and run, updates the continuation summary, records output and stops resources.
   A failed attempt follows the existing routing/failure policy after cleanup.

## Events and observation

All new runtime events carry owner kind, workspace, Office agent identity,
execution profile identity, run, session and attempt. Office subscribers validate
that tuple against the active record before any state transition or side effect.
Duplicate event/usage identities are idempotent. Delayed terminal events may
finish only their own attempt and cannot clear agent-working state for a newer
attempt. Retain legacy resolution only for legacy events; never emit new
agent-only events. Runtime task consumers explicitly ignore run-owned events
before reading/writing task sessions or workflow state.

Workspace cost readers join taskless ledger rows to their durable run session;
run totals include every attempt belonging to that run. Serialized event-bus
usage frames have the same decoding and deduplication behavior as typed frames.

Use the existing Office cost ledger and run events for usage/output projections;
do not write run-only events into task-message/session tables with task foreign
keys. Add durable usage deduplication keyed by session plus provider event/turn
identity. Inspect `office/service/event_subscribers.go` and `office/costs` for the
actual write boundary. Keep bounded output/continuation semantics rather than
introducing a new interactive chat UI. Run-detail session links must target a
run-owned surface, never a task-session URL for a nonexistent task.

## Cancellation and recovery

Extend pause inventory with all live Office run sessions, independently of run
queue status: a run may already be cancelled while its process is still live.
Persist cancellation intent before stopping each execution through Runtime.Stop.
Union this inventory with existing task cancellation, count partial failures,
and retry only still-live resources. Apply the same seam to run cancel, agent
removal/disable and workspace deletion. End-of-turn cleanup is idempotent.

A service-owned, joined startup reconciliation pass and existing maintenance
cadence inspect unfinished sessions. Reattach observation only with proof that
the runtime still owns the exact execution/session. Otherwise stop any retained
runtime resource and mark the attempt interrupted before applying existing retry
policy. Unknown liveness or failed stop parks recovery visibly; never start a
replacement beside an uncertain predecessor. No cross-fire ACP resume. A crash
between terminal session persistence and run completion is reconciled from the
same exact-attempt record, with idempotent cost/summary side effects.

## Verification and observability

Log sanitized run/session/attempt/execution identities and lifecycle transitions;
never credentials or full raw provider output. Test SQLite and the repository's
Postgres migration harness, concrete and routed launch, real mock-agent prompt
completion, usage dedup, cancellation at allocation/registration/start, mixed
successful/failed stop inventories, restart with a live/absent/unknown predecessor,
and stale completion during a successor. Keep existing task-bound tests in the
same targeted checks. See [delivery plan](../../../plans/office-mode-repairs/plan.md).
