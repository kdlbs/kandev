---
status: draft
system: office
requirements:
  - REQ-OFFICE-ASSIGN-RATE-001
  - REQ-OFFICE-ASSIGN-RATE-002
  - REQ-OFFICE-ASSIGN-RATE-003
---

# Office: Assignment Wake Rate Limit System Design

## Purpose and boundaries

This design places one additional admission gate in the Office run queue: an
allowance on agent-initiated `task_assigned` wakes, scoped per task and to one
producer — the task-mutation reactivity path, per
`AC-OFFICE-ASSIGN-RATE-001.12`. It adds no table, no column, no migration, and
no background job. The count is derived from rows the queue already writes.

It does not touch dedup key identity
(`docs/specs/office/requirements/run-dedup-generation.md`), the five-second
coalescing predicate, the 24-hour recent-duplicate lookback, the attended and
unattended provenance classifier, or the pre-launch budget gates.

## Requirement mapping

| Requirement | Mechanism |
|---|---|
| `REQ-OFFICE-ASSIGN-RATE-001` | A gate in `SchedulerService.queueRun`, evaluated after the idempotency and coalescing checks and before `CreateRun`, backed by a task-scoped window count over `runs`. Its position fixes the producer scope: see below. |
| `REQ-OFFICE-ASSIGN-RATE-002` | The gate is a pure read plus the insert it guards; it fails open, admits under a race, and derives its count from persisted rows only. |
| `REQ-OFFICE-ASSIGN-RATE-003` | A new `expvar.Map` beside the run-dedup counters, plus a Warn log at the decision site, plus a new `QueueOutcome` value. |

## Where the gate sits

`apps/backend/internal/office/scheduler/run.go`'s `queueRun` is where the
mutation producer's wakes converge — `reactToAssigneeChange` reaches it through
`QueueRunCtx` — and it already owns the other two admission decisions.

It is **not** a choke point every Office producer reaches, and nothing here may
assume it is. The repository has five `task_assigned` producers, listed by
inventory rather than inferred from the two this capability happens to discuss:

| Producer | Route to an insert | Actor on its payload |
|---|---|---|
| `scheduler/reactivity.go`'s `reactToAssigneeChange` — **in scope** | `QueueRunCtx` → `SchedulerService.queueRun` | `agent` or `user`, from the mutation |
| `service/event_subscribers.go`'s `queueTaskAssignedRun` | `Service.QueueRun` | none |
| `office/onboarding/service.go` (its own comment calls it "a third `task_assigned` producer") | `Service.QueueRun` | none |
| `service/scheduler_recovery.go`'s `recoverUnstartedTasks` sweep | `Service.QueueRun` | none |
| `orchestrator/event_handlers_workflow.go`'s `queueOfficeAutoStartRun`, the `auto_start_agent` step action (`officeAutoStartRunReason`) | `engineRunQueue.QueueRun` → `runsservice.QueueRun` | none |

`Service.QueueRun` (`service/run.go`) delegates to `runsservice.QueueRun` when a
runs service is wired and otherwise re-implements the idempotency, coalescing
and insert steps inline in `queueRunInline`; neither route, and neither does the
orchestrator's, passes through `SchedulerService.queueRun`. So a gate placed
here binds the mutation producer and only the mutation producer.

Two independent facts agree today — one producer reaches this function, and one
producer puts an actor on the payload — and `AC-OFFICE-ASSIGN-RATE-001.12`
deliberately makes the **second** the contract. Call position is an
implementation fact a refactor can move without anyone noticing; payload shape
is checkable from the payload alone, and that criterion's enumeration test is
what keeps the two from drifting apart silently. A future producer that reaches
`queueRun` without an actor is out of scope by the same rule that covers the
four below, and one that sets `actor_type: agent` violates a written invariant
instead of quietly joining the gate.

Excluding the other four is a scope decision, not an accepted hole: a *per-task*
allowance could not bind any of them. The subscriber's live path is
`handleTaskCreated`, which fires once per task creation, where a fresh task
identifier always brings a fresh allowance; its `handleTaskUpdated` is
documented in its own doc comment as a redelivery and defensive path, and today
reaches no wake at all, because neither `task.updated` publisher puts
`assignee_agent_profile_id` in the payload and `queueTaskAssignedRun` returns
early without one. Onboarding wakes a task it has just created. The recovery
sweep wakes a task that was never started, and is capped at `maxRecoveryPerTick`
rows per tick anyway. The orchestrator's is workflow-driven: it fires on a step
transition running `auto_start_agent`, and its one reference to the participant
slate is how it *resolves* which agent to wake, not a second producer. None of
the four is an agent minting fresh occurrences against one task in a loop, which
is the shape this capability bounds.

