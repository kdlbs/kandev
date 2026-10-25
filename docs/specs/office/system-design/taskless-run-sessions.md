---
status: current
system: office
requirements:
  - REQ-OFFICE-TASKLESS-001
---

# Taskless Office Run Sessions

## Purpose and boundaries

Office owns run scheduling and durable run-session records. The shared agent
runtime owns processes and executor resources. Task services retain strict task
ownership. Follow [ADR](../../../decisions/2026-09-17-office-taskless-run-sessions.md).
This design completes the taskless launch and observation behavior in
scheduler-01/02; it does not replace their queue, continuation or idle-skip
rules. Operator stop controls and restart reconciliation remain requirements
under `REQ-OFFICE-TASKLESS-001` and are not implemented by this coverage
change. The outstanding scope is recorded under
[Outstanding: stop controls and restart recovery](../requirements/taskless-run-sessions.md#outstanding-stop-controls-and-restart-recovery).
This document specifies how a taskless run acquires a session, runs and is
observed, and records the current lifecycle gaps for that follow-up.

The mechanism described here has landed. The imperative voice states a durable
contract, not a build order: each "add", "reserve" or "must" is a rule the code
has to keep satisfying, not an outstanding task. Where a statement is about the
current code rather than the contract, it says so.

## Requirement mapping

| Criteria | Sections |
| --- | --- |
| .1, .2 | Persistence; launch flow |
| .3 | Scheduling and routing |
| .4 | Events and observation |
| .5, .6 | Outstanding lifecycle and recovery work |
| .7 | Runtime admission and security |
| .8 | Persistence; launch flow; outstanding lifecycle scope (replay clause) |

All criteria belong to REQ-OFFICE-TASKLESS-001.

## Integration seams

These seams are in place today; the rest of this document is the contract they
must keep satisfying.

`office/service/scheduler_integration.go:launchAgent` branches on task presence
and routes an empty task ID to the routing dispatcher and then to the Office
`RunSessionLauncher`; `office/scheduler/dispatch_routing.go:launchCandidate`
takes the same launcher for its routed candidates. `failTasklessRun` remains
only as the fallback for an unconfigured launcher or a launch that errored, so
an unsupported taskless launch is a configuration failure rather than a
capability gap. Existing taskless-failure tests assert that narrower meaning and
still cover the unrelated failure, race and inbox guarantees.

`agent/runtime` exposes both preparation and start: `Launch` prepares an
execution and keeps its original semantics for task callers, while `Start`
additionally registers the execution, starts the process and dispatches the
first prompt for run owners. Lifecycle `ensureLaunchSessionStillActive` still
reads task sessions and is therefore a task-owner path; run owners are admitted
through owner admission instead and never acquire a task-session identity.

`office/service/event_subscribers.go` carries `handleTasklessAgentCompleted`,
continuation summaries and run-terminal compare-and-set behavior. New taskless
lifecycle events carry run, run-session, attempt and execution identity, and the
legacy agent-only lookup survives only to resolve legacy events.
`office/pause/sweep.go` inventories live Office run sessions directly through
`ListLiveRunSessionsForWorkspace` and stops them by execution ID, independently
of the task-ID set it uses for task-owned work.

## Persistence

Add `office_run_sessions` to the Office repository migrations, using portable
SQLite/Postgres SQL. Columns: `id`, `workspace_id`, `agent_profile_id`
(the Office identity), `run_id`, `attempt`, `state`, `execution_id`,
`execution_profile_id`, `adapter`, `model`, `acp_session_id`, `created_at`,
`started_at`, `finished_at`, `cancel_requested_at`, `error_message` and a version
for compare-and-set updates. States are preparing, running, finished, failed,
cancelled and interrupted. Unique `(run_id, attempt)` prevents duplicate launch
reservation. Every provider attempt has its own session ID and never shares an
ACP session with another attempt. The route ledger `office_run_route_attempts`
carries no session column, and no reader may assume its rows map one-to-one onto
attempt rows: a candidate rejected before reservation leaves a route attempt
with no session behind it. Correlating one route attempt to one session is not
supported, and adding a column for it is out of scope here.

`attempt` starts at 1 and is allocated as one greater than the highest attempt
already recorded for that run, so numbering is monotonic per run and a number is
never reused. Reservation is an insert that ignores a conflicting
`(run_id, attempt)` row: the caller that inserts wins, and the caller that
conflicts loses the reservation, leaves the predecessor row untouched, and fails
its own launch rather than retrying inline or adopting the predecessor's
session. The run then follows the existing failure and retry policy, which
allocates the next attempt number on its next pass. Allocation is not refused
for a run that already carries a `finished` attempt. The settlement guard that
must prevent a duplicate launch remains part of the outstanding `.6` recovery
work; this coverage change does not add that guard.

Reserve the session and bind `runs.session_id` atomically while the run is
claimed. That column names the run's most recently launched attempt, not its
first: reservation binds it only while it is empty, and each successful launch
then re-points it at the attempt that launched. It is a pointer to the live
attempt, not an admission control, and event resolution is not justified by it —
the comparison it would drive never runs for a run-owned event, because such an
execution carries no task-session identity and the event supplies no session ID
to compare. What decides whether an event may act is the attempt row itself,
stated under events and observation. Freezing the column on the first attempt
would still be wrong, because run detail would then link a dead session for the
rest of the run's life, but that is a surface consequence rather than a guard.
`office_run_sessions` stays authoritative for history — the column names one
attempt, never the set, so run history, costs and recovery read the session
table. Runtime registration persists `execution_id` before process startup. A
failed bind must stop/rollback the prepared execution. Workspace deletion
performs stop-before-delete and retains cleanup evidence when stopping fails.
Workspace pause does the same. The remaining operator controls — explicit run
cancellation, agent disable and agent removal — do not reach run sessions, which
is an open gap recorded with
[the outstanding follow-up](../requirements/taskless-run-sessions.md#outstanding-stop-controls-and-restart-recovery). Run-history retention deletes runs and run
events but not run-session rows, which are removed when their workspace is;
pruning them with the run is named out of scope in the requirement rather than
claimed as existing behavior. Do not store JWTs or environment secrets.
No changes to task-session nullability or synthetic task creation are permitted.

Shared runtime inventory uses the run-session ID as
`executors_running.session_id` and executor correlation identity, with an empty
task ID and the typed owner snapshot in inventory metadata. This key has no
task-session foreign key. `AgentExecution.SessionID` remains empty for run
owners, so task consumers cannot mistake it for a task session. Run workspaces
live under
`office-runs/<workspace>/<run-session>` and do not carry task ownership markers.
Normal stop persists terminal inventory. Recovery validates the owner snapshot
and stops the recorded predecessor before allowing a retry; failed stops and
unknown inventory retain the claim.

## Runtime admission and security

The runtime carries a typed `ExecutionOwner` on `LaunchSpec`, execution and
events, with `kind=task|run`, workspace/session identity, and run/attempt
identity for run owners. Admission goes through an owner admission provider, and
`Runtime.Start` prepares, registers, starts the process and dispatches the
initial prompt. Existing `Launch` callers retain their preparation semantics.
Keep all lifecycle imports inside `internal/agent/runtime`; backendapp only
composes interfaces. Runtime supplies process mechanics; Office supplies
admission and record callbacks through interfaces with no reverse Office import.

Task owner admission retains current task cleanup and terminal-session checks.
Run owner admission checks exact durable session, run claim/attempt, agent
eligibility, workspace existence and pause/cancel state before allocation and
again around runtime registration/start. Unknown owner, mismatched identity or
read error fails closed. A durable cancellation mark participates in
registration so a pause racing allocation either observes/stops the new
execution or causes registration to roll it back. Do not implement this as an
unchecked metadata flag that skips `GetTaskSession`.

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

The Office `RunSessionLauncher` interface sits alongside TaskStarter and is
implemented through the shared runtime. Branch on task presence after normal
queue claim, budget, idle-skip, executor and context preparation. Both the concrete
path and `launchCandidate` use the same run-session launcher; candidate model,
provider, flags and environment must reach it without losing the Office prompt
or skills. Preserve route ledgers, provider classification, parking and backoff.
No new scheduler, background heartbeat producer or default toggle is needed.

Fallback across provider candidates is not free of session identity. Each
candidate enters the launcher separately and therefore reserves its own attempt
row, so a run that falls back through three candidates leaves three attempts in
`office_run_sessions`, not one row rewritten three times. A candidate that fails
must reach a terminal session state and release its runtime resources before the
next candidate is launched; no window may exist in which two candidates of the
same run hold live executions. That rule binds the failure paths as strictly as
the success path: a stop that fails must be observable and must not be followed
by a terminal session state, because a terminal row with a live execution behind
it is the forbidden window in its least visible form — every later sweep lists
only non-terminal rows and so never sees it.

**The launch-failure path does not satisfy that rule today, and nothing in
scope fixes it.** `StartRunSession` discards the error from `runtime.Stop` and
writes the attempt terminal regardless, so a stop that fails produces exactly
the terminal-row-over-live-execution shape named above. Whether the dispatcher
then falls back to the next candidate is decided by `handleLaunchFailure` in
`office/scheduler/dispatch_routing.go`, which classifies the launch error by
string and continues when fallback is allowed. Making the failure path fail
closed remains pending lifecycle work under `.5` and `.6`; no work order in the
current coverage change implements it. The rule above is the target
the code must eventually meet, stated here so the gap is a named exclusion
rather than a silence. `runs.session_id` points at the most recently launched candidate,
which is why run history reads the session table rather than that column.

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

The continuation scope AC .2 refers to is chosen once, when the run is created,
and persisted on the run; it is not recomputed per attempt, so every attempt of
a run reads and writes the same summary. A run whose context snapshot carries a
routine identity is routine-scoped (`routine:<routine id>`); any other run,
including a coordinator wake with no routine, is agent-scoped
(`agent:<agent profile id>`). The retired `heartbeat` scope is not reintroduced.
Only a successful attempt writes the summary, so a failed or interrupted attempt
leaves the last successful summary in place and the next attempt resumes from it.

A summary write that itself fails is a third case, distinct from an attempt that
never earned the write. By then the run is already terminal, so the failure
cannot un-complete it and must not try: the run stays complete, the last
successful summary stays byte-identical, and the next attempt resumes from it
exactly as it would after a failed attempt. What the failure owes is visibility
rather than a rollback, so it is recorded on the run's own event stream naming
the scope being written, which is what makes a run that completed carrying a
stale summary distinguishable from one that refreshed it. Neither a partial
summary nor a cleared one is permitted: the write is all-or-nothing against the
stored row. The same holds for the load that feeds it — a load that fails writes
nothing and is reported the same way.

## Events and observation

All new runtime events carry owner kind, workspace, Office agent identity,
execution profile identity, run, session and attempt. Before any state
transition or side effect, Office subscribers resolve the event to a still
claimed run and check the fields that pin identity: run ID, Office agent
profile, task ID when the event carries one, the run's bound session ID when the
event carries one, and — when the event names a run session — that the session
belongs to that run, carries the event's attempt number, belongs to the same
agent profile, is not in a superseded terminal state, and does not name a
different execution. Owner kind and workspace ride along for observability and
are deliberately not part of that check: the run-session ID already pins both,
and re-checking a field the event supplies adds no independent evidence.

Two of those terms are conditional, and neither condition is a loophole. The
bound-session comparison applies only to an event carrying a task-session
identity; a run-owned execution carries none, so on the taskless path that term
never runs and `runs.session_id` neither admits nor rejects an event. The
execution comparison applies only once the attempt row has persisted an
execution ID: between reservation and runtime registration the row has none to
compare. Every new run-owned event carries an execution ID, so the tolerated
case is an attempt row not yet registered, never an event that declines to
identify itself. For a taskless attempt past registration, the terms that decide
admission are the attempt number, the attempt's state and the execution ID.

Duplicate event/usage identities are idempotent. "Complete" in AC .4 means
completing the run: a delayed terminal event from an older attempt may apply a
terminal state to its own run session, but must not complete the run, write a
second usage ledger row, or clear agent-working state for a newer attempt. An
event may complete the run only if the record still admits it, and admission is
not a stored column because it does not need to be. Two facts the record already
holds decide it: the run must still be claimed, and the attempt row the event
names must still be in `preparing`, `running` or `finished`. `finished` belongs
in that set because the normal path finishes the session before it finishes the
run.

That filter admits at most one attempt of a run at a time, and what keeps it to
one is the rule in scheduling and routing above: a failed candidate must reach a
terminal session state before its successor launches. Where that rule is not
enforced — the launch-failure gap named there — a superseded attempt can still
pass the filter. Closing that is pending lifecycle work, and it does not soften
this criterion: AC .4 is unconditional, and nothing in this document permits an
event from an older attempt to complete a run that has a live successor. A
completion arriving from a superseded attempt is a symptom of that gap, not a
tolerated outcome.

Completing the run is idempotent regardless. The run's own terminal
compare-and-set is the duplicate-suppression guard: the first admitted
completion to reach it wins, every later one is a no-op with its side effects
suppressed, and the run completes exactly once. That guard is not a licence —
an event reaches it only after passing the filter above, so it suppresses
duplicates of an admitted completion rather than legitimising an inadmissible
one.

Usage is unaffected by an attempt being superseded: each attempt's ledger row is
keyed by its own execution ID, so every attempt persists and run totals include
all attempts of that run, as provider fallback already relies on.

Retain legacy resolution only for legacy events; never emit new agent-only
events. Runtime task consumers must not act on run-owned events before reading
or writing task sessions or workflow state. What enforces that today is
structural rather than explicit: a run-owned `AgentExecution` carries an empty
`SessionID`, so a task consumer resolves no task session and falls out.
Structural tolerance is not a contract, so this rule is pinned by test.

Workspace cost readers join taskless ledger rows to their durable run session;
run totals include every attempt belonging to that run. Serialized event-bus
usage frames have the same decoding and deduplication behavior as typed frames.

Use the existing Office cost ledger and run events for usage/output projections;
do not write run-only events into task-message/session tables with task foreign
keys. Durable usage deduplication is keyed by a single usage-event identity on
the cost ledger, unique across the install wherever that identity is present.
It is not a composite of session and turn: specifying it as one would admit the
same provider event twice under two sessions. For run-owned usage the identity
is synthesized from the execution ID and the prompt generation, and an execution
ID belongs to exactly one attempt, so the key is attempt-scoped without naming
the session. Run-owned usage has exactly one write path and it always supplies
that identity, so the ledger's tolerance for a row without one is unreachable
from here. Inspect `office/service/event_subscribers.go` and `office/costs` for
the write boundary. Keep bounded output/continuation semantics rather than
introducing a new interactive chat UI. Run-detail session links must target a
run-owned surface, never a task-session URL for a nonexistent task.

## Outstanding: stop controls and restart recovery

Operator stop controls and restart reconciliation remain required by this
requirement, but are not implemented by this coverage change. The behavior, the
open gaps and the analysis a follow-up should start from are recorded in the
requirement under
[Outstanding: stop controls and restart recovery](../requirements/taskless-run-sessions.md#outstanding-stop-controls-and-restart-recovery).
Two consequences are load-bearing here and are stated so they are not mistaken
for oversights: the launch-failure gap named in scheduling and routing above
stays open, and a crash between an attempt's terminal write and its run's
terminal write leaves a run claimed behind a terminal attempt.

**AC .8's replay clause stays in scope and is satisfied structurally.** A
historical taskless run already recorded as failed must remain history and must
not be automatically replayed. No mechanism in scope can replay one: the
unfinished-session query selects `preparing` and `running` only, so a recorded
failure is not in any population this design inspects, and nothing here adds a
channel that relaunches from run-session state. The clause is therefore a
non-regression statement over the code as it stands, verified by the existing
task-bound and taskless lifecycle tests rather than by new recovery machinery.
It would stop being free when the outstanding lifecycle work lands, which is why
the follow-up owes it a case.

## Verification and observability

Log sanitized run/session/attempt/execution identities and lifecycle
transitions; never credentials or full raw provider output. Test SQLite and the
repository's Postgres migration harness, concrete and routed launch, real
mock-agent prompt completion, and usage dedup. Cancellation, stop inventories
and restart reconciliation are not listed because they belong to the pending
follow-up. Routed launch is named separately because it has no coverage at all: a candidate
that falls back must be shown to reserve its own attempt, carry the Office
prompt and skills to the launcher, and leave its predecessor terminal. Keep
existing task-bound tests in the same targeted checks.

The same standard governs the plan's traceability table. An entry cites the test
that asserts the criterion, not a test in the same area or file: a criterion
whose only evidence is a neighboring test is uncovered, and recording that is
cheaper than discovering it in Build.

Delivered under [office-mode-repairs](../../../plans/office-mode-repairs/plan.md);
remaining verification is tracked in
[office-taskless-run-sessions](../../../plans/office-taskless-run-sessions/plan.md).
