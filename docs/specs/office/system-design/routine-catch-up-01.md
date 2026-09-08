---
status: draft
system: office
requirements:
  - REQ-OFFICE-ROUTINE-CATCHUP-001
  - REQ-OFFICE-ROUTINE-CATCHUP-002
---

# Office Routine Catch-Up System Design, part 1: the tick and the gap

## Purpose and boundaries

This design makes the routine cron tick's resume behaviour explicit, gives the
gap it crosses a durable record, and delivers that record to the woken agent —
REQ-001 and REQ-002. Renaming the policy value that misdescribes all of it is
REQ-003, in [part 2](routine-catch-up-02.md).

The Office system owns the outcome: the trigger table, the routine-run table,
the wakeup queue, and the routine prompt path are all Office primitives.

Adjacent contracts read and constrained, but not owned:

- `internal/scheduler/cron` — the shared 30-second cron loop that calls
  `RoutineService.TickScheduledTriggers`. Its interval and handler ordering are
  unchanged.
- `internal/office/shared/cron.go` — the cron evaluator. Its tick times are
  taken as given; its day-of-month/day-of-week conjunction, DST behaviour, and
  unsatisfiable-expression fallback are gap 23 and out of scope.
- `internal/office/wakeup` — the wakeup queue, its idempotency index, and the
  dispatcher's concurrency policy. Unchanged except that `RoutinePayload` gains
  two fields alongside the existing `MissedTicks`.

## Requirement mapping

