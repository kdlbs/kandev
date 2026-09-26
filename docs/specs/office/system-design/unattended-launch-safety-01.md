---
status: draft
system: office
requirements:
  - REQ-OFFICE-LAUNCH-SAFETY-001
  - REQ-OFFICE-LAUNCH-SAFETY-002
  - REQ-OFFICE-LAUNCH-SAFETY-003
  - REQ-OFFICE-LAUNCH-SAFETY-004
  - REQ-OFFICE-LAUNCH-SAFETY-005
  - REQ-OFFICE-RUN-CAUSATION-001
  - REQ-OFFICE-BACKPRESSURE-001
  - REQ-OFFICE-BACKPRESSURE-002
  - REQ-OFFICE-BACKPRESSURE-003
  - REQ-OFFICE-ENQUEUE-CONSOLIDATION-001
---

# Office Unattended Launch Safety System Design

## Purpose and boundaries

Office owns the decision to start an agent process. That decision is expressed as
one row transition, `runs.status` from `queued` to `claimed`, performed by
`Repository.ClaimNextEligibleRun` (`internal/runs/repository/sqlite/runs.go`) and
driven by `SchedulerIntegration.tick`
(`internal/office/service/scheduler_integration.go`). Every control in this design
attaches either to that transition or to the enqueue that precedes it.

Adjacent contracts this design uses but does not own:

- `agent_profiles` and its `max_concurrent_sessions` column belong to the agent
  system. This design makes the column load-bearing; it does not change its shape,
  its API projection, or its config-sync behavior.
- `tasks` and `tasks.metadata` belong to the task system. This design writes seven
  reserved metadata keys and reads them back, reusing that system's existing
  `models.MetaKey*` constant family rather than introducing a second convention.
- The cost budget check (`CostService.CheckPreExecutionBudget`, reached through
  `SchedulerIntegration.checkBudget`) is a separate gate with its own fail-open
  behavior, unchanged here.

Two starting-state facts this design corrects, both verified against the code
rather than assumed.

**The claim query does not read the configured ceiling.** It carries a per-agent
lock written as a literal `= 0` on the count of that agent's `claimed` runs, so
`max_concurrent_sessions` is never read. The effective bound is one process per
agent profile and `N` profiles in total, with `maxRunsPerTick = 10` limiting only
how fast the pool fills. `system-design/scheduler-01.md` already specifies
`< a.max_concurrent_sessions`. Only that predicate is carried forward: the rest of
`scheduler-01.md` is superseded, since its claim query reads `office_wakeup_queue`
joined to `office_agent_instances` (removed in ADR 0005 Wave C) and counts
`task_sessions`, which the requirements reject as the unit of account.

