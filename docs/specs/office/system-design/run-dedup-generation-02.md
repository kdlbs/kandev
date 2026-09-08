---
status: draft
system: office
requirements:
  - REQ-OFFICE-RUN-DEDUP-001
  - REQ-OFFICE-RUN-DEDUP-002
  - REQ-OFFICE-RUN-DEDUP-003
  - REQ-OFFICE-RUN-DEDUP-004
---

# Office Run Deduplication — Generation Identity System Design Part 2: observability and verification

Part 2 covers what the queue *reports* when it acts on a dedup key
(`REQ-OFFICE-RUN-DEDUP-004`), the upgrade and failure behaviour, and the test
plan for all four requirements. The key contract itself — the producer audit,
the assignment generation, the per-producer sources, convergence and the keyless
path — is in [Part 1](run-dedup-generation-01.md). The full producer inventory
and the exact Go signatures named below are in
[Part 3](run-dedup-generation-03.md).

## Observability

### Counters

New expvar maps, following `internal/office/scheduler/metrics_vars.go`'s
`k=v;k=v` label model and the counters-only rule stated there.

**Three queue implementations make a dedup decision, not one, and all three are
in scope:**

| Queue | Reached from | Today |
| --- | --- | --- |
| `runs/service.Service.QueueRun` | `office/service.Service.QueueRun` when a runs service is wired | checks the key; maps the unique violation to `Deduped`; logs `Debug` |
| `office/service.Service.queueRunInline` | same, when no runs service is wired (older tests, transitional deployments) | checks the key; returns the raw `CreateRun` error; logs `Debug` |
| `office/scheduler.SchedulerService.QueueRun` | `QueueRunCtx`, and so every reactivity wake | checks the key; returns the raw `CreateRun` error; logs `Debug` |

The third most needs instrumenting: `ApplyTaskMutation` -> `QueueRunCtx` ->
`SchedulerService.QueueRun` is the path a reassignment travels and is the *least*
observable today. Instrumenting only `internal/runs/service` would leave
AC-004.1 / .2 / .3 unmet on the flow the card reports.