| Requirement | Design section |
|---|---|
| REQ-OFFICE-ROUTINE-CATCHUP-001 | [The tick](#the-tick), [Normalizing catch_up_max](#normalizing-catch_up_max), [Ordering, claiming and re-arming](#ordering-claiming-and-re-arming), [Dispatch failure](#dispatch-failure) |
| REQ-OFFICE-ROUTINE-CATCHUP-002 | [The gap summary](#the-gap-summary), [Delivering the gap to the agent](#delivering-the-gap-to-the-agent) |
| REQ-OFFICE-ROUTINE-CATCHUP-003 | [part 2](routine-catch-up-02.md): Renaming the policy, Documentation consistency |

## Current behaviour, as measured

Recorded because these claims are load-bearing and none is written down today.

- `processCronTrigger` (`internal/office/routines/service.go:360-397`) claims
  the trigger, calls `computeRoutineMissed`, arms `next_run_at`, then calls
  `DispatchRoutineRunWithMissed` **once**, passing `runCount - 1`.
- `computeRoutineMissed` (`:409-447`) walks `NextCronTime` from the armed
  `next_run_at` until the cursor passes `now` or the count reaches
  `routine.CatchUpMax` (default 25 when the stored value is not positive).
  Nothing bounds `CatchUpMax` above: `routines/handler.go` assigns it from the
  request unvalidated on both create and update, and the walk is one synchronous
  `NextCronTime` call per counted tick inside the 30-second scheduler tick.
- The count reaches the wakeup request's payload as `missed_ticks`
  (`marshalRoutinePayload`, `:665`). `createFreshRun` copies that payload onto
  `runs.context_snapshot` **verbatim** (`wakeup/dispatcher.go:291`); the
  coalesce paths do not, and `json_patch` it in instead, at **two separate SQL
  sites** that do not share an implementation — see
  [Delivering the gap to the agent](#delivering-the-gap-to-the-agent).
- **It stops there.** `buildPromptContext` is called with `run.Payload`
  (`service/scheduler_integration.go:375`), and `createFreshRun` sets
  `Payload: "{}"` on every wakeup-derived run. `prompt_builder.go` contains no
  reader for `ContextSnapshot`. No web view renders `context_snapshot` either —
  it is typed in `apps/web/lib/api/domains/office-runs-api.ts:113` and never
  read. `missed_ticks` is written and never observed, by an agent or a human.

Two further measurements shape the design:

- `GetDueTriggers` (`repository/sqlite/routines.go:65-79`) has no `ORDER BY`.
- `ClaimTrigger` (`:84-96`) sets `next_run_at = NULL` as its CAS and stamps
  `updated_at`; `UpdateTriggerNextRun` re-arms in a separate, unconditional
  statement. A process that stops between the two leaves an enabled cron trigger
  with a null `next_run_at`, which `GetDueTriggers` filters out — permanently.
  That `updated_at` stamp is what lets the reconciliation pass tell a live claim
  from an abandoned one.
- `models.RoutineRunStatusFailed` is declared in `models/enums.go` and set
  nowhere in the tree.

## Why the fan-out was rejected

The requirement records the decision; this is the mechanism behind each of its
four grounds, verified against the tree.

1. **It is not implementable without changing a different contract.** The wakeup
   idempotency key is `routine:<routine_id>:<trigger_id>:<unix_minute>`
   (`buildRoutineIdempotencyKey`) and `agent_wakeup_requests.idempotency_key`
   carries a partial unique index. N requests created inside one tick share one
   wall-clock minute, so the second through Nth are rejected as idempotency
   conflicts and absorbed as a warning. A naive fan-out silently produces one
   wake anyway. Re-keying on scheduled tick time is the generation-identity work
   tracked separately as gap 26.
2. **Whichever concurrency policy is in force would collapse them regardless.**
   The shipped default is `skip_if_active` — both `routines/handler.go`'s create
   path and the `office_routines.concurrency_policy` column default name it, and
   it drops a request while a run is in flight. `coalesce_if_active`, which only
   the pre-installed coordinator routine sets, merges the request into the
   in-flight run instead. Either way N requests become one wake. A fan-out is
   observable only under the third policy, `always_create` — that is the
   routine-layer name an operator sets; the wakeup layer calls the same policy
   `always_enqueue` after `dispatcher.go` maps it.
3. **Under `always_create` it would fan out into a system with no
   work-in-progress limit and a budget check that fails open.**
   `max_concurrent_sessions` is persisted and has no scheduler consumer;
   `checkBudget` returns allow on checker error. Both are separately carded and
   unshipped. Building the fan-out first builds the hazard before the two
   controls that would bound it.
4. **There is no per-tick work to replay.** A lightweight routine's payload is
   the routine id plus variables, and the time-valued built-ins (`{{date}}`,
   `{{datetime}}`) are resolved at dispatch time, not at tick time. A replayed
   missed tick would carry today's values and be indistinguishable from the
   current one.

## The tick

`computeRoutineMissed` is replaced by a function returning one value object
instead of `(int, time.Time, error)`, so the caller cannot discard a computed
re-arm time on the error path the way it does today:

```go
type catchUpResult struct {
    ElapsedTicks  int       // >= 1 on success; includes the tick due now
    FirstMissedAt time.Time // zero when ElapsedTicks <= 1
    Truncated     bool      // ElapsedTicks reached the cap with ticks still pending
    NextRunAt     time.Time // always strictly after the processing instant
    Unknown       bool      // the walk failed; ElapsedTicks is not meaningful
}
```

Invariants the constructor guarantees, so no call site has to restate them:

- `NextRunAt` is strictly after the processing instant on **every** path,
  including `Unknown`. It is never `now`. This closes the current hot loop:
  today the failure path arms `next_run_at` to `now`, which is immediately due
  again on the next 30-second tick, forever.

  On the failure path it is `now + catchUpFallbackInterval`, and
  **`catchUpFallbackInterval` is 24 hours** (AC-001.11). The value is
  load-bearing, not a defensive default, because the failure is not transient:
  `NextCronTime` returns an error only for a cron expression that is not five
  fields, a spec that does not parse, or a timezone `time.LoadLocation` rejects.
  All three are deterministic in the stored `(expression, timezone)` pair and
  not in the time argument, so retrying with a later `now` cannot succeed.
  (`findNextMatch` never errors; an unsatisfiable expression silently returns
  "24 hours from now", which is gap 23 and out of scope.) A trigger that lands
  on this path stays on it until it is deleted and recreated, so the interval
  *is* its permanent dispatch cadence. 24 hours caps that at one run per day and
  matches `findNextMatch`'s own give-up interval, so the subsystem's two
  fallbacks agree. Retrying at the 30-second scheduler interval would dispatch
  roughly 2,880 runs a day against a trigger that can never succeed — the same
  unbounded-spend defect this capability exists to close.

  The failure path also emits a `Warn` naming the trigger id and the underlying
  error, which is the only operator-visible signal that a trigger is stuck; no
  new metric is introduced for it.
- `FirstMissedAt` is the armed `next_run_at` at claim time — read directly, not
  derived from the walk, so it is exact even when `Truncated` is true.
- `Truncated` is true only when the walk stopped at the cap with the cursor
  still at or before the processing instant.

`missedTicks = ElapsedTicks - 1`, floored at zero. The catch-up policy decides
whether that number is *reported*, never whether runs are *created*: the tick
dispatches exactly one run either way. `skip_missed` and `summarize_missed`
therefore differ in exactly one observable: whether the woken agent is told
about the gap.

**A gap summary exists only when `missedTicks >= 1`.** This single rule collapses
what would otherwise be four near-identical states into two, and it is why
`Unknown` needs no representation in storage:

- `Unknown` (the walk failed) drives the re-arm and nothing else. No summary is
  written, because a due trigger has at least one elapsed tick but may have no
  *missed* one, and reporting an unmeasured gap on a merely-late trigger is a
  false positive (AC-001.6).
- `catch_up_max == 1` makes `missedTicks` structurally zero, so that
  configuration never produces a summary. That is a named consequence, not an
  edge case to detect (AC-002.11).
- A truncated count is always at least `catch_up_max - 1`, so truncation and a
  zero count cannot co-occur for any `catch_up_max` above 1.

## Normalizing catch_up_max

AC-001.4 clamps `catch_up_max` at **use** time, which is the fail-open safety
net: no stored value, however it got there, can stop a routine firing. On its
own that reintroduces the defect this whole capability exists to close, one
field over. A stored 5000 would keep being returned by `GetRoutine` /
`ListRoutines`, rendered in the routine-detail form, and edited there, while the
tick honoured 1000 — an operator sizing spend from the field would be wrong by
a factor of five, which is the Overview's disagreement table with a different
number in it.

Note the divergence is not created by the upper bound; it exists today on the
lower one. `CreateRoutine` / `CreateRoutineTx` clamp `<= 0` to 25 at write time,
but `UpdateRoutine` (`repository/sqlite/routines.go:200`) writes the field with
no bound at all and `routines/handler.go:297` assigns it unvalidated, so a
stored 0 is reachable now and already reads back as 0 while the tick uses 25.
AC-001.4's upper bound extends an existing hole rather than digging a new one,
which is why AC-001.13 closes both ends at once.

`CatchUpMax` is a plain `int` (`models/models.go:548`), not a named type, so the
`sql.Scanner` hook [part 2](routine-catch-up-02.md) uses for
`catch_up_policy` has nothing to attach to. Changing its type to get one would
ripple through the DTO, the JSON contract and the web state for no behavioural
gain — the same reasoning that keeps `## Out of scope`'s "Renaming
`catch_up_max`" excluded.

**Normalize on write instead, which the policy field cannot do and this one
can — through one named funnel, applied at the repository.**
`models.NormaliseCatchUpMax(int) int` is the `catch_up_max` sibling of part 2's
`NormaliseCatchUpPolicy`. It applies AC-001.4's clamp and is the only place the
two bounds exist in Go, as named constants beside it. Today neither bound has a
single point of change: `defaultCatchUpMax = 25` lives only in
`routines/service.go:401` and no `1000` appears anywhere.

**It is applied in the repository, not in the handlers, because the repository
is the only exhaustive choke point.** `catch_up_max` reaches SQL at exactly two
statements: `insertRoutine` (`repository/sqlite/routines.go:150`), the shared
INSERT that both `CreateRoutine` and `CreateRoutineTx` call, and `UpdateRoutine`
(:200). Config-sync's own update path, `updateRoutineConfigFields` (:243), does
not write the column at all. Normalizing in those two functions covers every
present and future writer by construction.

Clamping in `routines/handler.go` instead would not, and the enumeration is the
whole point of this section: **two independent service layers reach the same
repository methods without passing through that handler** —
`office/service.Service.CreateRoutine` and `UpdateRoutine` (`service/service.go`
:713 and :732), the second of which `config_import.go`'s `applyRoutines` (:259)
calls for every imported routine. That is the same bypass shape that lost
`catch_up_policy` to `CreateRoutineTx`, and the same reasoning part 2 uses to put
the policy's read-side normalization in `sql.Scanner` rather than in each query.
The existing `<= 0 → 25` clamps in `CreateRoutine` and `CreateRoutineTx` are
removed in favour of the funnel call in `insertRoutine`, so the lower bound stops
being stated twice as well.

Existing rows are corrected by one `r.migrate.Apply` step alongside the policy
migration. This SQL is the one place the bounds are restated outside the funnel,
so it carries the same two values by construction:

```sql
UPDATE office_routines SET catch_up_max = 25   WHERE catch_up_max < 1;
UPDATE office_routines SET catch_up_max = 1000 WHERE catch_up_max > 1000;
```

The boundary is exclusive on both ends and the SQL says so: `< 1` and `> 1000`,
so a stored 1 and a stored 1000 are both left alone, matching AC-001.4's "less
than 1" and "greater than 1000" exactly. There is no null case to consider —
the column is `catch_up_max INTEGER NOT NULL DEFAULT 25` (`base.go:424`), so
every row matches one predicate or neither, and a `NULL` cannot silently escape
a comparison. Both statements are idempotent, and their order relative to the
`office_routines` table rebuild in [part 2](routine-catch-up-02.md) does not
matter: the rebuild copies column values verbatim, so normalizing before or
after it yields the same rows.

Four consequences of clamping at the write, stated because a builder would
otherwise have to derive them. **The funnel reassigns `routine.CatchUpMax` on the
passed-in struct, before the statement binds that field**, rather than
normalizing the bind parameter alone — mutating
in place as the `CreateRoutine` clamp it replaces already does
(`routines.go:125-127`), with `UpdateRoutine` gaining the same behaviour.
Load-bearing, not stylistic: `routines/handler.go` serializes the SAME `*Routine`
it handed the service on both paths (`:95` create, `:136` update), so a bind-only
clamp would store 1000 while that one response reported the submitted 5000 —
AC-001.13's own divergence, reopened for one round-trip.
`NormaliseCatchUpMax` is a **fixpoint** —
normalizing an already-normalized value returns it unchanged — so applying it
twice is safe and a belt-and-braces call in a handler cannot corrupt anything.
Two concurrent updates of one routine row stay last-write-wins, unchanged by this
design, and both writers store a normalized value, so the row is in range
whichever lands second. And because the clamp sits on what the repository
*writes* rather than on what the request *carries*, an update that omits
`catch_up_max` altogether — it is `*int` with `omitempty` (`routines/dto.go:24`),
so omitting it means "leave it alone" — still rewrites the stored value
normalized. An out-of-range row is therefore corrected by the next edit routed
through `Repository.UpdateRoutine`, not only by the migration. It is **not**
corrected by config-sync's column-scoped update (`config/import.go`,
`configsync/reconcile_routines.go`), which does not write the column at all, so a
routine only ever touched that way waits for the migration.

Write-side normalization plus a migration makes AC-001.4's use-time clamp
belt-and-braces rather than load-bearing, exactly as the `sql.Scanner` does for
the policy: a row that somehow escapes both still ticks correctly, it just also
now reads correctly. AC-001.4 is unchanged and still governs the tick.

## Ordering, claiming and re-arming

- `GetDueTriggers` gains `ORDER BY next_run_at ASC, id ASC`. `id` is the named
  tiebreak column; it is a primary key, so the order is total and stable.
- Claiming is unchanged. The CAS on `next_run_at` already gives exactly one
  winner; the loser returns before computing or recording anything.
- The claim-then-arm window is closed by a reconciliation pass in
  `TickScheduledTriggers`: enabled cron triggers with a null `next_run_at` are
  armed to the first match strictly after the processing instant and are **not**
  dispatched in that tick. Re-arming rather than firing is deliberate: the row
  cannot distinguish "crashed after claiming" from "crashed after claiming and
  dispatching", and a spurious wake is the more expensive error.
- **The pass runs after the due-trigger loop, and only on stale rows.** A null
  `next_run_at` is precisely what `ClaimTrigger` writes to take a claim, so a
  live claim and an abandoned one are identical in the row. Without a second
  predicate the pass would arm a trigger whose claimant is still mid-tick.
  `ClaimTrigger` also sets `updated_at`, which is the discriminator: the pass
  considers only rows whose `updated_at` is older than `catchUpReclaimAfter`,
  **60 seconds** — two scheduler intervals, so a claim taken in this tick or the
  one before is never reconciled, while an abandoned one is recovered on the
  following tick. Running after the due-trigger loop means a trigger claimed and
  armed within this same tick is already non-null and never reaches the pass.
  This is what AC-001.9's "none shall dispatch **by way of this path**" scopes:
  a live claimant's own dispatch is legitimate and unaffected.
- That reconciliation write is also a CAS. The `UPDATE` carries `WHERE id = ?
  AND next_run_at IS NULL AND enabled = 1`, so a second reconciling processor
  affects zero rows and returns, and a trigger disabled or armed between the
  select and the write is left alone. Without the predicate two processors would
  each compute their own `NextCronTime(expr, tz, now)` from slightly different
  instants and the later write would silently move a trigger another had already
  armed. Note what the CAS does **not** buy: it cannot protect against a live
  claimant, whose own re-arm through `UpdateTriggerNextRun` is an unconditional
  write that wins regardless. The staleness guard is what keeps the two paths
  disjoint; the CAS only orders reconcilers among themselves.
  `UpdateTriggerNextRun` stays unconditional and the reconciliation path needs a
  conditional sibling rather than reusing it.
- Arming on create uses `NextCronTime(expr, tz, now)` and rejects the trigger
  when that call errors (`CreateRoutineTrigger`, `service.go:305-314`).
  **AC-001.8's invariant is enforced in the service: `CreateRoutineTrigger` is
  the only place that may arm a cron trigger.** Today's guard is
  `if t.Kind == "cron" && t.CronExpression != ""`, which lets an empty
  expression fall through and persist a cron trigger with a null `next_run_at`;
  dropping that second condition and rejecting the empty case is the change
  AC-001.8 asks for. One caller reaches the repository directly —
  `infra/reconcile.go`'s `createTriggersForNewRoutines` calls
  `repo.CreateRoutineTrigger` past the service. It is named here because this
  design already lost `catch_up_policy` to exactly this shape of bypass
  (config-sync reaching `CreateRoutineTx`). It writes `Kind: "manual"` only, so
  it cannot violate the cron invariant and needs no change; the rule it inherits
  is that any future path creating a `cron` trigger goes through the service,
  not the repository. **There is no trigger-update path to align with it.** The
  repository interface exposes `CreateRoutineTrigger`, `UpdateTriggerNextRun`
  (the scheduler's own re-arm, `next_run_at` only) and `DeleteRoutineTrigger`;
  the HTTP surface is `GET`/`POST /routines/:id/triggers` and
  `DELETE /routine-triggers/:triggerId` plus the webhook fire; and the web
  client has only `deleteRoutineTrigger`. Changing a cron expression, timezone
  or enabled flag is therefore a delete followed by a create, which arms from
  the create instant and satisfies AC-001.8 with no new code. The update
  endpoint is excluded in the requirement's `## Out of scope`, so this design
  creates no trigger-mutation surface.

## Dispatch failure

AC-001.10 and AC-001.12 have no implementation to point at today:
`models.RoutineRunStatusFailed` is declared in `models/enums.go` and **set
nowhere in the tree**. Both dispatch paths lose their failures:

- Heavy (`materialiseHeavyRoutineRun`) returns an error when
  `EnsureRoutineWorkflow` or `CreateOfficeTaskInWorkflow` fails, without
  touching run status, so the run sits at `received` forever.
- Lightweight (`materialiseLightweightRoutineRun`) logs a `Warn` and returns
  `nil` for both `CreateWakeupRequest` and `Dispatch` failures, so the failure
  never reaches `processCronTrigger` as an error at all.

Three changes, in the order the tick executes:

1. **`CreateRoutineRun` fails.** `dispatchRoutineRun` already returns
   `(nil, err)` here, before any run row exists. There is nothing to mark and
   nothing to re-attempt; the trigger was armed forward at the previous step, so
   the tick is simply lost. AC-001.12 makes that the stated contract rather than
   an accident. The caller logs and returns.
2. **The lightweight path stops swallowing.** `CreateWakeupRequest` and
   `Dispatch` errors are returned rather than absorbed, with one exception: the
   sentinel the office repository returns for an idempotency-key conflict stays
   a no-op and is still absorbed as a `Warn`. That distinction is the whole
   point of AC-001.10's second sentence — a duplicate is a successful dedup, not
   a failed dispatch, and marking it `failed` would turn correct coalescing into
   a red run in the UI.
3. **The status flip has one home.** `processCronTrigger` owns it: when
   `DispatchRoutineRunWithMissed` returns a non-nil run together with an error,
   it calls `UpdateRunStatus(run.ID, RoutineRunStatusFailed, "")`. Putting it in
   the caller rather than in each materialise path means the heavy and
   lightweight branches need no duplicate error handling, and the run is marked
   exactly once whichever branch failed.

The gap summary is untouched by any of this — it was written with the run row
(see below), so a run that later goes `failed` still carries the gap that was
measured for its tick.

## The gap summary

Stored on `office_routine_runs`, which both the lightweight and the heavy path
create — one durable surface for both routine shapes. Three columns, added via
`r.migrate.Apply` in `repository/sqlite/base_migrations.go` and to the
`CREATE TABLE` in `base.go:448`:

| Column | Type | Meaning |
|---|---|---|
| `catch_up_missed_ticks` | `INTEGER` nullable | missed ticks; **NULL means no gap summary** |
| `catch_up_first_missed_at` | `TIMESTAMP` nullable | exact timestamp of the first missed tick |
| `catch_up_truncated` | `INTEGER NOT NULL DEFAULT 0` | 1 when the count is a lower bound |

Nullable rather than `NOT NULL DEFAULT 0` because AC-002.3 requires absence to
be distinguishable from a measured zero. Per the rule above, a NULL
`catch_up_missed_ticks` always implies `catch_up_first_missed_at` is NULL and
`catch_up_truncated` is 0; the three columns are written together or not at all.

Explicit columns rather than folding into `trigger_payload`: that column is the
resolved-variables contract, and a queryable "which routines resumed after a
gap" is the operability question gap 21 will need.

The columns are written once, in the same repository call that creates the
routine run, before the concurrency policy is applied. A run that is
subsequently marked `skipped`, `coalesced` or `failed` therefore still carries
the gap that was measured — the gap is a property of the tick, not of the
dispatch outcome.

That write-once property, not any claim about deduplication, is what satisfies
AC-002.10, and the ordering is worth stating because the obvious reading is
wrong: `dispatchRoutineRun` calls `CreateRoutineRun` **unconditionally and
first**, before `applyConcurrencyPolicy` and long before
`materialiseLightweightRoutineRun` inserts the wakeup request carrying the
idempotency key. `CreateRoutineRun` is neither gated by nor aware of the
`agent_wakeup_requests` unique index, so a wakeup rejected as a duplicate does
**not** prevent a second `office_routine_runs` row — it only prevents a second
wake. Two rows, each with its own measured summary, is the correct outcome: no
row is ever rewritten, because nothing writes these columns after the insert.

For a cron claim two rows are hard to reach anyway — `ClaimTrigger` is a CAS on
`next_run_at`, the trigger is always re-armed strictly forward, and the
idempotency key is per *trigger* and per wall-clock minute. The reachable
multi-run cases are two triggers on one routine due in the same tick, and a
manual or webhook fire alongside a cron fire; AC-002.12 gives the latter no gap
summary at all. Note that AC-002.10 constrains the agent-run
`runs.context_snapshot` as well as these rows — see
[Delivering the gap to the agent](#delivering-the-gap-to-the-agent), where the
coalesce merge is the surface that can actually rewrite a delivered gap.

They surface on the routine-run API DTO and are the source for any future UI.

## Delivering the gap to the agent

Three hops, of which only the last is new.

1. `marshalRoutinePayload` already carries `missed_ticks`; it gains
   `missed_since` (RFC 3339) and `missed_truncated`. `wakeup.RoutinePayload`
   gains the matching fields. Both remain `omitempty`, so a gapless fire has a
   payload byte-identical to today's.
2. **One payload column feeds two destinations, and only one of them may
   carry the gap.** `createFreshRun` sets `ContextSnapshot: req.Payload`
   verbatim (`wakeup/dispatcher.go:291`) and is unchanged — that is the path
   AC-002.5 needs the three fields on. The **coalesce** paths merge the same
   persisted `agent_wakeup_requests.payload` row into an existing run's
   snapshot with `json_patch`, RFC 7386 merge-patch, so every top-level key the
   incoming payload carries **overwrites** the one already there. Left alone
   that gives two wrong outcomes: a second gap-carrying routine wake replaces
   the first claim's measurement with its own, and because the three fields are
   `omitempty`, a *gapless* later wake does not clear them, so the run keeps a
   gap belonging to a tick it is not answering.

   **There are TWO merge sites, not one, and they do not share an
   implementation.** `MarkWakeupRequestCoalesced`
   (`repository/sqlite/wakeup_requests.go:168`) calls
   `mergeWakeupPayloadIntoRunSnapshot` (:256, whose `json_patch` is at :265).
   `PromoteRunAndCoalesceWakeupIfQueued` (:193) does **not** call that helper:
   it carries its own inline copy of the identical `json_patch` statement at
   :229-238, executed on `tx` rather than `r.db` because it runs inside the
   promote transaction. `grep -n json_patch` over that file returns THREE lines:
   these two SQL sites, plus the helper's own doc comment, which names the
   function in prose and survives the unification.
   Naming only the helper would send a builder to patch one of them, and the
   one left unpatched is the promoted-run path — the same path
   [part 2](routine-catch-up-02.md)'s test plan already singles out as the one
   a naive implementation misses for the prompt gate.

   **The strip cannot live at the routine layer.** The routine layer writes the
   payload once, at wakeup-request creation, and `agent_wakeup_requests` has a
   single `payload` column. At that moment it is not yet known whether the
   request will become a fresh run or be coalesced — the dispatcher decides
   later, and both destinations read the same stored row. Stripping at
   creation would strip the fresh-run path too, which AC-002.5 forbids; not
   stripping leaves both merge sites carrying the keys. So the placement this
   design previously asserted is not implementable, and the decision is
   corrected here.

   **The strip happens at the merge, in exactly one place.** The two merge
   sites are unified: the `json_patch` statement moves into a single helper
   taking an executor (`sqlx.ExtContext`, satisfied by both `r.db` and `tx`),
   and `PromoteRunAndCoalesceWakeupIfQueued` calls it inside its transaction
   instead of duplicating the SQL. That refactor is what makes "one merge path"
   a true statement rather than the false one this design shipped. The helper
   patches `json_remove(payload, '$.missed_ticks', '$.missed_since',
   '$.missed_truncated')` rather than `payload`.

   Putting three catch-up key names in an otherwise generic helper is the cost,
   and it is the right trade against the alternative of a second payload column
   read only by `createFreshRun`: `json_remove` of an absent key is a no-op in
   SQLite (verified: `json_remove('{"a":1}', '$.missed_ticks')` returns
   `{"a":1}`), so the helper stays byte-for-byte correct for every other wakeup
   source, none of which writes these keys. A named constant holds the key list
   beside the helper so the two never drift.

   The decision itself is unchanged: the three keys are written once, by the
   wake that created the run, and no coalesced request adds, changes or removes
   them (AC-002.10). Nothing is lost, because each claim's gap is already
   durable on its own `office_routine_runs` row and readable through the
   routine-run API (AC-002.6, AC-002.8). What the agent gets is one coherent
   statement about the tick its run was created for, rather than a silent
   mixture of two claims' measurements.
3. `buildPromptContext` is called with `run.Payload`, which is `"{}"` for every
   wakeup-derived run. The call site at `scheduler_integration.go:375` passes
   `run.ContextSnapshot` as an additional argument, and `buildPromptContext`
   decodes it as `wakeup.RoutinePayload` when the run's reason is a routine
   dispatch. **The constant to test is not `RunReasonRoutine`, which does not
   exist.** `shared/runreasons.go` declares three:
   `RunReasonRoutineDispatchCron`, `RunReasonRoutineDispatchEvent`, and the
   legacy `RunReasonRoutineDispatch`. The decode gates on **all three**, not on
   Cron alone. Cron alone is the tempting choice, since only a cron claim ever
   measures a gap (AC-002.12), and it is wrong:
   `PromoteRunAndCoalesceWakeupIfQueued` rewrites an in-flight run's `reason`,
   so a cron-created run carrying a correctly measured gap can be holding the
   Event reason by the time its prompt is built, and a Cron-only gate would drop
   that gap silently. Gating on all three is safe in the other direction,
   because a manual or webhook claim never writes the fields at all, so an
   Event-reason run has nothing to render. `PromptContext` gains
   `MissedTicks int`, `MissedSince string`, `MissedTruncated bool`, and
   `BuildPrompt` renders a wake-context line when `MissedTicks > 0`.

Scoping the decode to the routine reasons keeps every other run's prompt
byte-identical, which is what makes this change safe to land without a prompt
regression sweep across all reasons.

The heavy path does not deliver the gap to the prompt: it creates a task from
the routine's template, and mutating a user's template text with scheduler
metadata is a worse contract than leaving the gap on the routine run where the
API exposes it (AC-002.6).

## Renaming the policy, documentation and tests

REQ-003's rename, the corrections to `scheduler-01.md` / `scheduler-02.md`, and
the test plan for both parts live in
[part 2](routine-catch-up-02.md). They are a separate deliverable: this part
changes what the system does on resume, part 2 changes what every surface says
about it.