**There is no single enqueue choke point today.** Four paths insert a run row:
`office/service.Service.QueueRun` delegating to `runs/service.Service.QueueRun`;
`office/service.Service.queueRunInline`, its fallback when no runs service is
wired; `office/scheduler.SchedulerService.QueueRun`, which calls
`repo.CreateRun` directly and is wired live through the dashboard reactivity and
approval adapters; and `runsServiceEngineAdapter.QueueRun`, which bridges the
workflow engine's `queue_run` step action. Three separate
`IdempotencyWindowHours = 24` constants are the visible symptom. A gate attached to
any one of these is bypassable by the others, which is why
`REQ-OFFICE-ENQUEUE-CONSOLIDATION-001` exists as a separate, prerequisite contract
and this design names the seam below.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ENQUEUE-CONSOLIDATION-001` | Part 1 [Components and responsibilities](#components-and-responsibilities) |
| `REQ-OFFICE-RUN-CAUSATION-001` | Part 1 [Run columns](#run-columns), [Actor contract](#actor-contract), [Task-boundary carrier](#task-boundary-carrier) |
| `REQ-OFFICE-LAUNCH-SAFETY-001` | Part 2 [Claim](unattended-launch-safety-02.md#claim) |
| `REQ-OFFICE-LAUNCH-SAFETY-002` | Part 1 [Launch ledger](#launch-ledger) |
| `REQ-OFFICE-LAUNCH-SAFETY-003` | Part 2 [Enqueue](unattended-launch-safety-02.md#enqueue) |
| `REQ-OFFICE-LAUNCH-SAFETY-004` | Part 2 [Enqueue](unattended-launch-safety-02.md#enqueue) |
| `REQ-OFFICE-LAUNCH-SAFETY-005` | Part 1 [Launch ledger](#launch-ledger), Part 2 [Claim](unattended-launch-safety-02.md#claim) |
| `REQ-OFFICE-BACKPRESSURE-001` | Part 1 [Priority class](#priority-class) |
| `REQ-OFFICE-BACKPRESSURE-002` | Part 2 [Claim](unattended-launch-safety-02.md#claim) |
| `REQ-OFFICE-BACKPRESSURE-003` | Part 2 [Attributing a deferral](unattended-launch-safety-02.md#attributing-a-deferral) |

The requirements are spread over six documents: `unattended-launch-safety.md`
(ceilings and depth), `self-trigger-suppression.md` (the two self-trigger
allowances, carrying `REQ-OFFICE-LAUNCH-SAFETY-004` under its original identifiers),
`launch-budgets.md` (the ledger and the budgets it feeds), `run-causation-chain.md`
(identity), `launch-backpressure.md` (order and observability), and
`enqueue-consolidation.md` (the seam every gate attaches to, and a delivery
prerequisite for the rest).

## Components and responsibilities

- **Authoritative enqueue API.** `runs/service.Service.QueueRun`
  (`internal/runs/service/service.go`) becomes the single insert path. It already
  owns idempotency, coalescing, the insert, the `OfficeRunQueued` publish, and the
  scheduler signal, and it is already the convergence point for the office service
  and the workflow-engine adapter. `office/service.Service.queueRunInline` and
  `office/scheduler.SchedulerService.QueueRun` are reduced to delegating callers;
  neither keeps an insert of its own, and neither keeps a fallback insert for the
  case where the authoritative API is unavailable, per
  AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6. This work is `REQ-OFFICE-ENQUEUE-CONSOLIDATION-001`
  and is a delivery prerequisite for every gate below: it is sequenced first, and it
  is what lets one gate cover every wake reason instead of being written three times
  and drifting. The `AC-...-001.3` guard test asserts against the run-row insert
  itself rather than a list of known callers, because a caller list is what fell
  behind and produced three `IdempotencyWindowHours` constants.
- **Enqueue gate.** A guard inside that one API. It resolves causation and actor,
  applies the depth and both self-trigger limits, stamps the priority class and the
  routine attribution, and either creates the run or refuses.
- **Wake-reason admission.** `runtime.Actions.SpawnAgentRun`
  (`internal/office/runtime/actions.go`) rejects a `SpawnAgentRunInput.Reason` that is
  not a member of `shared.WakeReasonRegistry` before it calls the enqueue API at all,
  per AC-OFFICE-LAUNCH-SAFETY-004.3. That field is the only wake reason an agent
  supplies directly, so it is the only place this check belongs; putting it inside the
  enqueue API instead would have to refuse the system's own reasons and the historical
  ones, which AC-OFFICE-BACKPRESSURE-001.6 requires stay admissible.
- **Claim gate.** The rewritten `ClaimNextEligibleRun`. It applies the three
  ceilings, the two launch budgets, and the ordering, and appends the launch ledger
  row, inside one serialized transaction.
- **Launch ledger.** An append-only record of every claim, which the budgets count
  instead of `runs.claimed_at`.
- **Launch safety configuration.** Typed operator settings resolved through
  `internal/common/config` and passed to the office service at construction, in the
  same style as the scheduler tick interval.
- **Launch safety metrics.** An `expvar` map set modelled on
  `internal/orchestrator/office_stall_metrics.go`, including the fail-closed skip
  counter that makes a degraded gate distinguishable from a quiet system.
- **Runtime causation carrier.** `runtime.RunContext`
  (`internal/office/runtime/context.go`) and the run token minted in
  `SchedulerIntegration.mintRuntimeToken`, extended to carry causation across the
  process boundary and back on every runtime action.

Frontend: the run detail view gains read-only causation fields. No new frontend
control is introduced; the numeric limits are operator configuration, not workspace
UI, per the requirements' out-of-scope notes.

## Data and contracts

### Run columns

Nine columns are added to `runs`, all non-null with defaults so that existing rows
converge without a backfill:

```sql
chain_causation_id TEXT    NOT NULL DEFAULT '',
parent_run_id      TEXT    NOT NULL DEFAULT '',
causation_depth    INTEGER NOT NULL DEFAULT 0,
priority_class     INTEGER NOT NULL DEFAULT 2,
human_rooted       INTEGER NOT NULL DEFAULT 0,
routine_id         TEXT    NOT NULL DEFAULT '',
actor_kind         TEXT    NOT NULL DEFAULT 'system',
actor_id           TEXT    NOT NULL DEFAULT '',
workspace_id       TEXT    NOT NULL DEFAULT ''
```

The column is `chain_causation_id`, not `causation_id`: `runs` already carries an
unrelated `causation_id` column from `REQ-OFFICE-LOOP-LIVENESS-002` (a wakeup-request
correlation), and this design's chain-root identity reached for the same name
independently. `office_launch_ledger` below has no such collision, so its own
column keeps the plain `causation_id` name.

An index on `(chain_causation_id)` supports chain reconstruction, the claim ordering
index becomes `(status, priority_class, requested_at, id)`, and the self-trigger
windows need two: `(agent_profile_id, reason, actor_id, requested_at)` for the
per-reason count of AC-OFFICE-LAUNCH-SAFETY-004.3, and
`(agent_profile_id, actor_id, requested_at)` for the reason-independent count of
AC-OFFICE-LAUNCH-SAFETY-004.8. The second is not redundant with the first: with
`reason` in position two, the per-reason index cannot serve a window range on
`requested_at` for a query that names no reason, so the total count would degrade to a
scan of every run the agent profile ever queued.

`actor_kind` and `actor_id` are persisted because the self-trigger windows of
AC-OFFICE-LAUNCH-SAFETY-004.7 count *past* wakes, and nothing today records that a
queued run was self-caused. The alternatives were rejected on their behaviour, not
their cost: joining a run to its parent's `agent_profile_id` reports a two-agent
A→B→A cycle as not self-caused and stops working when the parent is pruned, and an
in-process counter resets on restart and is not shared between processes. A column is
the only source that answers the question the AC actually asks.

`workspace_id` is persisted rather than joined from `agent_profiles` at claim time.
`runs` carries no workspace today, yet the workspace ceiling, the workspace budget
and the ledger row all need one; resolving it through a join would put a second
table inside the claim statement's counting subqueries. A normal profile supplies
its own workspace. A global Kanban profile has `workspace_id = ''` because
`agentInstanceFilter` selects only profiles with a workspace; for a task-bound
enqueue, the trusted task row supplies the workspace instead. A taskless global
enqueue, an unknown task, or any other empty workspace refuses rather than putting
rows into one shared budget bucket, per AC-OFFICE-RUN-CAUSATION-001.20.

An empty `chain_causation_id` is the legacy marker required by
AC-OFFICE-RUN-CAUSATION-001.6: a reader treats such a row as its own root at depth
`0` and does not rewrite it.

`routine_id` is a real column rather than a value read out of a payload document.
Two different JSON locations name `routine_id` today, the run payload written by
`marshalRoutinePayload` and the context snapshot read by
`models.ContinuationScopeForRun`, and neither is populated for a heavy routine that
works through a task. Promoting it to a column resolves that ambiguity, makes the
per-routine budget predicate indexable, and keeps a JSON extraction out of the
claim statement, where it would have been the second dialect-sensitive construct.

`human_rooted` is persisted and inherited rather than re-derived by joining to the
root run. Retention of run rows is out of scope, so a design that walks to the root
would silently change a run's budget exemption the day its root row is pruned.

`priority_class` is persisted rather than derived in the claim statement, because
the inputs that decide it are the wake reason and the actor and computing it once
keeps the claim statement indexable. Persisting it creates an obligation the first
draft of this design missed: `ScheduleRetry`, `RecoverStale`, and the routing
park-lift re-dispatch each re-queue an existing row, and each must re-stamp the
class to `recovery` unless it is already `human`, per
AC-OFFICE-BACKPRESSURE-001.7. Without that write a retried `periodic` run keeps
`periodic` forever and never receives the recovery priority that exists to let
stuck work drain a saturated pool.

### Launch ledger

```sql
CREATE TABLE office_launch_ledger (
  id           TEXT      PRIMARY KEY,
  run_id       TEXT      NOT NULL,
  workspace_id TEXT      NOT NULL,
  causation_id TEXT      NOT NULL DEFAULT '',
  routine_id   TEXT      NOT NULL DEFAULT '',
  human_rooted INTEGER   NOT NULL DEFAULT 0,
  claimed_at   TIMESTAMP NOT NULL
);
CREATE INDEX idx_launch_ledger_ws_time ON office_launch_ledger(workspace_id, claimed_at);
CREATE INDEX idx_launch_ledger_routine_time ON office_launch_ledger(routine_id, claimed_at);
```

The ledger exists because `runs.claimed_at` is not a durable record of a launch.
`ScheduleRetry` and `RecoverStale` both set `claimed_at = NULL` when they return a
run to `queued`, so a run that was claimed and then retried or recovered leaves the
budget window entirely. Counting `claimed_at` therefore under-counts precisely in
the retry-storm and stuck-agent scenarios the budget exists to bound. The ledger is
append-only: no path modifies or deletes a row except the pruning permitted by
AC-OFFICE-LAUNCH-SAFETY-002.6, and a run claimed twice appends twice, because two
launches happened.

Ceilings still count `runs.status = 'claimed'`, which is live occupancy and
correctly falls when a run finishes. Budgets count the ledger, which is history.
The two ask different questions and must not share a source.

### Actor contract

`runs/service.QueueRunRequest` gains declared fields rather than payload
conventions:

| Field | Meaning |
| --- | --- |
| `ActorKind` | `user`, `agent`, or `system` |
| `ActorID` | the user or agent profile that acted; empty for `system` |
| `CausingRunID` | the run the action was performed inside; empty for a root |
| `RoutineID` | the routine this enqueue is chargeable to; empty for none |

`ActorKind` is **required**, per AC-OFFICE-ENQUEUE-CONSOLIDATION-001.7: it has no zero
value that means "unspecified", so a path that does not set it does not compile rather
than silently resolving to `system`. There is deliberately **no** `WorkspaceID` field
(AC-OFFICE-ENQUEUE-CONSOLIDATION-001.8). The workspace is derived from the woken
agent's profile, or from the target task when the profile is a global Kanban profile;
accepting a caller-supplied workspace would let a caller aim a ceiling or budget at a
workspace unrelated to the trusted profile or task.

**Every enqueue path declares where its actor comes from**, per
AC-OFFICE-RUN-CAUSATION-001.23, in one table beside the request type rather than at
each call site:

| Path | Actor source | Kind |
| --- | --- | --- |
| Runtime action (`POST /runtime/...`) | the authenticated run's agent profile | `agent` |
| User request (API / UI) | the authenticated user | `user` |
| Routine fire | none; the fire is a root cause | `system` |
| Wake caused by a task | the actor on the task carrier | as carried |
| Retry / recovery / re-dispatch | re-queues an existing row; actor unchanged | as persisted |
| Workflow engine `queue_run` action, task carrier unresolved | none; a task with no carrier and no live claimed run is itself a root cause | `system` |
| Step-entry wake (ledger `DispatchStepEntry` actions, Office `auto_start_agent`) | the `task_step_transitions` row for the entry: `agent` resolves the run bound to its `session_id` as the causing run; `human` is a human-rooted root; anything else falls back to the task carrier | `agent`, `user`, or as carried |

The seventh row exists because step-entry wakes run after the transition commits and
carry no live session in `MachineState`. The ledger row is the durable record of who
caused the entry, so the wake reads it instead of a live claimed-run lookup keyed on
an agent profile that the step-entry path never had (AC-OFFICE-RUN-CAUSATION-001.25).

The sixth row is the workflow engine's `queue_run` bridge
(`runsServiceEngineAdapter.QueueRun`, `internal/backendapp/main.go`): it always resolves
a `TaskID` and reads that task's boundary carrier, but a task that never carried a
causation carrier and has no run currently claimed against it resolves an empty
carrier. That empty result is still a declared source — the carrier resolver ran and
found nothing to attribute, which is itself the "root cause" case — so the adapter sets
`ActorKind` to `system` explicitly rather than leaving it unset, matching the routine-fire
row instead of falling through to `normalizeActor`'s caller-never-declared-a-source
counter.

The fourth row is the one this design previously lacked, and its absence is why the
actor is now a carried value rather than a re-derived one. `queueTaskAssignedRun`
(`internal/office/service/event_subscribers.go`) is an event-bus subscriber with no
caller identity, and it reaches the queue through `office/service.Service.QueueRun`,
whose signature carries no actor at all. Both change: the office wrapper takes the
declared fields and passes them through, so the compiler enforces what
AC-OFFICE-RUN-CAUSATION-001.23 requires, and the subscriber's named source is the
carrier rather than nothing.

The existing `scheduler.RunContext.ActorType` is the nearest thing that exists
today, but it lives on only one of the four insert paths, is `omitempty`, and is
absent for `approval_resolved`, `budget_alert`, `agent_error`, routine dispatch, and
the workflow-engine adapter. Keying the `human` class and the self-trigger check on
it as-is would make both silently path-dependent. Converging every caller on the
authoritative API is what lets the field be required rather than best-effort;
`RunContext.ActorType` becomes one producer that populates it.

An absent or unrecognized `ActorKind` resolves to `system` and increments
`office_launch_actor_missing_total` labelled by reason, per
AC-OFFICE-RUN-CAUSATION-001.16. It never resolves to `user`, so a rule keyed on a
human actor fails toward the restrictive answer.

### Task-boundary carrier

The chain breaks at the task boundary: an agent calls `CreateTask`, a task row is
written, `task.created` is published, and `queueTaskAssignedRun`
(`internal/office/service/event_subscribers.go`) queues the wake from an event
subscriber that has no reference to the creating run. The **carrier set** of
AC-OFFICE-RUN-CAUSATION-001.18 is therefore recorded on the task, as seven reserved
`tasks.metadata` keys:

- `office_causation_id`
- `office_causation_depth`
- `office_causing_run_id`
- `office_human_rooted`
- `office_routine_id`
- `office_actor_kind`
- `office_actor_id`

**The set is closed by a rule, and the rule was not strong enough.** Three rounds of
this design each added one key — `office_causing_run_id`, then `office_routine_id`,
then `office_human_rooted` — and a fourth round found a fourth, the actor. The rule of
AC-OFFICE-RUN-CAUSATION-001.18 (a persisted causation value belongs in the carrier
unless a later enqueue can re-derive it *without reading the creating run*) was
correct; its **test** was not. The test asked whether a disposition had been recorded,
and the actor had one — "recomputed from the new wake's own reason and actor" — which
was simply false at the task boundary, where no wake has an actor of its own. A test
that checks a claim exists cannot catch a claim that is wrong.

The rule is therefore now enforced against AC-OFFICE-RUN-CAUSATION-001.23's actor
source table: a value may be kept out of the carrier only when a **named source** is
declared for it at *every* path, and the guard test fails on a path with no source.
Applying that to the nine columns: the causation id, depth, creating run id,
human-rooted flag, routine and actor are carried; the priority class is not, because
AC-OFFICE-BACKPRESSURE-001.3 recomputes it from the new wake's reason and its now
correctly sourced actor; and the workspace is not, because its named source at every
path is the woken agent's profile. A tenth column cannot repeat the pattern without
naming a source that does not exist.

The keys reuse the existing `models.MetaKey*` family in `internal/task/models`
(`MetaKeyAgentProfileID`, `MetaKeyDeferredLaunch`, `MetaKeyQueuedMoveExitPending` and
the rest), which is the established convention for reserved `tasks.metadata` keys and
already fixes their encoding and their absent-versus-empty reading. An earlier draft of
this design asserted no such family existed and specified a bespoke accessor pair in
the office service; that was wrong, and building it would have created a second
convention for one table — the same defect as the three `IdempotencyWindowHours`
constants this capability exists to remove.

`office_human_rooted` is the key this round added, and it is not cosmetic.
`human_rooted` is persisted precisely so a consumer never has to walk to a root that
retention may have pruned — but without carrying it, the wake from an agent-created
task had to read the creating run to obtain it, reintroducing exactly that
dependency. Under AC-OFFICE-RUN-CAUSATION-001.13 the fallback for an unavailable
value is `false`, so the failure was silent and one-directional: a human-rooted chain
would quietly lose its budget exemption once its creating run aged out.

`office_causing_run_id` makes AC-OFFICE-RUN-CAUSATION-001.3 satisfiable across the
boundary, since the causation id names the chain's *root*, not the parent.
`office_routine_id` makes a heavy routine's launches chargeable per
AC-OFFICE-RUN-CAUSATION-001.14; without it the per-routine budget reaches only
lightweight taskless fires. A routine that creates a task writes it with an empty
causing run id at depth `0`, per AC-OFFICE-RUN-CAUSATION-001.5, because a routine
fire is a root cause and has no creating run.

A task created by a human carries none of these keys, so the resulting wake is a root
under AC-OFFICE-RUN-CAUSATION-001.9. A task created by a routine fire carries an empty
`office_causing_run_id`, which makes its wake a root at depth `0` under
AC-OFFICE-RUN-CAUSATION-001.24 rather than a depth-`1` run with an empty causation id —
the carried routine and human-rooted values still apply, so the routine stays
chargeable.

### Configuration

Added to the operator settings catalog (`internal/common/config/catalog.go`), each
with the default named in the requirements:

| Key | Environment variable | Default |
| --- | --- | --- |
| `office.maxConcurrentInstance` | `KANDEV_OFFICE_MAX_CONCURRENT_INSTANCE` | `8` |
| `office.maxConcurrentWorkspace` | `KANDEV_OFFICE_MAX_CONCURRENT_WORKSPACE` | `4` |
| `office.maxCausationDepth` | `KANDEV_OFFICE_MAX_CAUSATION_DEPTH` | `8` |
| `office.workspaceBudgetPerHour` | `KANDEV_OFFICE_WORKSPACE_BUDGET_PER_HOUR` | `120` |
| `office.routineBudgetPerHour` | `KANDEV_OFFICE_ROUTINE_BUDGET_PER_HOUR` | `20` |
| `office.selfTriggerAllowance` | `KANDEV_OFFICE_SELF_TRIGGER_ALLOWANCE` | `3` |
| `office.selfTriggerTotalAllowance` | `KANDEV_OFFICE_SELF_TRIGGER_TOTAL_ALLOWANCE` | `8` |
| `office.promotionAgeMinutes` | `KANDEV_OFFICE_PROMOTION_AGE_MINUTES` | `15` |
| `office.gateFailureThreshold` | `KANDEV_OFFICE_GATE_FAILURE_THRESHOLD` | `3` |

These are the key spellings the catalog carries, flat under the `office.` owner
alongside `office.schedulerTickMs` and `office.jwtSigningKey`. An earlier draft of
this table wrote them as `office.launch.*`, which named no key the system reads: an
operator following it would have set `KANDEV_OFFICE_LAUNCH_*` and seen the default
apply with no error. The flat spelling is authoritative because it is the one that
ships, it matches every other key this owner already has, and no acceptance criterion
requires the nested form. The environment column is part of the contract here for the
same reason — a table of keys alone is what let the two spellings diverge unnoticed.
`office.selfTriggerAllowance` is the per-reason bound of
AC-OFFICE-LAUNCH-SAFETY-004.3 and `office.selfTriggerTotalAllowance` the
reason-independent bound of AC-OFFICE-LAUNCH-SAFETY-004.8; a total below the
per-reason value is honored as configured and logged once at resolution, per that
criterion, rather than being corrected.

A resolved value below the minimum stated in its AC is replaced by the default and
logged at warn level. Per AC-OFFICE-LAUNCH-SAFETY-001.5 every key in this table is a
boot-time-only startup setting: `yamlOnlyStartupKeys` (source.go) resolves it once
from YAML/environment via `clampOfficeLaunchSafetyConfig`, and the resolved
`OfficeConfig` value is handed to the owning repository/service's `SetXxx` method at
construction. None of these keys is registered with ADR 0018's runtime
settings-override tier, so there is no running-instance override path and no second
read site to clamp — unlike a `runtimeflags` registry entry, changing one of these
keys always requires a restart. This is the reason `0` cannot express "unlimited":
the catalog has no sentinel for it and inventing one would make the most dangerous
configuration the easiest typo.

### Priority class

One exported function maps a run to a class, returning both the class and whether
the reason was recognized. It applies AC-OFFICE-BACKPRESSURE-001.3's rules in order
and stops at the first match.

1. `ActorKind == user` yields `human`.
2. A reason only a human can cause yields `human`, whatever the recorded actor:
   `manual_resume_after_failure`.
3. A re-queue by retry, the recovery sweep, or routing re-dispatch yields
   `recovery`.
4. Otherwise the reason decides, using the table below.

| Class | Value | Members |
| --- | --- | --- |
| `recovery` | `1` | assigned by rule 3 only; no reason maps here directly |
| `periodic` | `3` | `routine_dispatch_cron`, `heartbeat` |
| `event` | `2` | every other reason in the registry, by explicit rule |

**Exhaustiveness comes from a registry, not from a list of files.** The reason
constants are consolidated into one declared registry — a single exported,
enumerable collection — and every reason is declared there and nowhere else, per
AC-OFFICE-BACKPRESSURE-001.8. This replaces the file-list approach, which has already
failed twice in this document's own history: an earlier draft omitted the reactivity,
approval-flow and legacy literals, and the draft that fixed that named three source
files (`office/service/run.go`, `office/scheduler/run.go`,
`office/shared/runreasons.go`) while `manual_resume_after_failure` — the one reason
rule 2 depends on — is declared in a fourth, `office/service/failure.go`. A guard
test scoped to a hand-written file list would have passed while missing it.

The two tests are distinct and both are required. AC-OFFICE-BACKPRESSURE-001.4
resolves every reason in the registry and fails if any resolves through the fallback,
which is what makes the `event` default safe to keep: reasons reach `event` by an
explicit rule that names the registry, so the fallback is never how a *known* reason
is classified. AC-OFFICE-BACKPRESSURE-001.8 fails when a reason constant is declared
outside the registry, which is what stops the registry itself falling behind. Without
the second test the first is satisfiable by a registry that has quietly stopped being
complete.

The fallback of AC-OFFICE-BACKPRESSURE-001.6 remains reachable for a reason read off
a persisted row that no longer appears in the registry at all — a downgrade, or a row
older than a retirement. It resolves to `event` and increments
`office_launch_priority_unmapped_total`. Failing closed to `periodic` was rejected: a
new reason is far more likely to be event-driven work than a cron fire, and
misclassifying it as periodic would also make it idle-skippable elsewhere.

Two entries are deliberate and would otherwise look like mistakes.
`routine_dispatch` is **not** `periodic`, even though it is a routine dispatch,
because it is the legacy literal written for every trigger alike and the originating
trigger cannot be recovered from a persisted row. `shared.IsPeriodicTasklessWake`
already refuses to guess in exactly this case and treats such a row as non-periodic;
AC-OFFICE-BACKPRESSURE-001.5 makes that the rule rather than a local habit.
`heartbeat` is retired and has no production writer, but pre-retirement rows may
still be queued, so it stays mapped. `routine_dispatch_event` is `event` because a
"Fire now" or an inbound webhook is event-triggered. `routine_trigger` is `event`:
it has no production writer today, only declarations and a test helper, so it is
exactly the historical-row case AC-OFFICE-BACKPRESSURE-001.4 names.

`recovery` outranks `event` because recovery work is already bounded to five rows
per tick and its purpose is to release capacity that is currently stuck. A saturated
pool that starves recovery cannot drain itself.

## Continued

Control flow, failure handling, persistence, security and observability are in
[Office Unattended Launch Safety System Design Part 2](unattended-launch-safety-02.md).
The two parts are one design; they are split because this one reached the size
ceiling for a system-design document, following the `scheduler-01` / `scheduler-02`
precedent in this system.