To keep one behaviour rather than three, the *reporting* is one exported helper
in `internal/runs/service`: it classifies a suppression as `windowed` or
`durable`, increments the counter, emits the log, and returns the `QueueOutcome`.
All three queues call it. Counters are declared once there; nothing declares a
parallel map. Its name, its two entry points, and what it does with an error that
is *not* a unique violation are specified in
[Part 3](run-dedup-generation-03.md#the-suppression-reporter-and-the-no-op-outcome).

The keyless counter is **not** driven from the queues. It has its own exported
reporter in the same package, `ReportKeylessEnqueue(reason, cause, detail)`, called by the
producer at its own decision site, because only the producer knows whether the key
was never needed or could not be resolved — see
[Unresolvable generation](run-dedup-generation-01.md#unresolvable-generation).

| Name | Labels | Incremented when |
| --- | --- | --- |
| `office_run_dedup_total` | `reason`, `kind` (`windowed` or `durable`), `queue` (`runs` or `wakeup`) | the queue suppresses a wake |
| `office_run_dedup_keyless_total` | `reason`, `cause` (`unresolved` or `by_design`) | a producer calls `ReportKeylessEnqueue` before enqueuing with no dedup key |

`kind` earns the metric: `windowed` is the expected, high-frequency outcome on
the engine redelivery paths, while `durable` means the 24-hour lookup found
nothing and the unbounded index still rejected the insert. After this ships that
is one of three things — a genuine race, a producer minting a colliding key, or a
**legitimate late redelivery** of one occurrence past the lookback window. The
third is not a fault and is not rare: `ParentWakeReconciler` is a level-triggered
backstop that re-derives the identical `wakeOperationID` from current task state
on every tick, with no age bound, so a parent stuck for more than a day produces a
durable hit as normal operation. `Warn` is still the right level — a durable hit
always means the fast path missed something an operator may want to see — but
neither the log text nor any alert built on this counter may call a durable hit an
anomaly on its own. One undifferentiated counter would bury the first two.
`cause` earns its place the same way: `by_design` is high-frequency and expected,
`unresolved` is the generation that should have been carried and was not
(AC-003.3), and by `reason` alone the two are indistinguishable. Cardinality is
the run-reason enum times a small constant, the same order as `routing_*`.

`office_run_dedup_total{queue="wakeup"}` increments where `CreateWakeupRequest`
returns `ErrWakeupIdempotencyConflict` (AC-004.6); that path has no windowed
lookup, so its `kind` is always `durable`. Its `reason` label is
`WakeupRequest.Reason`, the field already on the row being inserted, not the
coarser `Source`, which would collapse every wakeup onto one series.

### Logs

- Windowed hit: `Info`, carrying key and reason. Currently `Debug`.
- Durable hit: `Warn`, carrying key, reason, and resolved agent. Currently
  `Debug`, on both the `runs` and wakeup paths.
- Keyless enqueue with `cause=unresolved`: `Info`, carrying `reason` and the
  `detail` constant naming what failed to resolve — `ReportKeylessEnqueue`'s
  third argument, whose per-producer values
  [Part 3](run-dedup-generation-03.md#keyless-causes-per-producer) fixes. Without
  it the four producers that go keyless under `reason=task_assigned` are
  indistinguishable in the log, which is what AC-003.3 exists to prevent.
  `detail` is a log field only, never a counter label. A `cause=by_design`
  keyless enqueue is counted but **not** logged — it is the normal path for
  several reasons and would be pure noise.

### Outcome propagation

`runs/service.QueueRun` already returns `QueueOutcome`. Office's own enqueue
interfaces discard it: `office/shared.RunQueuer.QueueRun`,
`office/service.Service.QueueRun`, `office/scheduler.SchedulerService.QueueRun`
and `QueueRunCtx`, plus the duplicate declarations in `office/runtime/actions.go`
and `office/approvals/service.go`, all return only `error`. Widen them to
`(runsservice.QueueOutcome, error)` (AC-004.4).

The three queues under [Counters](#counters) produce that outcome, and
`office/service.Service.QueueRun` forwards whichever of its two it used.
`queueRunInline` and `SchedulerService.QueueRun` return no `QueueOutcome` today
and map no unique violation; both gain the mapping from the shared helper, so a
durable conflict on the reactivity path becomes `Deduped` with a nil error
instead of today's `"reactivity run failed"` `Error` log (AC-002.3).

**The wakeup enqueue is outside this widening**, and AC-004.4 now says so in its
own text rather than leaving it to be inferred. `CreateWakeupRequest`
(`office/repository/sqlite/wakeup_requests.go`, interface in
`office/routines/service.go`) keeps its `error` signature: no coalescing and no
windowed lookup, so there is no third outcome to report, and its one suppression
is the sentinel `ErrWakeupIdempotencyConflict`, which AC-004.5 exempts by name.
The exemption covers the signature only — a caller receiving that sentinel must
still not treat it as a failure, which the one live caller already does by
logging and continuing.

**Sizing.** This widening is the highest-diff item here — `.QueueRun(` appears
~112 times across 33 files in `internal/office`, ~9 of them production call
sites. Size it as its own task rather than folding it into "observability".

**A path that enqueues nothing reports `QueueOutcomeNone`.** `QueueOutcome`'s
zero value is the empty string, which is none of `queued`, `deduped` or
`coalesced`. Part 3 gives it a name and a meaning
([the no-op outcome](run-dedup-generation-03.md#the-suppression-reporter-and-the-no-op-outcome)).
This is additive and changes no acceptance criterion: AC-004.4 obliges the queue
to report one of three outcomes *when a caller enqueues a wake*, and a widened
signature returning a non-nil error never meets that condition. The reactivity
pipeline's empty-agent-id guard is **not** an instance of it: that guard lives in
the `queue` closure and returns *before* calling `QueueRunCtx`, so it yields no
outcome at all rather than `QueueOutcomeNone`. `QueueRunCtx` itself has no such
early return.

Callers that have nothing to decide assign the outcome to `_`. No caller may
treat `Deduped` or `Coalesced` as an error (AC-004.5); the reactivity pipeline's
`queue` closure in particular must keep appending to `res.Runs` only for
`QueueOutcomeQueued`, so `ApplyTaskMutationResult.Runs` stops reporting
suppressed wakes as queued ones.

## Key durability

A dedup key is durable only for an enqueue that reached `insertRun`. All three
queues run `CheckIdempotencyKey` -> `CoalesceRun` -> `insertRun`, and
`CoalesceRun` (`internal/runs/repository/sqlite/runs.go`) selects its neighbour
on `agent_profile_id`, `reason`, `status='queued'`, `requested_at` inside the
window, — for `task_assigned` only — the payload's `task_id`, **and
`(idempotency_key IS NULL OR idempotency_key NOT LIKE 'task_comment:%')`**.

That last predicate is the one to be exact about, because two different keys are
in play and only one of them is read:

- **The incoming enqueue's key is neither read nor written here.** The `UPDATE`
  sets `coalesced_count` and `payload` only, so the merged row keeps its own key
  and the incoming key is persisted nowhere. That is what makes a coalesced
  enqueue **persist no row bearing its own key**; neither unique index ever sees
  it. Whether the incoming enqueue may coalesce at all is decided earlier and
  elsewhere, by `shouldCoalesceRun` (`internal/runs/service`), which excludes
  only a `task_comment:`-prefixed key.
- **The predicate reads the candidate *neighbour's* key**, keeping a
  `task_comment:`-keyed row from absorbing another reason's wake. A neighbour is
  therefore matchable when its key is NULL **or** any non-`task_comment:` key,
  including a `task_assigned:<task>:<agent>:<generation>` row. A test that needs
  a matchable neighbour must not construct a `task_comment:`-keyed one — it would
  never match, and the test would pass while asserting nothing.

Two consequences the contract states rather than leaving to be discovered:

- Two producers racing on one key may **both** coalesce into a queued neighbour,
  leaving **zero** rows bearing that key. If the neighbour is claimed between
  their two coalesce checks, the later one inserts and exactly one row bears it.
  AC-002.4 is therefore written as *at most one, never two* rather than *exactly
  one*: that is the only invariant true in every interleaving, and a test must
  not pin the count to one.
- An occurrence that coalesced and is then redelivered *after* the coalescing
  window has no durable key to match, and queues a second run (AC-001.4). That
  duplicate is accepted, not closed: AC-003.2 elects a duplicate over a
  suppression, and keying the merged row would change `CoalesceRun`, which
  [Part 1](run-dedup-generation-01.md#purpose-and-boundaries) holds unchanged.

This is a property of the pre-existing coalescing mechanism, not of the
generation component — but the generation component makes it *reachable* for
`task_assigned`, which is why it is stated here. Before this capability two
assignments inside one window shared a permanent key, so the second was stopped
by `CheckIdempotencyKey` and never reached `CoalesceRun`. With distinct
generations the windowed lookup misses and the coalesce arm is live.

## Failure and recovery

- A producer handed no `assignment_generation` goes keyless, never permanent:
  the wake happens, dedup for that one enqueue does not. The only degraded case.
- A counter increment is best-effort and never fails an enqueue.
- The migration is additive with a `0` default and runs at startup, so both bump
  sites always have the column. Every task starts at `0` and the first assignment
  after upgrade commits `1`, so no task's first new-format key is `...:0`.
- **A second migration rebuilds `tasks` and must carry the column through it.**
  `taskPriorityMigrationStatements`
  (`office/repository/sqlite/base_migrations.go`) recreates `tasks` from an
  **explicit column list** to change `priority` from INTEGER to TEXT. That list
  would silently drop `assignment_generation`, so the column joins it the same
  way eight other columns already do in that file — **its established idiom for
  this exact hazard, followed rather than replaced**:
  1. a defensive `ALTER TABLE tasks ADD COLUMN assignment_generation INTEGER NOT
     NULL DEFAULT 0` before the recreate, with the error swallowed like its
     neighbours (`archived_by_cascade_id`, `wip_admitted`, `external_id` and the
     rest), so the `SELECT` can reference the column on a legacy fixture that
     predates it;
  2. the column declared in the `tasks_priority_new` definition;
  3. `COALESCE(assignment_generation,0)` in the recreate `SELECT`, so values are
     **preserved**, not reset.
  Copying rather than defaulting is what makes this order-independent: it holds
  whether or not the task repository's own migration ran first, so nothing rests
  on an argument about which repository initialises when.
  Left undone the defect is not a hard failure, which is why it is easy to miss.
  The task repository initialises first (`backendapp/storage.go`), so the column
  is added, then dropped by the rebuild, absent for the remainder of that boot,
  then re-added at `0` on the next one — `db.MigrateLogger.Apply`
  (`internal/db/migratelog.go`) is not ledger-backed, so the ALTER re-runs. A
  task previously at generation 3 re-mints `...:1` and collides with its own
  historical key on the unbounded index: this capability's own defect,
  resurrected.
- Events in flight across the upgrade carry no `assignment_generation`. Their
  producer goes keyless with `cause=unresolved` rather than minting a
  legacy-shaped key: one event's lifetime, in the direction AC-003.2 elects.
- No runtime feature flag. A key-format change plus telemetry has no
  partially-migrated state to guard — a mixed fleet writing old and new formats
  does not collide, the property AC-001.8 relies on.

## Testing

Backend `*_test.go` beside each source. Behaviours with no equivalent test today:

- A -> B -> A reassignment wakes the first agent again on the third assignment
  (AC-001.2): three distinct keys, and the third enqueue reports queued or
  coalesced, never deduped. Assert the **outcome**, not a row count — a test
  driving all three inside the 5-second window legitimately gets two rows.
- A repeat assignment whose prior run's `requested_at` predates the window queues
  a run (AC-001.3), driven by writing an aged row. That row is also outside the
  coalescing window, so this case asserts `Queued` exactly.
- Both producers, driven directly with the same task, agent and carried
  generation, derive one key and queue one run (AC-002.1 / .2). At the producers,
  not through a dashboard call — only one fires there.
- A repeat assignment to the **same** agent passes the relaxed reactivity gate,
  bumps, and wakes the agent (AC-001.3), including when the prior run is aged
  past the window; and it does **not** set `InterruptSessionID`. A reassignment
  to a *different* agent still does — both branches of the guard added in
  [The reactivity gate](run-dedup-generation-01.md#the-reactivity-gate).
- A non-assignment task update (title, priority) reaches `syncRunnerInTx` but
  does not bump and queues no `task_assigned` run — the inert-writer claim in
  [Four writers](run-dedup-generation-01.md#four-writers-of-the-same-row-that-must-not-bump).
- A task created with an assignee whose `task.created` event carries
  `assignment_generation` commits generation `1` and queues one run; the same
  creation with the field absent goes keyless with `cause=unresolved` and still
  queues, exercising `fallbackToStoredRunner`.
- **A task created UNASSIGNED publishes `assignment_generation` `0`**, carried on
  `publishTaskEventWithExtra`'s `extra` map, and queues no `task_assigned` run.
  Assigning it afterwards commits `1` and queues one. This pins the boundary that
  keeps a creation and a first assignment from both minting `...:1`, and it is
  the only assertion that catches a create path that hardcodes `1`.
- A producer handed a generation does not read the task row for one: drive
  occurrence 1's producer after occurrence 3 has committed and assert it still
  mints occurrence 1's key.
- A durable conflict on the **reactivity** path (`SchedulerService.QueueRun`, and
  the same for `queueRunInline`) returns `Deduped` with a nil error and does not
  log `"reactivity run failed"` (AC-002.3).
- A key-format table test over **every row of the
  [producer audit](run-dedup-generation-03.md#producer-audit)**, with the
  assertion chosen by what the row produces. **Two assertions, not one:**
  - A row that mints a key asserts its output has a segment that varies across
    two constructed occurrences (AC-001.1), so a new producer reintroducing a
    permanent key fails a test rather than shipping.
  - A row that is **keyless by contract** has no key and therefore no segment,
    so the varying-segment assertion is unsatisfiable and is not applied to it.
    It asserts instead that the enqueue carries an **empty** key and that the
    producer reported the `cause` and `detail`
    [Part 3](run-dedup-generation-03.md#keyless-causes-per-producer) assigns it.
    These are the three permanently-keyless rows of
    [Part 3](run-dedup-generation-03.md#keyless-today--must-report-a-cause) —
    the fourth, `agent_error`, becomes generational here and takes the first
    assertion — plus the three status-driven reactivity reasons.
  The same-set blocker re-resolution named in the requirements' `## Out of scope`
  is the one row that mints a key and is asserted *stable* rather than varying,
  pinning the collapse rather than letting it drift.
- Durable-index conflict increments `office_run_dedup_total` with `kind=durable`
  and returns `Deduped` with a nil error.
- An unresolvable generation enqueues keyless with
  `office_run_dedup_keyless_total{cause=unresolved}`; a status-driven reason
  enqueues keyless with `cause=by_design` and emits no log record.
- `SpawnAgentRun` with an empty agent key and a live caller run id enqueues
  keyless, and two such calls in one run are **not deduplicated**. Assert the
  outcome, not a row count: an empty key passes `shouldCoalesceRun`, and the
  first call's own row — which persisted a NULL key — satisfies the
  `idempotency_key IS NULL` arm of the neighbour predicate in
  [Key durability](#key-durability), whose task-scoping clause applies only to
  `task_assigned`, so two calls to one agent with one reason inside the 5-second
  window legitimately coalesce. Drive them with distinct reasons, or
  outside the window, to assert `Queued` twice.
- Two recipients' wakes for one comment (assignee plus a mentioned agent) mint
  distinct keys and both queue.
- **AC-001.7's run-id prefix, on the non-empty agent key** — the case the
  empty-key bullet above does not reach. Two `SpawnAgentRun` calls passing the
  *same* literal key inside one caller run dedupe; the same literal key from a
  *second* caller run queues, because the prefix differs. A test that drives only
  the empty-key path leaves the prefix logic unpinned.
- **The wakeup queue increments its own counter (AC-004.6).** A
  `CreateWakeupRequest` rejected with `ErrWakeupIdempotencyConflict` moves
  `office_run_dedup_total{queue="wakeup",kind="durable"}` **through
  `ReportDurableDedup`, not `ReportInsertResult`** — the caller does its own
  `errors.Is` on the sentinel, so assert the counter moves without
  `runs/service` ever being handed that error. Labelled by
  `WakeupRequest.Reason` and not by `Source`, and the caller does not treat the
  sentinel as a failure. Nothing else in this plan exercises that file, so
  without this the label can be unwired or wired to the wrong `queue` value with
  the suite green.
- **The four direct keyless producers each report their assigned cause** — the
  onboarding wake and `handleTaskCreated`'s fallback as `unresolved`, the
  recovery sweep and `requeueRunForTask` as `by_design` — per
  [Part 3](run-dedup-generation-03.md#keyless-causes-per-producer). The recovery
  sweep additionally asserts it does **not** mint the assignment key: drive a
  sweep over a task whose assignment run already persisted
  `task_assigned:<task>:<agent>:<generation>` and assert the sweep still queues,
  rather than being suppressed by the durable index.
- **`agent_error` is generational on the failed run id.** Two distinct run
  failures for one agent mint different keys and both queue; a redelivery of one
  failure escalation is suppressed. There is no two-CEO case to assert: a
  workspace admits at most one CEO (`ErrAgentCEOAlreadyExists`) and
  `queueCEOAgentError` escalates to `ceos[0]` alone.
- **Both blocker producers derive a byte-identical digest** for one blocker set,
  driven through the shared builder from both packages, including a set whose ids
  were returned in a different order. An empty blocker set enqueues nothing and
  moves no keyless counter.
- **A path that enqueues nothing returns `QueueOutcomeNone`**, not
  `QueueOutcomeQueued`: a widened signature returning an error. Assert
  `ApplyTaskMutationResult.Runs` gains no entry. The `queue` closure's
  empty-agent-id guard is a **separate** case with a different assertion — it
  reaches no queue at all, so assert `res.Runs` gains no entry and that
  `QueueRunCtx` was never called, not that it returned an outcome.
- **`QueueOutcomeNone` is declared identically in both packages.** A compile-time
  or table assertion that `runsservice.QueueOutcomeNone` and
  `engine.QueueOutcomeNone` hold the same value, pinning the "both MUST match"
  invariant those two declarations already carry.
- **`ReportInsertResult` passes a non-conflict error through unchanged**, with
  `QueueOutcomeNone` and no counter movement — a disk error is not a dedup
  decision and must not be counted or swallowed as one.
- **The `tasks` rebuild preserves the column and its values.** Drive
  `taskPriorityMigrationStatements` over a legacy fixture whose `tasks.priority`
  is still INTEGER, seeded with one row at `assignment_generation` 3 and one at
  `0`, and assert both survive the recreate unchanged. Also drive it over a
  fixture predating the column entirely and assert the defensive ALTER makes the
  `SELECT` succeed at `0`. Without this the loss is invisible: the column is
  re-added on the next boot, so nothing fails and only the resurrected key
  collision would ever show it.
- Routine keys, per source: two cron fires of one trigger on **different claimed
  ticks** mint different keys; one slot re-dispatched with the same claimed tick
  mints the identical key; two **manual** fires of one routine inside one minute
  mint different keys (today they collide); and a `source == "cron"` dispatch with
  no claimed tick carried enqueues keyless with `cause=unresolved`. The cron case
  is driven by supplying the claimed tick directly, not by advancing a clock.
- A cron producer does not recover the tick by re-reading the trigger row
  (AC-001.9): drive a dispatch after `UpdateTriggerNextRun` has advanced
  `next_run_at` and assert the key still names the claimed slot, not the next one.
- `ReportKeylessEnqueue` is what moves `office_run_dedup_keyless_total`: a
  producer that goes keyless increments its own `cause` and logs its own
  `detail` constant — assert two producers going keyless under
  `reason=task_assigned` are distinguishable in the log record, which is the
  whole reason the argument exists; and an enqueue with an
  empty key that did **not** call the reporter moves no counter — the assertion
  that keeps the withdrawn "empty key means `by_design`" inference from returning.
- An occurrence that coalesced into a queued neighbour and is then redelivered
  after the coalescing window queues a **second** run (AC-001.4, see
  [Key durability](#key-durability)): enqueue generation N, enqueue generation
  N+1 inside the window and assert `Coalesced`, age the merged row past the
  window, then re-enqueue generation N+1's key and assert `Queued`. This pins the
  accepted duplicate — a test asserting one run here is asserting the wrong
  contract.
- Two producers enqueuing one key concurrently **with** a coalescible run already
  queued for that agent, reason and task: both observe a suppression, neither
  errors, and no row bears the new key (AC-002.3, AC-002.4). The no-neighbour
  arrangement is the separate case the same ACs bound, and needs its own test.
- A durable-index conflict produced by re-deriving one occurrence's key past the
  lookback window (the `ParentWakeReconciler` shape) increments
  `office_run_dedup_total{kind=durable}`, returns `Deduped` with a nil error, and
  is not reported as a failure anywhere.

No Playwright coverage: the observable surfaces are `/debug/vars` and backend
logs, and the one user-visible consequence (a run row appearing for a repeat
assignment) is asserted at the queue in Go.
