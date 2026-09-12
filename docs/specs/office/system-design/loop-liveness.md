---
status: draft
system: office
requirements:
  - REQ-OFFICE-LOOP-LIVENESS-001
  - REQ-OFFICE-LOOP-LIVENESS-002
  - REQ-OFFICE-LOOP-LIVENESS-003
  - REQ-OFFICE-LOOP-LIVENESS-004
  - REQ-OFFICE-LOOP-LIVENESS-005
---

# Office Loop Liveness System Design

## Purpose and boundaries

This design adds one write to the routine dispatch path, one identifier carried
across three Office tables, one set of counters, and two read-only workspace
endpoints. It changes no scheduling decision.

Adjacent contracts read and constrained but not owned:

- `internal/scheduler/cron` — the shared 30-second cron loop, `RoutinesHandler`.
- `internal/office/routines` — dispatch, catch-up, concurrency policy.
- `internal/office/wakeup` — the dispatcher's coalesce/skip/fresh-run branches.
- `internal/office/service` — `SchedulerIntegration`'s 5-second drain, claim,
  guards, terminal transitions.
- `internal/runs/repository/sqlite` — the shared `runs` table, `FinishRun`, and
  `ClaimNextEligibleRun`, whose filter the stuck-run predicate mirrors.
- `docs/specs/task-delivery-ledger/spec.md` — owns `runs.outcome`. Not modified.

## Prior art

Two external legs (the compiled wiki, and a cross-vendor comparison) were attempted
and both were UNAVAILABLE; receipts are in the card's plan, and nothing is claimed
here about what any vendor ships.

**In-repository prior art, which was read.** Existing patterns are adopted, and one
is deliberately departed from:

- `docs/specs/office/requirements/stall-visibility.md` is the closest sibling:
  detection-only, `expvar.NewMap` counters with a `k=v;k=v` label model, and a skip
  counter labelled by reason. This capability copies all three and **departs on
  failure direction**; see [Failure](#failure).
- `docs/specs/task-delivery-ledger/spec.md` owns `runs.outcome`, so this design
  derives a classification on top rather than widening it. Its
  `telemetry.run_outcome.activated_at` key, written only after a positive schema
  probe, is the pattern reused for the activation instant.
- `internal/office/service/wake_metrics.go` states the argument this generalizes:
  "production cannot otherwise tell a working sweep (candidates found, nothing to
  do) from a dead one (handler not ticking at all)."

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-LOOP-LIVENESS-001` | [Routine fire recency](#routine-fire-recency) |
| `REQ-OFFICE-LOOP-LIVENESS-002` | [Causation identity](#causation-identity) |
| `REQ-OFFICE-LOOP-LIVENESS-003` | [Counters](#counters), [The counter read](#the-counter-read) |
| `REQ-OFFICE-LOOP-LIVENESS-004` | [The liveness read](#the-liveness-read) |
| `REQ-OFFICE-LOOP-LIVENESS-005` | [Terminal shapes](#terminal-shapes) |

## Persistence

Three additive columns and three partial indexes, applied with the existing
`r.migrate.Apply` idiom in `internal/office/repository/sqlite`. No table is
created, no column is dropped, no value is backfilled.

| Table | Column | Type | Meaning |
| --- | --- | --- | --- |
| `office_routine_runs` | `causation_id` | TEXT NOT NULL DEFAULT `''` | The id minted by this fire. |
| `agent_wakeup_requests` | `causation_id` | TEXT NOT NULL DEFAULT `''` | Copied from the fire, or minted when the wake has no routine origin. |
| `runs` | `causation_id` | TEXT NOT NULL DEFAULT `''` | Copied from the wakeup request that created the run. |
| `office_routines` | — | — | No new column. `last_run_at` already exists. |

`''` rather than `NULL` because every existing row in all three tables predates
this feature and the reader must treat "legacy" and "uncorrelated" identically.

An index is added on the `causation_id` of **all three** tables, each **partial to
`causation_id != ''`**, on both dialects. Partial because the legacy rows would
otherwise collapse into one bucket a lookup could accidentally join. All three
rather than `runs` alone because NFR-2 is symmetric: an operator walks *backwards*
to the originating row as often as forwards, and an unindexed backward hop is a
table scan wearing the words "direct lookup".

### Activation

`telemetry.office_loop_liveness.activated_at` is written into `kandev_meta` by
`persistence.WriteMetaKeyIfAbsent` at boot, only after a positive `columnExists`
probe of all three `causation_id` columns — the same shape as `activateRunOutcome`,
for the same reason: the migration runner swallows failures at `WARN`, so the probe
is all that stands between a failed migration and a consumer believing the
mechanism is live.

The instant matters beyond bookkeeping. Every `outcome = 'processed'` row on the
reference instance has `session_id = ''` because nothing wrote it; classifying them
as silent successes would report dozens of defects on the day this ships and train
the operator to ignore the number. A run whose `requested_at` precedes the
activation instant is therefore `pre_activation`, so `silent_success` counts only
rows that could have been correlated.

## Routine fire recency

One new repository method, deliberately narrow:

```go
TouchRoutineLastRun(ctx context.Context, routineID string, at time.Time) error
```

```sql
UPDATE office_routines
SET last_run_at = ?, updated_at = ?
WHERE id = ? AND (last_run_at IS NULL OR last_run_at < ?)
```

It is **not** `UpdateRoutine`, which writes eleven columns from an in-memory
struct: using it here would turn a dispatch into a read-modify-write that clobbers
a concurrent edit from the UI or a config sync (AC-001.5).

**The other writers, surveyed**, because a monotonic statement secures the
statement, not the column. `last_run_at` is written by the insert, by
`TouchRoutineLastRun`, and by `UpdateRoutine`, which carries it unconditionally
from `routine.LastRunAt`. That third writer defeats REQ-001: a caller holding a
routine loaded before a fire, or one that never populated the field, silently puts
a stale value or `NULL` back over a fresh instant and restores the "reads as never
ran" state. `UpdateRoutine` therefore **drops `last_run_at` from its column list**
(AC-001.8); the field stays on the struct for reads, and
`UpdateRoutineConfigFields` beside it already excludes the same column for the same
stale-snapshot reason. `last_run_at` then moves only forward, and only from a fire.

The `last_run_at < ?` guard makes the write monotonic, hence idempotent (AC-001.3)
and order-independent under concurrency (AC-001.4): whichever of the catch-up
dispatch and a manual fire commits first, the surviving value is the greater
instant, because the earlier fails the predicate. A replay is a zero-row no-op, not
an error (AC-001.6).

The call site is `dispatchRoutineRun`, immediately after `CreateRoutineRun` returns
and **before** `applyConcurrencyPolicy`, so a coalesced or skipped fire advances the
column (AC-001.1). The value passed is `*run.StartedAt` — the `now` also written
into the routine-run row, so the two are the same instant by construction
(AC-001.2). The existing `routine.LastRunAt = &now`, which mutates a struct nobody
persists, is deleted. Errors are logged and counted, never returned (AC-001.7): a
routine that fired must not read as having failed to fire because a bookkeeping
write lost a lock.

## Causation identity

### Minting

A causation id is a UUIDv4 string, minted at exactly one place per origin:

| Origin | Minted in | Persisted first on |
| --- | --- | --- |
| cron routine fire | `dispatchRoutineRun` | `office_routine_runs.causation_id` |
| manual / webhook routine fire | `dispatchRoutineRun` | `office_routine_runs.causation_id` |
| every other wake | the `agent_wakeup_requests` writer | `agent_wakeup_requests.causation_id` |

`dispatchRoutineRun` is a single funnel for all three routine sources, so one mint
site covers them (AC-002.1). The lightweight path copies the id onto the
`WakeupRequest` it builds (AC-002.2); `createFreshRun` copies it from the request
onto the run (AC-002.3).

The routines path is today the **only** production writer of
`agent_wakeup_requests`, so the third row is a rule for a future writer, not a
second implementation site now. Any new wake source mints at its own origin:
inheriting an id from an unrelated wake would make two causes look like one.

### What must not change it

The dispatcher has three branches and only one creates a run:

- `MarkWakeupRequestCoalesced` and `MarkWakeupRequestSkipped` touch the request,
  never the run. The in-flight run keeps the id of the wake that created it, the
  coalesced request keeps its own, and the two stay joinable through
  `agent_wakeup_requests.run_id` (AC-002.4). First writer wins: a run whose
  identity changed under a reader would break the one thing the id is for.
- `PromoteRunAndCoalesceWakeupIfQueued` promotes the run's `reason` and must not
  touch `causation_id` — a promotion is not a new cause.
- `createFreshRun` is the only writer, including where promotion lost the CAS race
  and the event gets its own run: that run carries the *requesting* wake's id.

Retry, park and lift (`ScheduleRetry`, `LiftParkedRuns`) mutate the existing row
and never re-create it, so AC-002.5 holds with no code change and is asserted by
test.

### Empty is not a key

`''` means "not correlated". A reader must never `GROUP BY causation_id` or join on
it without excluding `''` (AC-002.6). Every query this design adds carries the
exclusion in SQL rather than trusting a caller to remember it, and the partial
index makes the exclusion the cheap path.

### The session hop

`runs.session_id` is `NOT NULL DEFAULT ''` with exactly two writers today.
`ContextBuilder.BuildAndPersist` writes it from `run.Payload["session_id"]`, empty
for every Office-created run. `RequeueRunForNextCandidate` (`run_routing.go`)
*clears* it, in the same statement that returns the run to `queued` and nulls
`claimed_at` and `finished_at` — easy to miss, and it bounds what AC-002.10 can
promise.

The session id that does exist is returned by the orchestrator and thrown away at
the Office seam: `orchestrator.Service.StartTask` returns a
`*executor.TaskExecution` whose `SessionID` is "the database ID of the agent
session", and both `schedulerTaskStarterAdapter.StartTask` and
`orchestrator.Service.StartTaskWithRoute` discard it.

The fix widens the Office-facing seam, using the optional-interface idiom the
package already uses for `TaskStarterWithEnv`:

```go
type TaskStarterWithSession interface {
    StartTaskReturningSession(...) (sessionID string, err error)
}
```

with the routed path widened symmetrically. On success the scheduler persists the
returned id onto the run before `launchAgent` returns (AC-002.7). The production
adapter is **required** to satisfy the interface — no silent fallback — pinned by a
wiring completeness test, the device `TestOfficeRouteGroupMountsScopeGuard` uses.

An empty returned session id is left empty and counted as
`office_loop_launch_without_session_total` (AC-002.8), never replaced with the run
id or any other placeholder: a placeholder would make a broken hop look correlated.

The *write itself* can also fail with a real session id in hand, and that is the
dangerous case: the row it leaves behind — `outcome = 'processed'`,
`session_id = ''` — is byte-identical to a genuine `silent_success`, so a feature
built to detect that defect would manufacture it. Hence AC-002.11: do not fail the
launch, because the agent is already running; do not retry, because a failure of a
single-row `UPDATE` by primary key means the database is unavailable and a retry
loop would block the 5-second drain; and count it under a **distinct** counter,
`office_loop_session_persist_failed_total`. The column keeps *whatever it already
held*, because on a relaunch it may already name a live session. The log line
carries the run id and the unstored session id. The two counters are not merged:
the stored rows are identical, so the counters are the only place the two causes
stay apart.

One run can be launched twice: `failTasklessRun`'s doc comment records the
post-start-fallback case, and the requeue reuses the same row. The write is **last
non-empty wins** (AC-002.10), ordered by `runs.claimed_at` — a *total* order, not a
race: a run is launched only while it holds a claim, and a relaunch requires a
return to `queued` and a fresh claim, so two launches are never in flight at once.
The guard is asymmetric on purpose —

```sql
UPDATE runs SET session_id = ? WHERE id = ? AND ? != ''
```

— so the launch write never replaces a known link with an empty one.

Its reach is narrow. `RequeueRunForNextCandidate` clears `session_id` on the way
back to `queued`, so the guard stops an empty write clobbering a good id *within one
claim* and cannot preserve one across a requeue. AC-002.10 is scoped to the launch
write accordingly, and a relaunch yielding no session id leaves the run
uncorrelated, counted under AC-002.8.

### The heavy path

`materialiseHeavyRoutineRun` writes `causation_id` and `linked_task_id` on the
routine run (AC-002.9). The task is then started through the orchestrator, not
queued through the `runs` table this design instruments — `runs` carries
`agent_profile_id` and no task id, so there is no causation hop to add there. The
reachable path is `office_routine_runs.linked_task_id` → `tasks.id` →
`task_sessions.task_id`, a two-key join, declared as such in the requirements'
**Out of scope**.

## Counters

`expvar.NewMap` with the `k1=v1;k2=v2` label convention already used by
`internal/office/scheduler/metrics_vars.go` and
`internal/orchestrator/office_stall_metrics.go`, in a new
`internal/office/service/loop_metrics.go`.

| Counter | Labels |
| --- | --- |
| `office_loop_cron_tick_total` | — |
| `office_loop_trigger_claimed_total` | `workspace` |
| `office_loop_routine_run_total` | `workspace`, `source`, `disposition` |
| `office_loop_wakeup_created_total` | `workspace`, `source` |
| `office_loop_run_claimed_total` | `workspace` |
| `office_loop_launch_total` | `workspace` |
| `office_loop_launch_without_session_total` | `workspace` |
| `office_loop_session_persist_failed_total` | `workspace` |
| `office_loop_terminal_total` | `workspace`, `shape` |
| `office_loop_last_run_at_write_failed_total` | `workspace` |
| `office_loop_liveness_degraded_total` | `workspace`, `reason` |

Two published instants deviate from the "counters only" note in `metrics_vars.go`:
`office_loop_cron_tick_at` and `office_loop_process_started_at` never decrease, so
they are not the drifting gauges that note warns about, and without them a flat
counter is ambiguous between a restarted process and a stopped loop (AC-003.6).

AC-003.9's two increment rules split this table. The six that **name a persisted
change** (`trigger_claimed`, `routine_run`, `wakeup_created`, `run_claimed`,
`launch`, `terminal`) increment at the persisting site after the write succeeds, so
each counts rows that exist rather than attempts. The five that **name an event
persisting nothing** (`cron_tick`, `launch_without_session`,
`session_persist_failed`, `last_run_at_write_failed`, `liveness_degraded`)
increment at the observing site — three exist precisely to count a failure, so
gating them on a successful write would silence the signal they carry.

These count **transitions, not entities**: a run requeued after a provider fallback
reaches a terminal state twice and contributes two `office_loop_terminal_total`
events, so no consumer may read the counter as a distinct-run count. Deduplicating
would need per-run state in the metric, which the label-cardinality rule forbids:
cardinality is bounded by the install's Office workspaces, and no label carries a
run, task, agent or routine id.

When an event's workspace cannot be resolved — a failed agent row read, an orphaned
`agent_profile_id` — the counter records `workspace=_unattributed` rather than
dropping the event or guessing (AC-003.8); dropping it would make a partly-broken
loop look quieter than a healthy one. It is never added into any workspace's
values, but nor is it hidden behind `/debug/vars`: it is returned in the
**process-scoped** block below. Every workspace-labelled value is filtered to the
requesting workspace before serialization (AC-003.5).

### The counter read

Expvar publication keeps `/debug/vars` working (AC-003.7), but that route is
registered only under `p.devMode` (`internal/backendapp/helpers.go`) and is an
unfiltered global dump that could not satisfy AC-003.5 even where mounted. Hence:

`GET /api/v1/office/workspaces/:wsId/loop-counters`

Mounted on the Office route group beside the liveness read, inheriting
`AgentAuthMiddleware` and `officeWorkspaceScopeMiddleware`; `:wsId` is its only id,
so it needs no new `officeParamScopeResolvers` entry and leaves
`TestOfficeRouteScopeCompleteness` passing. Expvar remains the single source of
truth — this route is a filtered projection, not a second accounting.

The response has two blocks, and that split is the isolation boundary:

- **`workspace`** — every counter above carrying a `workspace` label, filtered to
  the requesting workspace, other labels (`source`, `disposition`, `shape`,
  `reason`) preserved. Zero-filling is at *counter-name* granularity: every name in
  the table appears with a zero total even if unobserved, so "no events" is
  distinguishable from "key absent", while secondary-label breakdowns appear only
  as observed — their cross-product would invent combinations the code never emits.
- **`process`** — `office_loop_cron_tick_total`, the two instants, and the
  `_unattributed` totals: identical for every reader, no workspace's data
  (AC-003.8).

Reading parses the `k1=v1;k2=v2` keys back out of each `expvar.Map` and keeps only
entries whose `workspace` matches. The parse is total — a key that does not split
into the expected labels lands in a malformed bucket in `process` rather than being
dropped, on the same argument as `_unattributed`.

Counters are process-lifetime totals and reset on restart, which is what
`office_loop_process_started_at` is for. The route takes no query parameters
(AC-004.14).

## Terminal shapes

A pure function over three persisted columns plus the activation instant. It
writes nothing (AC-005.8) and reads no free text (AC-005.7).

It is defined only over **terminal** runs, where terminal means "not `queued` and
not `claimed`" — an exclusion rather than an enumeration, so a status added later is
classified rather than silently skipped. Those two have no shape and are excluded
from every terminal total (AC-005.9); they are the stuck-run detector's input.
`runs` has no `running` status — `claimed` is the executing state.

AC-005.2's totality ranges over the values actually written to `runs.status`, not
the Go `RunStatus` enum, which omits `cancelled` (written by
`internal/runs/repository/sqlite/cancel.go`). No writer of `timed_out` exists; it
appears below only as a defensive read. The cross-product test therefore enumerates
`finished`, `failed`, `cancelled`, `timed_out` and an unknown value.

Evaluated top to bottom, first match winning. The final row makes it total
(AC-005.2), which absorbs the historical `no_agent_launched` value without a
migration.

| Shape | Predicate |
| --- | --- |
| `pre_activation` | `requested_at` < activation instant, or activation instant unpublished |
| `launched_completed` | `status = 'finished'` and `outcome = 'processed'` and `session_id != ''` |
| `launched_failed` | `status IN ('failed','timed_out','cancelled')` and `session_id != ''` |
| `silent_success` | `status = 'finished'` and `outcome = 'processed'` and `session_id = ''` |
| `unlaunched_skipped` | `status = 'finished'` and `session_id = ''` and `outcome IN ('idle_skipped','budget_blocked','agent_inactive','task_tree_held')` |
| `unlaunched_failed` | `status IN ('failed','timed_out','cancelled')` and `session_id = ''` |
| `unclassified` | anything else, including `outcome IS NULL` and unknown values such as `no_agent_launched` |

`silent_success` is the NFR-1 shape (no unverifiable success — see the requirements'
[Source obligations](../requirements/loop-liveness.md#source-obligations)): an
outcome asserting work happened on a run that never named a session (AC-005.4).
`unlaunched_skipped` is the legitimate no-launch, kept apart from
`unlaunched_failed` (AC-005.5).

The ordering is load-bearing: `pre_activation` is first so the activation guard
cannot be bypassed by a later row matching; `silent_success` sits below
`launched_completed` so the session test is what separates them, and above
`unclassified` so a null outcome never absorbs it.

`unlaunched_skipped` carries `session_id = ''` for the same reason
`unlaunched_failed` does: without it a `finished` run with a skip outcome *and* a
recorded session would be labelled "unlaunched", contradicting AC-005.3. That cell
should never occur — the skip outcomes are pre-launch guards — but the cross-product
test reaches it, and it falls to `unclassified`.

## The liveness read

`GET /api/v1/office/workspaces/:wsId/loop-health`

Mounted on the Office route group, so it inherits `AgentAuthMiddleware` and
`officeWorkspaceScopeMiddleware`; `:wsId` is the only id it names, so it needs no new
`officeParamScopeResolvers` entry and satisfies `TestOfficeRouteScopeCompleteness`
as written. When the Office feature is off the group is not mounted and the route is
a 404.

### Thresholds

Named constants, all returned in the response (AC-004.8):

| Constant | Value | Why |
| --- | --- | --- |
| `officeLoopTriggerOverdueGrace` | 3m | Six ticks of the 30s cron loop — far above jitter, and below the 5-minute coordinator routine's period, so a missed period is visible before the next is due. It measures the *loop*, not the routine's period, so it holds for a daily cron too. |
| `officeLoopTriggerStrandedGrace` | 5m | A claim clears `next_run_at` and the re-arm follows within one tick. Five minutes is ten ticks; past that the `NULL` is not a race, it is permanent. |
| `officeLoopRunQueuedGrace` | 2m | Applied only to a **claimable** run, so it measures the drain, not the queue: the drain ticks every 5s and takes 10 runs a tick, so two minutes without claiming a run the filter accepts means it is dead or starved. A run waiting on a claimed sibling is excluded from the predicate, not aged by it. |
| `officeLoopRunClaimedGrace` | 10m | Detection must precede recovery, so this is deliberately **not** `staleClaimedRunAge` (30m), the *recovery* threshold: sharing it would let a future tuning of recovery silently retune detection. |
| `officeLoopEvaluationWindow` | 24h | Terminal-shape counts and silent successes cover `COALESCE(finished_at, requested_at) >= now − 24h`, lower bound inclusive. The `COALESCE` is load-bearing: `finished_at` is nullable, so filtering on it alone would let a terminal run recording no finish instant vanish from every total (AC-004.16); `requested_at` is `NOT NULL`, so the fallback is total. It does **not** bind stuck runs. |
| `officeLoopEvidenceCap` | 50 | Per list. |

### Queries

Workspace scoping follows the existing join, `runs → agent_profiles.workspace_id`
(`ListRuns`); triggers scope through
`office_routine_triggers → office_routines.workspace_id`. That run join is inner and
no foreign key backs it, so a run naming a deleted profile reaches no workspace; the
requirements exclude it by name and route it to the unattributed counter.

The trigger predicate is written out rather than reusing `GetDueTriggers`, whose
`next_run_at IS NOT NULL AND next_run_at <= ?` clauses are fatal here: every
stranded trigger has `next_run_at IS NULL` and would be filtered out, deleting the
`dead` path, while a trigger due later today would read as `not_armed`. Eligibility
is `kind = 'cron' AND enabled = 1`; `next_run_at` then splits eligible triggers into
overdue, stranded and claiming.

The stuck-run predicates name their clocks, because `runs` has no single "waiting
since" column. Claimed age is `claimed_at`, treating `NULL` as stuck. Queued age is
the **queue-eligible instant**, `COALESCE(scheduled_retry_at, requested_at)`,
restricted to `current_route_attempt_seq = 0` — the restriction is not defensive:
`RequeueRunForNextCandidate` nulls `claimed_at` but leaves `requested_at` at the
first request, so without it every post-start provider fallback would read as stuck
the moment it requeued.

**The queued predicate mirrors `ClaimNextEligibleRun`** (AC-004.17): a `queued` run
is stuck only if the scheduler would claim it *now* and has not, so it carries that
filter's exclusions — a future `scheduled_retry_at` (the `COALESCE` handles it: a
future instant is never older than `now − grace`), `routing_blocked_status IS NOT
NULL`, and `NOT EXISTS (SELECT 1 FROM runs s WHERE s.agent_profile_id =
r.agent_profile_id AND s.status = 'claimed')`. Without that last clause the detector
reports the normal steady state of a busy agent: sessions routinely outlast two
minutes, so every sibling queued behind one crosses the grace and the workspace
reads `degraded` forever — the false alarm that teaches an operator to ignore the
verdict. A parked run is the same story: `parkRunMaxAttempts` parks *for* a human.
What survives the filter is a run nothing is holding back that was not claimed.

Neither predicate is windowed (AC-004.5) — `officeLoopEvaluationWindow` binds
terminal rows, and a run stuck longer than the window is the one an operator most
needs to see.

Ordering is by named columns with a named tiebreak (AC-004.9):

- triggers — **one list carrying both overdue and stranded rows**, each naming its
  condition (AC-004.7): `ORDER BY (next_run_at IS NULL) DESC, next_run_at ASC,
  id ASC` — stranded first, because they are permanently dead rather than merely
  late. One list rather than two because the ordering is what ranks them against
  each other, and two lists would leave "which is worse" to the reader.
- stuck runs — also one list, each row naming its condition, ordered by the
  **stuck instant** (`claimed_at` when claimed, `COALESCE(scheduled_retry_at,
  requested_at)` when queued): `ORDER BY (status = 'claimed') DESC,
  (stuck_instant IS NULL) DESC, stuck_instant ASC, id ASC`. Two clocks cannot share
  one column, so ranking by `requested_at` alone would put a long-dead claimed run
  below a merely old queued one. Claimed first because such a run holds its
  profile's only claim slot and blocks every queued sibling behind it; `NULL` first
  because a claimed row with no `claimed_at` has no age at all. The `(expr) DESC`
  idiom is the trigger list's, and sorts identically on both dialects unlike
  `NULLS FIRST`.
- silent successes: `ORDER BY COALESCE(finished_at, requested_at) DESC, id ASC` —
  newest first, because a silent success is only actionable while the session it
  failed to name might still be found.

**Silent successes are an evidence list, not just a count** (AC-004.7): they can
decide `degraded` on their own, and "degraded, one silent success" without naming
the run sends the reader back to hand-written SQL. Same cap and total treatment as
the other lists.

Each list is capped at `officeLoopEvidenceCap`, and the response carries the cap and
the untruncated `total`. The total comes from `COUNT(*) OVER ()` in the **same
statement** as the capped page, not a second query: two statements outside a
transaction can straddle a write, and a list whose own total contradicts it is not
the "internally consistent" AC-004.12 requires. Consistency is deliberately scoped
to one list — the read takes no transaction, so different lists may observe the row
set at different instants; what is forbidden is a list disagreeing with itself.

A trigger with `next_run_at IS NULL` whose claim instant is *inside* the stranded
grace is a **claiming trigger** (AC-004.15), excluded from every evidence list
rather than given a third label: a trigger caught mid-claim is the loop working,
and every tick would otherwise produce one. The stranded predicate's own age test
implements this; there is no separate branch.

The route takes no query parameters (AC-004.14): a caller able to widen the window
or soften a threshold could make any workspace report `healthy`, and a monitor must
not be able to negotiate a verdict. `now` is captured once at the top of the
handler and passed to every predicate, so the trigger, run and terminal reads
cannot disagree about the instant they were evaluated against; it is returned with
the verdict.

### Verdict

Precedence `unknown` → `dead` → `degraded` → `not_armed` → `healthy`, first match
winning (AC-004.2). `dead` and `degraded` outrank `not_armed` because a workspace
with no cron trigger can still have stuck runs from event-driven wakes, and
reporting that as "nothing scheduled" would hide them.

### Failure

Fail loud: any unreadable input returns `503` with the failing input named, and
increments `office_loop_liveness_degraded_total{reason}` over the enumerated set
`activation_read_failed`, `trigger_read_failed`, `run_read_failed`,
`terminal_read_failed`. No partial verdict is ever returned.

This inverts `stall-visibility.md`'s fail-closed rule on purpose: that detector
fails closed because a false alert trains operators to ignore it, whereas here a
partial answer rendering as `healthy` is the exact lie the feature removes, and the
caller is a monitor that can act on a `503`.

## Security

No new trust boundary. Both routes go through the existing Office authorization
guard. Their responses carry routine, trigger, run, causation and session
identifiers plus timestamps and counts — no prompt text, agent output, error body
or cost figure. Cross-workspace values are limited to the process facts named in
[The counter read](#the-counter-read) (AC-003.5).

## Testing

Go tests only; no user-visible surface changes, so no Playwright work.

- `internal/office/repository/sqlite` — monotonic `TouchRoutineLastRun` including
  the equal-instant and older-instant no-ops, the zero-row case, and the migration
  on both dialects; and `UpdateRoutine` leaving `last_run_at` untouched (AC-001.8),
  asserted by touching, then updating from a struct whose `LastRunAt` is stale and
  again from one where it is `nil`, and reading the fresh instant back both times.
- `internal/office/routines` — the fire advances `last_run_at` on the coalesced
  and skipped paths, mints one causation id per fire, and survives a
  `TouchRoutineLastRun` error.
- `internal/office/wakeup` — causation id survives coalesce, skip, promotion, and
  the lost-CAS fresh-run path.
- `internal/office/service` — terminal-shape classification table-driven and
  asserted total over the cross-product of `finished`, `failed`, `cancelled`,
  `timed_out` and an unknown status against every outcome including
  `no_agent_launched` and `NULL`, with a skip outcome carrying a session id landing
  in `unclassified`; session id persisted on both launch paths; a failing session
  write leaves the column at whatever it already held, does not fail the launch,
  and increments `office_loop_session_persist_failed_total` rather than the
  without-session counter; relaunch keeps the greater-`claimed_at` id and an empty
  relaunch does not clear it; the wiring completeness test for the
  session-returning seam.
- `internal/office/dashboard` — verdict precedence per branch, threshold echo,
  ordering and truncation, the silent-success evidence list, a claiming trigger
  appearing in no list, a terminal run with `NULL finished_at` still inside the
  window, a stuck run reported regardless of age, empty-workspace shape, and a
  `503` per degraded reason. For AC-004.17, one case per exclusion, each an
  otherwise-stuck queued run that must **not** be reported and must leave the
  workspace `healthy`: a future `scheduled_retry_at`, a non-`NULL`
  `routing_blocked_status`, and a claimed sibling on the same `agent_profile_id`;
  plus the positive control with no sibling claimed, which **is** reported, and a
  run whose `scheduled_retry_at` has just passed being aged from it rather than
  from `requested_at`. For the ordering, a mixed list asserting a claimed-stuck run
  outranks an older queued-stuck one and a `NULL` `claimed_at` row sorts first. For
  the counter read: workspace-labelled values
  filtered to the requesting workspace with another workspace's key absent, an
  unobserved counter returned as zero rather than omitted, `_unattributed` and the
  process instants present in the `process` block and absent from `workspace`, and
  a malformed key counted into the malformed bucket rather than dropped.

## Related decisions

None. This design reuses the counter convention, the activation-key idiom, the
optional-interface seam, and the workspace scope guard Office already has.
