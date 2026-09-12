---
status: current
system: office
requirements:
  - REQ-OFFICE-ROUTINE-STATUS-001
  - REQ-OFFICE-ROUTINE-STATUS-002
  - REQ-OFFICE-ROUTINE-STATUS-003
  - REQ-OFFICE-ROUTINE-STATUS-004
  - REQ-OFFICE-ROUTINE-STATUS-006
---

# Office Routine Status Gating System Design

## Purpose and boundaries

Office owns `office_routines.status`, the `office_routine_triggers.next_run_at`
cursor, and all three routine fire paths. This design makes status the single
gate those paths consult, and defines what a suppressed slot does to the cursor.

Adjacent contracts this design uses but does not own:

- The shared cron loop (`internal/scheduler/cron`) drives `TickScheduledTriggers`
  every `DefaultTickInterval` (30s). It owns cadence, not routine policy.
- The wakeup dispatcher (`internal/office/wakeup`) turns a routine wakeup request
  into a run. It reads the routine only for its concurrency policy and gains no
  status responsibility here.
- Outage catch-up (`computeRoutineMissed`) is owned by the routine catch-up
  capability. This design calls it only on the firing path.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-ROUTINE-STATUS-001` | [The firing-status predicate](#the-firing-status-predicate) |
| `REQ-OFFICE-ROUTINE-STATUS-002` | [Cursor advance on suppression](#cursor-advance-on-suppression) |
| `REQ-OFFICE-ROUTINE-STATUS-003` | [Control flow](#control-flow) |
| `REQ-OFFICE-ROUTINE-STATUS-004` | [Manual and webhook fires](#manual-and-webhook-fires) |
| `REQ-OFFICE-ROUTINE-STATUS-006` | [Frontend surfaces](#frontend-surfaces) |

## Components and responsibilities

- **`office/models`** owns the firing-status predicate and the status constants it
  compares against, alongside the existing `RoutineConcurrencyPolicy` and
  `RoutineCatchUpPolicy` types in `enums.go`, so no caller re-derives the rule from
  string literals.
- **`office/shared`** owns cron expression evaluation, and gains one exported
  predicate for checking that a computed time is a time an expression actually names.
  See [the match check](#the-match-check) for why the check cannot be expressed with
  the package's existing exported surface, and why it is added here rather than in
  `office/routines`.
- **`office/routines` service** owns the gate on both the cron path and the manual
  path. `processCronTrigger` consults the predicate before claiming; `FireManual`
  consults it on the routine it already reads and returns a refusal its handler can
  distinguish. `DispatchRoutineRun` and `DispatchRoutineRunWithMissed` are unchanged.
- **`office/routines` handler** owns the HTTP status codes and response bodies for
  the two refusals, and owns the webhook gate, which sits on the routine that handler
  already holds. Create and update take no new validation.
- **`office/repository/sqlite`** owns the compare-and-set that advances a cursor
  without recording a fire.
- **`apps/web` routines list** owns the display of a non-firing routine.

## Data and contracts

### Routine status

`office_routines.status` is `TEXT NOT NULL DEFAULT 'active'` with no check
constraint. This design adds no migration and no write-side validation: the stored
value set is unchanged, and the only new rule is how the fire paths read it.

```
RoutineStatusActive   = "active"
RoutineStatusPaused   = "paused"
RoutineStatusArchived = "archived"
```

`CanFire()` accepts `active` and the empty string and rejects everything else,
including values that are not one of the three constants at all. It compares the
stored bytes with no case folding and no trimming, so `Active` and `" active"` are
non-firing (AC-OFFICE-ROUTINE-STATUS-001.3). There is deliberately no `Valid()`
counterpart: nothing in this capability validates a status on write, so a predicate
that guards writes would have no caller. Adding one is named out of scope in the
requirements, together with the three problems it drags in.

### API

Routes are mounted on `officeRoutePrefix = "/api/v1/office"`
(`backendapp/office_scope.go`), so every path below carries that prefix.

- `POST /api/v1/office/routines/:id/run` returns 409 when the routine cannot fire.
- `POST /api/v1/office/routine-triggers/:publicId/fire` returns 409 the same way,
  evaluated after `verifySignature` and after the existing `trigger.Enabled` check.

Both refusals carry the same body shape, and the shape is the contract, not just the
prose in it (AC-OFFICE-ROUTINE-STATUS-004.4):

```
{ "error": "<human-readable sentence naming the status>",
  "error_code": "routine_not_firing",
  "status": "<the observed status, verbatim>" }