The gate belongs in `queueRun` rather than in `reactToAssigneeChange` or in
`DashboardService.SetTaskAssigneeAsAgent`:

- In `reactToAssigneeChange` it would bind the same single producer, but
  *upstream* of the recent-duplicate lookup and the coalescing attempt. A wake
  later suppressed or merged would already have consumed the allowance,
  contradicting `AC-OFFICE-ASSIGN-RATE-001.5` and `.4`. Position, not producer
  coverage, is what rules it out.
- In `SetTaskAssigneeAsAgent` it would have to refuse the *mutation*, which
  `REQ-OFFICE-ASSIGN-RATE-001.7` and the `## Out of scope` entry on
  rate-limiting the write both forbid.

The resulting order inside `queueRun`, satisfying
`AC-OFFICE-ASSIGN-RATE-001.5`:

1. `guardAgentStatus`
2. `checkPauseGate`
3. `CheckIdempotencyKey` — windowed dedup (unchanged)
4. `CoalesceRun` — five-second merge (unchanged)
5. **the assignment allowance** (new)
6. `CreateRun`

Placing it at 5 is what makes "admitted" mean exactly "a row was inserted".
A wake suppressed at 3 or merged at 4 never reaches the gate, so it neither
consumes the allowance nor can be refused by it — `AC-OFFICE-ASSIGN-RATE-001.5`
and the coalescing exclusion in `## Out of scope` are the same fact.

## The predicate

The gate applies only when all three hold, per
`AC-OFFICE-ASSIGN-RATE-001.1`, `.2` and `.9`, and in this order:

1. `reason == RunReasonTaskAssigned`
2. the wake's `actor_type` is `agent`
3. the wake names a task

The order is contract, not style. A wake failing 2 is out of scope and admitted
silently; a wake that passes 2 but fails 3 is the counted unattributed case of
`AC-OFFICE-ASSIGN-RATE-002.3`. Evaluating 3 first would count a wake that
carries neither field against the unattributed reason, which is the one the
event-subscriber producer's payload would land on if it ever reached here.