```

`error_code` is what makes the refusal discriminable. The existing `error` field is
already promoted into `err.message` by `lib/api/client.ts` `throwFromResponse`, and
both "Run now" handlers toast that message verbatim, so without a code the operator
would read a Go-side English string — which cannot pass through the i18next pipeline
AC-OFFICE-ROUTINE-STATUS-006.6 requires, and which the surface cannot tell apart from
any other failure. On the manual route that means a 500, which is all `runRoutine`
returns today; on the webhook route it would additionally collide with the
`trigger is disabled` 409 that route already returns. The
repository already has this mechanism and this convention: `ApiError` parses a body
`error_code` into `.errorCode`, and `lib/api/task-delete-errors.ts` and
`lib/api/domains/canvas-error-copy.ts` are the two existing consumers. `status` is
reported verbatim so the surface can interpolate it into localized copy rather than
parse it back out of a sentence. The `error` string remains a fallback for any caller
with no localized copy, which is why it is not itself subject to -006.6.

### Repository

One new method advances a cursor without recording a fire:

```
AdvanceTriggerWithoutFiring(ctx, triggerID string, oldNextRunAt, newNextRunAt time.Time) (bool, error)
```

It is a single statement, `UPDATE office_routine_triggers SET next_run_at = ?,
updated_at = ? WHERE id = ? AND next_run_at = ?`, returning whether a row
changed. It exists because `ClaimTrigger` cannot be reused: that statement sets
`last_fired_at = now` and clears the cursor, both of which are evidence of a fire
that did not happen (AC-OFFICE-ROUTINE-STATUS-001.7). The `next_run_at` predicate
is the same compare-and-set, so it carries the same concurrency guarantee: the
loser of a race changes no rows and does nothing
(AC-OFFICE-ROUTINE-STATUS-002.6, -002.7).

## Control flow

`processCronTrigger` reorders into: read routine, decide, then act.

1. Return early when the trigger has no cursor. Unchanged.
2. **Read the routine.** A read failure or a missing routine returns without
   touching the cursor, so the trigger stays due and the next tick retries
   (AC-OFFICE-ROUTINE-STATUS-003.2, -003.3). This is the read that today happens
   after the claim, where a failure leaves the cursor cleared and the trigger
   permanently dead.
3. **Evaluate `CanFire()`.** When it is false, compute the first slot strictly
   after `now` from the trigger's cron expression and timezone, check it against
   [the match check](#the-match-check), call `AdvanceTriggerWithoutFiring` with the
   observed cursor as the compare value, log routine id, trigger id, and observed
   status, and return. No claim, no run row, no wakeup, no task.
4. **Claim the trigger.** Unchanged, and only reached on the firing path.
5. **Compute outage catch-up and dispatch.** Unchanged.

`shared.NextCronTime` returns the first match strictly after its argument
(`findNextMatch` starts at `after` truncated to the minute plus one minute), so
step 3 needs no additional guard against re-selecting the slot it is suppressing.

### The match check

Step 3 needs one guard in the other direction. `findNextMatch` searches 366 days
and, finding nothing, returns `after + 24h` with **no error** rather than reporting
that the expression names no slot. An expression like `0 0 30 2 *` therefore yields a
cursor the schedule never names, and the trigger would drift forward a day at a time
forever, each hop looking like a legitimate advance. So step 3 checks that the
computed time actually matches the trigger's expression before writing it, and routes
a mismatch to the failure path below (AC-OFFICE-ROUTINE-STATUS-002.1, -002.8): cursor
untouched, nothing fired, suppressed again next tick.

That check has no callable implementation today, which is why
[Components](#components-and-responsibilities) assigns it an owner rather than leaving
it to the builder. `office/shared/cron.go` exports exactly one symbol, `NextCronTime`;
`cronSpec`, `parseCronSpec` and `matchesSpec` are all unexported, so `office/routines`
cannot perform the check at all with the package's current surface. Three approaches
exist and they are not equivalent:

- **Export a match predicate from `office/shared`.** Chosen. It adds one exported
  function next to the parser it depends on, and changes no existing behavior.
- **Change `NextCronTime` to return an error on the fallback.** Rejected. It is the
  cleanest expression of the underlying bug, but `computeRoutineMissed` on the firing
  path calls the same helper, so changing its error contract changes outage-catch-up
  behavior, which belongs to a different capability. That path is deliberately left
  alone here.
- **Re-implement cron matching inside `office/routines`.** Rejected: two parsers for
  one expression syntax, which will drift.

The new predicate takes an expression, a timezone and a time, and reports whether that
time is one the expression names, returning the same class of error `NextCronTime`
already returns for a malformed expression or an unloadable timezone. It parses the
expression rather than consulting any cached state, so it stays correct if the parser
gains syntax later.

**The comparison is performed in the trigger's timezone.** The predicate loads that
location and converts the given instant into it before comparing any field. This is the
one detail the check cannot leave to its caller: a cron expression names wall-clock
values in its own zone, but `NextCronTime` returns its result in UTC, so a predicate
that compared the instant exactly as handed to it would read UTC wall-clock fields
against a spec written in another zone. For `0 9 * * *` in `America/New_York` that is
hour 13 against a spec naming hour 9 — no match, for a cursor that is perfectly correct.
Every non-UTC routine would then fail the check on every suppressed slot, route to the
failure path below, and sit with a frozen cursor and a permanently due trigger. The
check is only meaningful against the same clock the expression is written in.

### The firing-status predicate

`CanFire()` is an allowlist. `active` and `""` fire; `paused`, `archived`, and
any unrecognized value do not. The empty string is admitted because the column
default is `active`, so an empty stored value means no writer set one. Reading it
as non-firing would stop routines that fire today, and this design never changes
behavior for a routine nobody paused.

### Cursor advance on suppression

Advancing to the first slot strictly after `now`, rather than by one slot from
the old cursor, is what makes AC-OFFICE-ROUTINE-STATUS-002.4 hold. If the backend
was down for part of a suppression window, the trigger comes back due with a backlog;
advancing from `now` collapses that backlog in one evaluation instead of walking it. A
suppression window therefore costs one evaluation per scheduled slot while it lasts,
and contributes nothing to any later catch-up computation, because
`computeRoutineMissed` reads a cursor that was never left in the past.

This holds only once a suppressing evaluation has run, which is why
AC-OFFICE-ROUTINE-STATUS-002.3 and -002.4 are both conditioned on one. `cron.Loop`
uses a ticker and does not tick at startup, so after a restart there is a window of at
least `DefaultTickInterval` in which a routine paused before the restart still holds
its old cursor. A resume inside that window reaches step 5 with that cursor intact and
reports the missed ticks the pause accumulated, on that one fire. Repairing it needs a
mechanism attached to the status write itself, which is named out of scope in the
requirements along with the three constraints such a repair has to satisfy.

The alternative, clearing the cursor on suppression and re-arming it on resume,
was rejected for the same reason it is rejected in that out-of-scope entry: it makes
correctness depend on a resume hook, so any writer that sets `status = 'active'`
without going through it, config sync, a direct database edit, a future bulk
operation, leaves a routine that never fires again. Advancing rather than clearing
means the worst case is a stale count on one fire, never a dead schedule. Nothing
about *stopping* a routine depends on any of this, which is the property that matters
for the kill switch built on top of it.

## Manual and webhook fires

Both refuse rather than suppress: there is no cursor to advance, and the caller
is synchronous and can be told why. The webhook path evaluates status after
`verifySignature` so an unauthenticated caller cannot probe a routine's status
through the response code (AC-OFFICE-ROUTINE-STATUS-004.3). The status check sits
alongside the existing `trigger.Enabled` 409, which is the same shape of refusal.

That existing `trigger.Enabled` check runs *before* the signature, and is
deliberately left there. It leaks the same class of fact this design is careful not
to leak, but moving it changes the behavior of a path this capability does not own,
so the two checks are intentionally ordered differently and the inconsistency is
recorded rather than quietly fixed.

The two paths look symmetrical from outside and are not symmetrical inside, so the
gate sits in a different place on each. The webhook handler already reads the routine
itself, with `GetRoutine`, after verifying the signature and before dispatching; its
gate is one branch on the routine it is holding, and that read is the single read
AC-OFFICE-ROUTINE-STATUS-004.7 requires.

The manual route has no such seam. `runRoutine` delegates the whole operation to one
service call, `FireManual`, which performs its own routine read and then dispatches,
and the handler maps every error it returns to 500. So the gate goes **inside
`FireManual`**, on the routine that call already read, and the refusal is returned as
a value the handler can recognize and map to 409 with the body in
[the API contract](#api). Two alternatives were rejected. Reading the routine again in
the handler to decide before calling would violate -004.7 by deciding on a different
read from the one that dispatches, and would double the cost of the most common path.
Leaving the refusal as an ordinary error would surface a paused routine as a 500,
which -004.1 forbids and which the operator cannot act on.

Concretely, the refusal is a distinct error type carrying the observed status as a
field, and the handler recognizes it with `errors.As` before its existing catch-all.
The type is what carries the status: the body in [the API contract](#api) reports that
value verbatim, and `FireManual` is the only place that read it, so a shared sentinel
compared with `errors.Is` could not deliver it. Recognition must also be by type rather
than by message, because every *other* error `FireManual` returns keeps its present 500
mapping — including a routine that does not exist, which stays 500 rather than becoming
404 (named out of scope in the requirements). `office/dashboard` already does exactly
this: `ApprovalsPendingError` (`dashboard/decisions.go`) carries its payload as a field
and `respondStatusUpdateError` (`dashboard/handler.go`) type-asserts it into a 409 body
while everything else falls through to one generic status. This design follows that
shape rather than inventing a second one, and `FireManual`'s single caller means no
other call site has to learn about the new type.

Splitting `FireManual` into separate fetch and dispatch calls is a reasonable future
refactor and is not required here: the gate needs the routine and the dispatch needs
the same routine, and one call that reads once satisfies both.

## Frontend surfaces

Two routes read routine status: the list (`app/office/routines/`) and the detail
view (`app/office/routines/[id]/`). Both render a next-fire time, so both are gated
here; beyond that and how it reports a refused "Run now", the detail view is unchanged.

- `routine-row.tsx` computes `isActive` as `routine.status === "active"` and uses
  it for the badge and the toggle. That predicate is widened to the same
  firing-status rule the backend uses, so a routine with an empty stored status
  reads as on rather than off (AC-OFFICE-ROUTINE-STATUS-006.5). The mapping stays
  `active`/empty to on, everything else to off, so `archived` continues to render
  as off with no new state. The list's enabled toggle already writes only `active`
  or `paused`, so widening the read side introduces no new written value.
- `nextFireText` renders a countdown from any cron trigger carrying a
  `next_run_at`. It is gated on the same predicate. Without this gate, suppression
  would leave a paused routine advertising a fire that will not happen, which is
  the failure mode this capability exists to remove, reintroduced one layer up.
- The detail view renders the same promise through a different component:
  `routine-detail-view.tsx` passes its cron trigger's `nextRunAt` to
  `DetailReadOnlyCard`, which prints `office:nextFire` unconditionally. These are the
  only two places a next-fire time reaches an operator, so gating one and not the other
  would leave the defect reachable at `/office/routines/:id`. The card is gated on the
  same predicate and renders the same empty state it already shows when no cron trigger
  carries a `next_run_at` (AC-OFFICE-ROUTINE-STATUS-006.1, -006.2). Its `lastFiredAt`
  line is unaffected: a past fire is a fact, not a promise.
- The `handleRunNow` handlers on both routes today toast the API error message
  verbatim. They instead branch on `ApiError.errorCode`: the status-refusal code from
  [the API contract](#api) selects new localized copy in the `office` namespace,
  interpolating the `status` field, and anything else keeps the existing fallback
  toast. This is the pattern `lib/api/task-delete-errors.ts` already uses, and it is
  what makes AC-OFFICE-ROUTINE-STATUS-006.4 and -006.6 satisfiable together: a message
  naming the status, in the operator's locale, without the backend owning display copy.

The detail view's status control is untouched. It seeds its draft with
`routine.status ?? "active"` and submits `status` on every save, which is harmless
while nothing validates the field. It stops being harmless the moment a writer
rejects out-of-set values, and that is exactly why the two are cut together: the
requirements' out-of-scope entry for write validation carries the trap and its fix,
so a follow-up cannot ship one without the other.

## Failure and recovery

- **Routine read fails.** No fire, cursor untouched, trigger stays due, logged
  under the repeat bound. The next tick retries. This is the fail-closed direction
  required by [ADR 0009](../../../decisions/0009-fail-closed-gc-semantics.md): the
  consequential action is launching an agent, so an unreadable status does not
  authorize it.
- **Routine is gone for good.** Deleting a routine normally takes its triggers with
  it: `DeleteRoutine`'s statement targets only `office_routines`, but
  `office_routine_triggers` declares
  `FOREIGN KEY (routine_id) REFERENCES office_routines(id) ON DELETE CASCADE` and
  the production connection enables foreign keys, so the delete cascades and no
  orphan is created. An orphaned trigger is therefore the exception, not the normal
  outcome of a delete: it needs a database with foreign keys disabled (which the
  service's unit-test harness is, since it opens `:memory:` without the flag), or a
  row removed by some path the constraint does not cover. Where one does exist,
  `infra/reconcile.go` reaps it at startup, matching triggers against the routines
  table.
  Such a trigger lands in the case above and stays due rather than retiring itself,
  which is a real change: today the claim nulls the cursor before the failing read,
  so the trigger goes quiet by accident of the bug this design fixes. It is
  nonetheless left alone, and the reason is the read, not the frequency.
  `GetRoutineFromConfig` swallows the underlying error and falls back to a name
  scan, so an absent row and a failed read arrive identically; disarming on that
  signal would be deleting live data on a lookup that failed, which is precisely
  ADR 0009's incident. The rule has to hold for the common case it also covers, a
  routine that is present but momentarily unreadable, where disarming would be
  simply wrong.
  The residual cost is worth stating accurately rather than optimistically. Each
  such tick costs the fallback scan inside `GetRoutineFromConfig`, which is
  `SELECT * FROM office_routines ORDER BY name` across every workspace, not an
  indexed read; the repeat bound below bounds the log entries but not that query. It
  is accepted because the state is rare and self-clearing at the next restart.
  Making the reconciler cheaper or more frequent is named out of scope in the
  requirements.
- **The new cursor cannot be computed.** Either `shared.NextCronTime` rejects the
  input, on a malformed expression or an unloadable timezone, or it returns a time
  that [the match check](#the-match-check) rejects, which is how its silent
  `after + 24h` fallback for a schedule with no match is caught. Both suppress without
  writing, logged under the repeat bound. The trigger stays due and is suppressed again
  every tick, which is correct: the routine is not firing either way, and the
  misconfiguration is the operator's to fix.
- **Cursor advance write fails.** Same outcome: no fire, cursor unchanged, logged
  under the repeat bound, re-evaluated next tick. The evaluation repeats every tick
  until the write succeeds; the log entry does not.
- **Compare-and-set loses a race.** No rows change and the loser returns. Exactly
  one evaluation advances the cursor and none fires.
- **Status changes mid-evaluation.** The gate reads status once per slot and the
  decision stands for that slot in both directions: a slot already past the gate
  completes its fire, and a slot already suppressed stays suppressed even if the
  routine is resumed a moment later, having consumed its cursor advance
  (AC-OFFICE-ROUTINE-STATUS-003.4). The practical effect of the second case is
  that resuming a routine exactly on a slot boundary may skip that one slot.
  Cancelling in-flight work is agent-level pause's contract, not this one.
- **`TickScheduledTriggers` already tolerates a per-trigger error**: it logs and
  continues to the next trigger. Suppression returns nil, so it does not enter
  that path.

## Persistence

No schema change and no migration. The only new write is
`AdvanceTriggerWithoutFiring`, which touches `next_run_at` and `updated_at` on
one `office_routine_triggers` row per call, one call per suppressed slot. Run
history, trigger rows, and routine rows are untouched by suppression, so a routine
paused for a week and then resumed has the same durable state it would have had if it
had never been due, apart from an advanced cursor.

Across a restart, a suppressed routine behaves identically: its cursor is in the
future, the trigger is not due, and nothing replays.

## Security

The webhook ordering rule is the only trust-boundary change: signature first,
status second, so response codes do not disclose a routine's status to an
unauthenticated caller. The 409 body names the status, which is deliberate and safe
in that order, because only a caller that has already proved possession of the signing
secret can reach it. Manual fires are already inside the authenticated Office API and
its workspace scoping (`officeWorkspaceScopeMiddleware` resolves `/routines/:id` to
its owning workspace), which this design does not change.

## Observability

Structured zap logs only, from the routines service logger
(`component=routines-service`). Three entries, under two different rules, and the
split is the point:

- **Suppressed slot** — routine id, trigger id, observed status, old and new cursor.
  **Not deduplicated.** This entry is written only when the cursor advance succeeds
  (AC-OFFICE-ROUTINE-STATUS-001.8), so the trigger stops being due and the next entry
  cannot arrive until the routine's next scheduled slot. Its volume is therefore the
  routine's own schedule, which is what an operator watching a pause take effect wants
  to see, and it needs no bound. Deduplicating it would be actively wrong: a routine
  paused, resumed, and paused again would log only the first pause.
- **Unreadable routine on a due trigger** — trigger id, and the error as observed. It
  does not say the routine was deleted, because the read cannot tell that from a
  failed read (AC-OFFICE-ROUTINE-STATUS-003.2).
- **Cursor not advanced** — trigger id, and whether the slot could not be computed or
  could not be written.

The last two are bounded to **once per trigger per process**
(AC-OFFICE-ROUTINE-STATUS-003.6). They are the two outcomes that leave the trigger due
without advancing it, so it is re-evaluated every tick and an unbounded rule would
write one entry every 30s for the life of the process. The comparison key is the
trigger id and which of the two outcomes it is, and nothing else. Keying on a
distinguishing value as well — the error text, an error class, the observed status —
was considered and rejected: it buys re-logging when the failure mode changes, at the
cost of a rule whose behavior depends on how errors are classified, which is not
something an acceptance criterion can test deterministically. The state is a
per-process in-memory set of `(trigger id, outcome)` pairs, bounded by trigger count
times two, and a restart logs each outcome once more, which is what an operator
reading a fresh log wants anyway.

That set is shared mutable state, and the check and the insert are one atomic
operation whose result decides whether the entry is written. The reason is a boundary,
not an observed race, and it is worth stating precisely because the obvious
justification is wrong. Today's wiring cannot overlap two `TickScheduledTriggers`
passes: `cron.Loop` starts one goroutine per *handler* per tick but then waits on all
of them before the loop returns to its ticker, there is a single routines handler with
a single call site, and `TickScheduledTriggers` walks its due triggers sequentially. So
one pass finishes before the next begins, and a plain map would in fact be safe.

It is required anyway, because that serialization belongs to `internal/scheduler/cron`,
which [Purpose and boundaries](#purpose-and-boundaries) names as a contract this design
uses and does not own. A bound this capability promises must not rest on a cadence
another package is free to change — and a second process, a test driving the service
directly, or a future per-trigger fan-out would each break it. The failure is also
silent and non-deterministic: a non-atomic test-then-insert lets two passes both find
the pair absent and log twice, breaking the "at most once" in
AC-OFFICE-ROUTINE-STATUS-003.6, while an unguarded map is a data race outright. The
guard costs one lock; discovering its absence costs a flaky assertion nobody can
reproduce.

No expvar counter is added; that is named out of scope in the requirements. A
suppressed slot writes no `office_routine_runs` row, so the runs list of a
routine paused for a week is empty for that week rather than holding one row per
missed slot.

## Prior art

### Our own prior reasoning (wiki)

**Receipt:** searched for the tool, not the vault. None of the 57 skills in
`~/.claude/skills` is `wiki-query`; `wiki-query`, `wiki-switch` and `qmd` are absent
from `PATH`; `~/.obsidian-wiki` does not exist, so there is no symlink to resolve and
no `OBSIDIAN_VAULT_PATH` to report. No vault was queried, no grep fallback ran. This
leg did not run.

### What others shipped (saas-kb)

**Receipt:** the only MCP tools exposed to this session are `mcp__kandev__*`, and
`search_fsm_docs` is not among them, so `saas-kb` is not configured here. No query
was issued and no `ai_sdlc` filter applied. This leg did not run.

### In-repo prior art, used instead

Both external legs were unavailable; the substitute is this repository's own record.

- **Fail-closed on an uncertain signal.**
  [ADR 0009](../../../decisions/0009-fail-closed-gc-semantics.md) followed a garbage
  collector deleting 307 live worktrees by reading a failed lookup as "orphan, delete":
  a consequential action needs a positive signal, never the absence of a negative one.
  REQ-OFFICE-ROUTINE-STATUS-001 reads status as an allowlist for that reason, and
  -003 declines to disarm a trigger on the same grounds. The deliberate contrast is
  `checkIdleSkip`, which fails open because its consequential action is skipping work.
- **A status gate that refuses rather than defers.** `guardAgentStatus`
  (`office/scheduler/run.go`) and `heartbeatAgentRuntime.AllowFire` (`backendapp/cron.go`)
  both refuse to queue work for a paused agent, with no override. -004 follows that,
  which is why "Run now" is a 409. Neither faces a cursor, so both are prior art for
  the refusal, not the catch-up decision.
- **A discriminable API refusal the frontend localizes.** `lib/api/task-delete-errors.ts`
  and `lib/api/domains/canvas-error-copy.ts` already read `ApiError.errorCode` to choose
  localized copy rather than displaying a server string. -004.4 follows that convention
  rather than inventing a second one.
- **The contract named the field and never defined it.**
  [Scheduler system design part 1](scheduler-01.md) lists `status: active | paused |
  archived` and says the coordinator routine can be paused, but nothing states what
  pausing does. That silence is this defect.

**What we are doing differently:** nothing departs from a position already taken here.
The one decision with no in-repo precedent is REQ-OFFICE-ROUTINE-STATUS-002, dropping
suppressed slots rather than banking them, because no existing gate sits in front of a
schedule cursor. Banking them would turn a pause into a burst on resume.

## Related decisions

- [ADR 0009: Fail-closed GC semantics](../../../decisions/0009-fail-closed-gc-semantics.md)