`actor_type` and `task_id` are already carried on `RunContext` and already
serialised into the payload (`run.go`'s `RunContext.ActorType`, `.TaskID`).
`queueRun` receives the encoded payload rather than the struct, so it reads both
back out of it **in Go**, with one `json.Unmarshal` of the incoming payload —
exactly what `CoalesceRun` does through `taskIDFromPayload` before it builds any
SQL.

The two reads are not interchangeable and the build must not collapse them.
`dialect.JSONExtract(driver, col, path)` returns a SQL fragment over a *column*,
so it applies to the **stored** rows the window count scans
(`## Counting without new state`) and cannot apply to the **incoming** wake,
which is not a row yet. Incoming: Go. Stored: SQL.

Parsing the incoming payload has three failure shapes, which `CoalesceRun`
deliberately keeps apart. The evaluation order above decides where each one
lands, and the mapping is deliberately not symmetric:

| Incoming payload | Where it lands |
|---|---|
| `task_id` a non-empty JSON string | step 3 passes, the gate applies |
| `task_id` absent, JSON null, or `""` | step 3 fails: `AC-OFFICE-ASSIGN-RATE-002.3`, admit and count unattributed |
| `task_id` present but not a string | step 3 fails: `AC-OFFICE-ASSIGN-RATE-002.3`, admit and count unattributed |
| the payload does not parse at all | no readable `actor_type` either, so it fails **step 2** first: out of scope under `AC-OFFICE-ASSIGN-RATE-001.1`, admitted, no counter moved |

The last row is the one to be careful about, and it is part of why the order is
contract. A payload this gate cannot parse is not an unattributable *in-scope*
wake, it is a wake never shown to be in scope at all, and
`AC-OFFICE-ASSIGN-RATE-002.3` says in terms that a wake out of scope by actor
type never reaches it. Counting it as unattributed would count wakes never shown
to be agent-initiated, which `AC-OFFICE-ASSIGN-RATE-001.1` exempts by
definition.

The two task-shaped failures do share one bucket, unlike `CoalesceRun`, which
must hold `invalidTaskID` apart from taskless because it *merges* rows and a
wrong merge overwrites another launch's payload. This gate only counts and both
shapes admit, so splitting them would buy nothing and would add a counter value
`AC-OFFICE-ASSIGN-RATE-003.1`'s closed set does not have.

Step 2 needs no equivalent table: an `actor_type` that is absent, not a string,
or any string other than `agent` simply is not `agent`, which
`AC-OFFICE-ASSIGN-RATE-001.1` already covers as "any other actor type ... or no
actor type at all" — out of scope, admitted, no counter moved.

`actor_type` can be *absent* rather than merely different: the out-of-scope
event-subscriber producer builds its payload as `{"task_id": …}` and carries no
actor at all. Absent therefore has to classify the same way `"user"` does — out
of scope, admitted, no allowance consumed, no counter moved, per
`AC-OFFICE-ASSIGN-RATE-001.1` — and not as a degraded case with its own reason
value. A counter that increments on every Office task creation is not a
regression signal, it is background noise that hides one. What guards against
the in-scope producer actually losing the field is
`AC-OFFICE-ASSIGN-RATE-001.13`'s payload contract, pinned by a test, rather than
a runtime counter that could not tell that loss apart from ordinary traffic.

Because `actor_type` is `"user"` whenever the acting agent identifier is empty
(`runReactivityForAssigneeChange`'s `userSentinel` default), the
internal and administrative caller that passes `callerAgentID == ""` classifies
as `user` and is never limited. `AC-OFFICE-ASSIGN-RATE-001.1` states that as a
contract rather than leaving it as a consequence of the sentinel's value.

## Counting without new state

`AC-OFFICE-ASSIGN-RATE-002.4` requires the gate to hold no state of its own.
One repository read satisfies it:

```text
SELECT COUNT(*) FROM runs
WHERE reason = 'task_assigned'
  AND <json_extract payload task_id>   = :taskID
  AND <json_extract payload actor_type> = 'agent'
  AND requested_at > :windowStart
  AND requested_at <= :evaluationInstant
```

`windowStart` is `evaluationInstant - W`. The lower comparison is strictly
greater-than and the upper comparison is inclusive, which is
`AC-OFFICE-ASSIGN-RATE-001.11`'s half-open interval. `requested_at` is written
by `queueRun` itself on every row, so it is the persisted request instant the
window is measured against.

No `ORDER BY` appears here or anywhere else in this capability:
`AC-OFFICE-ASSIGN-RATE-001.10` makes the decision explicitly order-independent,
which is what a count needs and all it needs. That is not a simplification of a
richer rule — the richer rule was not implementable. The assignment generation
is not orderable from a run row: `runs` has no `assignment_generation` column,
`RunContext` carries no such field, and the generation survives only inside the
string `dedupkeys.AssignmentKey` formats, which a keyless wake does not have at
all. Adding a payload field to carry it would *not* break coalescing —
`CoalesceRun` matches on `agent_profile_id`, `reason`, status, the window, the
comment-key exclusion and `task_id` alone, then overwrites the row's payload
rather than comparing payloads for equality — but the surviving coalesced row
would then carry the *last* merged wake's generation instead of the first, which
makes a payload-borne generation an unreliable ordering key in precisely the
case an ordering would be asked for.

Two portability notes carried over from `CoalesceRun`, which solved the same
problem in the same table:

- Use `dialect.JSONExtract` / `dialect.JSONTypeIsString`, never a literal
  `json_extract`. The runs repository supports SQLite and PostgreSQL, and
  `runs_queue_postgres_test.go` exists precisely because a hand-written
  `json_extract` diverges between them.
- Guard the string type. `CoalesceRun` learned that `{"task_id":42}` textually
  matches an incoming `{"task_id":"42"}`; the same guard applies here or a
  numeric `task_id` in an old payload silently joins another task's allowance.

Deriving the count from `runs` has a deliberate consequence worth stating: the
history-retention capability deleting old run rows lowers an observed count.
That cannot raise the allowance above `N` inside a live window, because
retention operates on rows far older than ten minutes.

### The actor label on a stored row is not stable

The count reads `actor_type` off the **stored** payload, and `CoalesceRun`
rewrites exactly that. It matches on `agent_profile_id`, `reason`, status, the
window, the comment-key exclusion and `task_id` — never on `actor_type` — and
then writes the incoming wake's payload over the surviving row while leaving
that row's `requested_at` untouched. A queued row therefore carries the **last**
merged wake's actor and the **first** wake's request instant.

Reachable, not theoretical. `runReactivityForAssigneeChange` stamps `agent` or
`user` off `callerAgentID`, and one endpoint — `dashboard/handler.go`'s assignee
route — produces both, so two wakes for one (agent, task) inside five seconds
can carry different actors and merge. Per mixed-actor merge the window count
runs one high (an agent wake relabels a queued user row, so a row
`AC-OFFICE-ASSIGN-RATE-001.1` exempts starts counting) or one low (a user wake
relabels a queued agent row, returning a slot).

Neither direction is an agent-only capability: the loosening one needs a human to
assign the same task within five seconds of the agent's own wake, and the
tightening one is conservative and bounded at one row per merge.

Accepted rather than fixed, and recorded in the requirement's
`## Accepted consequence` so the build meets it as a stated consequence rather
than as a surprise. Both available fixes are closed: matching `actor_type` inside
`CoalesceRun` is the coalescing change the requirement's `## Out of scope` holds
unchanged by repeated human decision, and a column or cache of this gate's own is
what `AC-OFFICE-ASSIGN-RATE-002.4` forbids. What the build owes here is a test,
not a mechanism — `## Test plan` carries the mixed-actor coalesce in both
directions, so the behaviour is pinned at its measured value instead of assumed
away.

## Constants

`N = 5` and `W = 10 * time.Minute`, declared once beside
`CoalesceWindowSeconds` and `IdempotencyWindowHours` in
`scheduler/run.go`, and referenced by the tests rather than restated —
`AC-OFFICE-ASSIGN-RATE-002.6`. The duplicate `RunReason*` constant blocks in
`office/scheduler` and `office/service` are a known wart that
`run-dedup-generation.md` already excluded from its scope; do not add a second
copy of these two.

## Failure direction, and why it differs from its neighbour

Three gates in this one function now fail three different ways, so the choice
has to be explicit rather than inferred:

| Gate | On its own read failing | Why |
|---|---|---|
| `checkPauseGate` | **closed** (`ErrPauseGateUnavailable`) | a kill switch that fails open is not a kill switch |
| `CheckIdempotencyKey` | **error** returned to the caller | a duplicate admitted twice is a correctness defect |
| assignment allowance | **open**, counted and logged | a closed failure drops a legitimate wake, trading a bounded cost problem for an unbounded liveness one |

`AC-OFFICE-ASSIGN-RATE-002.2` fixes the third. The in-repo precedent is
`checkIdleSkip` (`service/scheduler_integration.go`), which fails open on a
database error and is documented as doing so in
`apps/backend/internal/office/AGENTS.md`. The degraded admission is counted
under its own reason so that a gate silently admitting everything because its
read is broken is visible, rather than indistinguishable from a workspace that
simply never exhausts its allowance.

`AC-OFFICE-ASSIGN-RATE-002.3`'s unattributed case is the same reasoning applied
to a payload-shape change: if `task_id` stops resolving, every wake becomes
unattributable and the gate stops binding. Counting that case separately turns
a silent regression into a visible one.

## Concurrency

The count read and the subsequent `CreateRun` are not in one transaction, and
`AC-OFFICE-ASSIGN-RATE-002.1` deliberately does not require them to be. Two
concurrent agent-initiated assignment wakes for one task can both read `N-1`
and both insert, yielding `N+1` admitted in the window.

That is accepted rather than fixed. Making it exact would need either a
serialising lock on the task or a conditional insert, both of which put a
contended write on the hot path of a queue shared by every workflow style, to
buy exactness in a bound whose purpose is to turn "unbounded" into "about five
per ten minutes". The asymmetry in `AC-OFFICE-ASSIGN-RATE-002.1` is the part
that matters and is testable: over-admission is bounded by the number of
concurrent evaluations, and under-admission — refusing before `N` are admitted
— is never permitted.

## Observability

A new `expvar.Map`, published beside `office_run_dedup_total` and
`office_run_dedup_keyless_total` in `internal/runs/service/metrics_vars.go`,
counting refusals and degraded admissions by a closed three-value `reason`
label: allowance exhausted, count read failed, task unattributed. Labels follow
the existing `metricLabel("reason", …)` `k=v;k=v` convention.

`AC-OFFICE-ASSIGN-RATE-003.2`'s no-identifier rule is what keeps the map
bounded, matching the reasoning behind `metricReasons`' `custom` fallback: a
process-global `expvar.Map` must not be able to grow once per distinct input.
`AC-OFFICE-ASSIGN-RATE-003.4` keeps admitted volume off these counters
entirely, so the map's absolute values read as "how often did the guard act",
not as traffic.

The Warn log's fields are fixed by the criteria rather than left to the
implementation: `AC-OFFICE-ASSIGN-RATE-003.3` for a refusal,
`AC-OFFICE-ASSIGN-RATE-002.2` for a failed count read (no observed count — the
read did not produce one), and `AC-OFFICE-ASSIGN-RATE-002.3` for an
unattributable task (no task identifier, for the same reason). The
no-identifier rule above governs counter *labels* only; these are log fields.

The reporting helper belongs beside `ReportWindowedDedup` and
`ReportDurableDedup` in `internal/runs/service/dedup.go` and follows their
shape: increment, log, return an outcome.

## The outcome value

`AC-OFFICE-ASSIGN-RATE-003.5` needs an outcome distinct from
`QueueOutcomeQueued`, `QueueOutcomeCoalesced`, `QueueOutcomeDeduped` and
`QueueOutcomeNone`. Reusing `QueueOutcomeNone` would conflate a deliberate
refusal with "no enqueue was attempted, or the attempt errored", which is what
its own doc comment says it means.

So a new value — and one trap: `QueueOutcome` and `QueueOutcomeNone` are
declared **twice**, in `internal/runs/service` and in
`internal/workflow/engine/adapters.go`, with a comment on each stating the two
declarations must match. A new member has to be added to both or the engine
adapter cannot report it.

Callers treat the new value the way they treat `QueueOutcomeCoalesced`: not an
error, and not a queued run. `ApplyTaskMutation`'s `queue` closure already
appends to `res.Runs` only on `QueueOutcomeQueued`, so a refused wake correctly
does not appear in `ApplyTaskMutationResult.Runs` with no change to that
branch.

## What the mutation still does

`AC-OFFICE-ASSIGN-RATE-001.7` is mostly a statement that nothing needs to
change: the gate lives downstream of the mutation, and
`runReactivityForAssigneeChange` already ignores a non-queued outcome. So a
refusal leaves intact the `UpdateTaskAssignee` write and its
`assignment_generation` bump, the `publishTaskUpdated` broadcast, the
`res.InterruptSessionID` hard-cancel of a displaced assignee's session, and the
prev-assignee office session row transition. The criterion exists so a future
refactor that moves the gate upstream has to break a written contract rather
than a coincidence.

It does **not** include a comment or mention wake. `ApplyTaskMutation` calls
`reactToComment` only when the mutation carries a comment, and the assignee
mutator builds its `TaskReactivityChange` without one, so no such wake exists on
this path to preserve. `AC-OFFICE-ASSIGN-RATE-001.7` says so explicitly, because
a regression test that asserted one would fail against correct code.

## Prior art considered

**Searched:** `search_saas_docs` (`category: "ai_sdlc"`) on runaway-loop and
self-trigger rate limiting, then the LangChain deepagents and Cloudflare Agents
SDK documents. Each item is a vendor claim, not evidence the approach works.

- **Cloudflare Agents SDK** passes a `source` to `onStateChanged` and documents
  the best practice as *"Avoid infinite loops: be careful not to trigger state
  updates in response to your own updates"*. Same insight: gate on *who caused
  it*. **We cannot adopt its conclusion**, which is to suppress self-caused
  reactions outright — `AC-OFFICE-RUN-DEDUP-001.3` requires a same-agent repeat
  assignment to wake the agent. We keep the provenance distinction and bound the
  rate instead of suppressing the class.
- **LangChain deepagents** classifies *"Excessive calls (runaway loops)"* as
  system-handled, with the remedy *"cap model and tool calls per run"* — a
  **count cap** over a scope, not a minimum interval. We follow that shape,
  because a cooldown long enough to matter also breaks a legitimate
  assign/hand-back/reassign burst.
- **Concurrency caps are near-universal** and per-principal (Multica, Coder,
  Kiro). All bound *in-flight* work; none bounds the *arrival rate* of
  self-caused triggers, the axis here.

**In-repo precedents.** The requirement's `## Prior art` names three; their call
sites live here. `internal/automation` caps a firing with `MaxConcurrentRuns` and
records a `RunStatusSkipped` run rather than retrying — the better surface in the
abstract, excluded here only because `runs` has no such status. The workspace
pause gate (`checkPauseGateForAgent`) is the established refused-admission
pattern for `runs`: typed error, no row written. `checkIdleSkip`
(`service/scheduler_integration.go`) fails open on a database error, the
precedent `AC-OFFICE-ASSIGN-RATE-002.2` follows and the deliberate opposite of
the pause gate's choice; the table in `## Failure direction, and why it differs
from its neighbour` is that contrast.

## Test plan

Backend, table-driven, in `apps/backend/internal/office/scheduler`:

- `N` agent-actor assignment wakes for one task admit; the `N+1`th refuses,
  inserts no row, and returns the new outcome (`001.2`, `001.6`).
- A `user`-actor wake admits after the allowance is exhausted, and an
  empty-`callerAgentID` call classifies as `user` (`001.1`).
- A wake carrying *no* `actor_type` admits after the allowance is exhausted,
  consumes no allowance, and moves no counter (`001.1`).
- Alternating assignment between two agents shares one allowance (`001.3`).
- A refused wake does not consume the allowance: after `N` admits and several
  refusals, exactly `N` wakes are admitted in the next window (`001.4`).
- A wake inside the coalescing window merges and does not consume the
  allowance; a wake suppressed by the idempotency lookup likewise (`001.5`).
- A keyless wake from the in-scope producer is still bounded (`001.8`).
- The in-scope producer's payload carries `actor_type` `agent` for an
  agent-actor mutation (`001.13`). This is the test that makes losing the field
  a failure rather than a silent unbinding of the gate.
- A wake from the event-subscriber producer is unaffected by an exhausted
  allowance, fixing the producer scope as a test rather than a comment
  (`001.12`).
- Two admitted wakes sharing one persisted request instant both count, and the
  decision at `N` is the same whichever of them was enqueued first — the gate
  reads a count, not a sequence (`001.10`).
- An unassignment neither refuses nor consumes (`001.9`).
- A row whose `requested_at` is exactly `W` old does not count (`001.11`).
- A failing count read admits and increments the degraded counter, and logs
  the fields `002.2` names without an observed count (`002.2`); an
  unattributable task likewise, without a task identifier (`002.3`).
- A wake carrying neither an `actor_type` nor a task moves no counter at all,
  pinning the predicate's evaluation order (`001.1`, `002.3`).
- A mixed-actor coalesce in both directions: an agent-actor wake merging into a
  queued user-actor row, and a user-actor wake merging into a queued agent-actor
  row. Assert the count the gate then observes — the accepted off-by-one of the
  requirement's `## Accepted consequence`, not an assumed exact count — so a
  later change to `CoalesceRun` breaks a test rather than silently moving the
  allowance (`001.4`, `001.5`).
- Re-delivering a *keyed* wake does not consume the allowance twice;
  re-delivering a *keyless* one does, which is what `002.5` states and what
  `001.8` bounds. Two cases, not one.
- The `task_assigned` producers are enumerated and the actor type each one emits
  is asserted, so a new producer setting `actor_type` `agent` fails this test
  instead of joining the gate unannounced (`001.12`).
- An absent, null or empty `task_id` and a present-but-non-string `task_id` each
  take the unattributed path; an unparseable payload instead leaves scope at the
  actor step, admitted with no counter moved, so the two are distinguishable in
  the counters (`002.3`, `001.1`).
- Refusal moves the counter with the expected `reason` label and no identifier
  label, and logs the five fields `003.3` names (`003.1`, `003.2`, `003.3`); an
  ordinary admit moves none (`003.4`).

Time is injected rather than slept — `synctest` per the repo's test discipline,
never `time.Sleep`. PostgreSQL twins of the window-count query belong beside
the existing `runs_queue_postgres_test.go` cases, for the same reason those
exist.

No E2E coverage: `REQ-OFFICE-ASSIGN-RATE-001`'s `## Out of scope` excludes any
frontend, so no user-visible surface changes and there is nothing for
Playwright to drive.
