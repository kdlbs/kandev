---
status: draft
system: office
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-001
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-003
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-004
---

# Parent Wake Wave Identity System Design

## Purpose and boundaries

This design gives the `task_children_completed` wake a single durable identity,
persisted on the run row, produced by every producer, and compared by the backstop's
candidate query in place of a timestamp. Office owns the outcome: whether an
autonomous run enters the Office run queue. Adjacent contracts it reads and
constrains but does not own:

- `internal/workflow/engine` — the `on_children_completed` trigger, the action path
  turning it into a `queue_run`, the idempotency key it synthesises, and the operation
  ledger. This design adds a value travelling beside that key.
- `internal/orchestrator` — the task-state and terminal-step handlers dispatching the
  same trigger for every workflow, Office or not.
- `internal/task/repository/sqlite` — `tasks` and its `archived_at`, `is_ephemeral`,
  `origin` and `updated_at` columns, plus `IsFromOfficePredicate`.
- `internal/runs/service` — run admission, the idempotency window, coalescing, and
  unique-violation classification.
- `internal/db/dialect` — the expression helpers new cross-dialect SQL goes through.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-WAKE-WAVE-IDENTITY-001` | [Wave identity](#wave-identity) |
| `REQ-OFFICE-WAKE-WAVE-IDENTITY-002` | [Persistence](#persistence), [Failure and recovery](#failure-and-recovery) |
| `REQ-OFFICE-WAKE-WAVE-IDENTITY-003` | [Backstop admission](#backstop-admission) |
| `REQ-OFFICE-WAKE-WAVE-IDENTITY-004` | [Producers](#producers) |

## Current state

Four producers can queue a `task_children_completed` run for one parent, each
verified at `cd7823631`.

| Producer | Entry point | Fires on | Route to the queue | Identity it uses today |
| --- | --- | --- | --- | --- |
| P1 cascade | `office/scheduler/reactivity.go` `cascadeChildrenCompleted` | inline from `ApplyTaskMutation`, via the dashboard status update | direct `QueueRun` | `childrenCompletedIdempotencyKey`: parent, agent, digest of child ids, straight to `runs.idempotency_key` |
| P2 edge | `office/service/event_subscribers.go` `queueChildrenCompletedRun` | `events.TaskMoved` into a done-category step | workflow engine | `wakeOperationID`: parent, digest of child ids, wrapped by the engine's `idempotencyKey` |
| P3 backstop | `office/service/scheduler_wake_reconciler.go` | cron sweep over `ListStuckParents` | workflow engine | same `wakeOperationID` as P2 |
| P4 orchestrator | `orchestrator/event_handlers_children_completed.go` `processOnChildrenCompleted` | task state → terminal, and terminal step moves | workflow engine | `childCompletionOperationID`: parent, then per child id, state, step, terminal flag and `updated_at` at ns resolution |

P4 is not gated on Office: it reaches any parent with an active session whose step
configures `on_children_completed`, which shipped `office-default` does with
`reason: task_children_completed`. Its `EvaluateOnly` call suppresses persistence and
the step transition but still evaluates actions, so its `queue_run` reaches the queue.

### The four child-set sources disagree, three ways

The fact the design turns on, and wider than an archived-child mismatch:

| Source | Feeds | `archived_at` | `is_ephemeral` | automation origin | ordered by |
| --- | --- | --- | --- | --- | --- |
| `ListChildStates` (`office/repository/sqlite/blockers.go`) | P1 | not filtered | not filtered | not filtered | `id` |
| `GetChildSetKey` (`office/repository/sqlite/wake_receipts.go`) | P2, P3 | `IS NULL` | not filtered | not filtered | `id` |
| `ListChildCompletionRows` (`task/repository/sqlite/task.go`) | P4 | `IS NULL` | `= 0` | excluded | `created_at`, then `id` |
| `ListStuckParents` child aggregate (`wake_receipts.go`) | admission | `IS NULL` | not filtered | not filtered | `id` |

Two consequences. Telling a producer to reuse "the rows it already has" gives four
different answers, so the constraint would be decorative for at least one. And
`ORDER BY id` is *not* what every existing query uses — P4's source orders by
`created_at` first, so reusing its rows in arrival order derives a different string
from the same set. Separately, the backstop's staleness arm compares
`runs.requested_at` against `MAX(tasks.updated_at)` over children while
`updateTaskTx` stamps `updated_at` on every write regardless of which column changed:
symptom B's root.

## Wave identity

### Which children count

The wave-member predicate is the one `ListChildCompletionRows` already applies: not
archived, not ephemeral, not automation-origin. P4 forces the choice: P1–P3 can each be narrowed cheaply, but P4's row set is the one its
readiness check consumes, so widening it would change *when the
`on_children_completed` trigger fires*, a task-system contract that is out of scope.
It is the only predicate all four can share without moving a readiness boundary, and
it is right on its merits: ephemeral and automation-origin tasks are machinery,
not delegated work.

Two consequences:

- **Readiness and identity now count different children, deliberately.** A parent with
  a non-terminal ephemeral child is still not *ready* (readiness arms are untouched,
  per .004.7) even though that child is not part of its wave. Do not "fix" this by
  propagating the wave-member predicate into a readiness predicate.
- **Narrowing cannot split a wave.** Excluding a child can only merge two identities
  that differed by an invisible row, never make one wave look like two, so the change
  direction is safe for duplicate wakes.

The predicate is expressed once, as a shared SQL fragment in the office repository;
`task/repository/sqlite`'s `andNotAutomationOrigin` is the precedent.

### The two encodings

```text
waveString(parentID, memberIDs) = parentID + "|" + strings.Join(memberIDs, ",")

waveKey(parentID, memberIDs) =
    "task_children_completed:" + parentID + ":" +
    lowerhex(sha256(waveString(parentID, memberIDs)))
```

`memberIDs` are the ids of the parent's wave members, ascending by `tasks.id`.

Both are pure functions of the same two inputs, with no I/O and no error return,
which is what .001.10 requires of the *derivation* — the function, not the caller: a
producer may still need a read to obtain the member ids, and .002.12 and .002.15
govern that read.

Why two encodings rather than one. **The backstop must compare in SQL** (.003.8) — a
candidate rejected in Go after the SELECT occupies a row of the query's `LIMIT` every
tick and starves an actionable parent behind it — and SQL here cannot hash, since
`mattn/go-sqlite3` registers no custom functions and there is no `pgcrypto` in the
tree, so the compared encoding has to be the plain string. **The uniqueness
constraint must be bounded** — a PostgreSQL btree entry caps near 2704 bytes, so a
parent with roughly seventy children would push an unhashed key past it and fail the
insert outright, so the indexed encoding has to be the digest. Neither job can use
the other's encoding, so both are persisted, derived together from one member-id
slice at one call site per producer.

Choices a builder would otherwise have to invent, fixed here:

- **The separator is `|`, deliberately not a NUL byte.** PostgreSQL `text` cannot
  contain `U+0000` at all, so a NUL-separated string is unstorable there. A UUID
  holds only hex digits and dashes, so `|` cannot occur inside an id.
- **Ordering is `ORDER BY id` ascending**, and a producer whose rows arrive
  otherwise re-sorts first. `tasks.id` is the primary key, so the order is total and
  there is no tiebreak to choose.
- **That order must be the *same* in Go and in SQL** (.001.13). Go sorts by byte;
  PostgreSQL `ORDER BY c.id` sorts by database collation, which for most locales is
  not byte order. Task ids are `uuid.New().String()` — lowercase hex, dashes at fixed
  positions — and for that alphabet the two coincide, so no `COLLATE` clause is
  needed. This is a premise worth stating because its failure is invisible: SQLite
  (BINARY) never diverges, while on PostgreSQL a divergence would make every parent
  look permanently stale and the sweep re-wake continuously. A non-UUID id source is
  what would break it, and the cross-dialect test is what would catch it.
- **The digest covers ids only** — state, workflow step, terminal flag and
  `updated_at` all excluded.
- **The parent id appears in the wave string and again as a readable prefix on the
  wave key.** The prefix keeps the stored value diagnosable; its presence in the
  digest input stops two parents with an identical child set colliding.
- **An empty `memberIDs` slice has no wave.** Producers return before deriving, so
  neither function is called with one and no key of that shape is written.

### Where the derivation lives

A small leaf package holding the two pure functions and nothing else, imported by
`office/scheduler`, `office/service` and `orchestrator`. The constraint is *not* that
`office/scheduler` may not import `office/service` — it already does
(`executor_resolver.go`, `wake_payload.go`, `run.go`). It is the reverse:
`office/service` must not import `office/scheduler`, because that edge closes a cycle,
which rules out the scheduler as the shared home. `orchestrator` importing a leaf
office package is established practice — `orchestrator/model_info.go` imports
`internal/office/costs/modelsdev`.

## Persistence

### Two columns on `runs`

```sql
ALTER TABLE runs ADD COLUMN wake_wave_key    TEXT NOT NULL DEFAULT '';
ALTER TABLE runs ADD COLUMN wake_wave_string TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_run_wake_wave
    ON runs(wake_wave_key, agent_profile_id)
    WHERE wake_wave_key <> '';
```

`wake_wave_key` carries the digest and is indexed; `wake_wave_string` carries the
plain string the backstop compares. Both are written in the same insert from one
derivation, so they cannot disagree; a test asserts `wake_wave_key == waveKey` of the
inputs that produced `wake_wave_string`.

Applied through the replayable `r.migrate.Apply` path the other office column
migrations use (`runs.continuation_scope` in `base_migrations.go` is the model) and
added inline to the fresh-schema `CREATE TABLE` in `office/repository/sqlite/base.go`
so a new database and an upgraded one converge. The office repository creates `runs`,
so this is its migration to own, and both statements are written for both dialects.
They run after `runs` exists and none rewrites a row; index creation cannot fail on a
pre-existing duplicate pair, because both columns are new and default to empty, so no
existing row is inside the partial index.

The columns live on `runs` rather than in a claim table because the constraint must
be enforced in the same transaction as the run insert, and that insert is a single
statement; a claim table would need a caller-owned transaction threaded from the
office producers through the engine's action dispatch into the runs service, and no
seam supports that.

Both columns carry a value only for `task_children_completed` runs; every other run
leaves them empty and, the index being partial, sits outside the constraint.

`(wake_wave_key, agent_profile_id)` rather than `wake_wave_key` alone: the engine can
fan one trigger out to several targets, which is why the receipt records an operation
id rather than one delivered run id. Constraining the pair suppresses a second run to
the same agent for one wave while leaving a genuine fan-out to *different* agents
intact. Per .002.13, a fan-out across participants resolving to the *same* profile
collapses to one run: an agent holding two roles is not two waves.

### Coalescing: the third write path into `runs`

`runs` is written three ways, not two, and the third is easy to miss because it is an
UPDATE. `runs/service.QueueRun` consults `CoalesceRun` **before** `insertRun`, and
`CoalesceRun` merges the request into an existing `queued` row for the same agent and
reason inside a time window, incrementing `coalesced_count` and **replacing that
row's payload**. `shouldCoalesceRun` excludes only `task_comment:`-prefixed keys, so
`task_children_completed` is eligible, and `CoalesceRun`'s task-scoping predicate
applies **only** when `reason == 'task_assigned'`.

So a children-completed wake can merge into a queued one for a **different parent**.
That row then carries parent A's wave columns and parent B's payload, and the backstop
matches runs to parents through the payload: A has no matching run and is correctly
re-queued, while B's row shows A's wave string, never matches B's current wave, and B
is queued a **second** time. The duplicate this capability removes survives, through a
path the constraint never sees.

Note what is *not* the mechanism: two engine-path producers deriving the same
`wakeOperationID` are already collapsed upstream by `CheckIdempotencyKey`. Coalescing
bites where the keys differ — here, different parents.

The decision (.002.14): **a request carrying a wave identity is never merged, and a
queued run carrying one is never merged into.** The guard is on the wave columns, not
the reason string, so a future wave-keyed reason is covered without a second edit.
Both requests are then reconciled by `idx_run_wake_wave`, the stronger mechanism
anyway: exact and unbounded, where coalescing is approximate and windowed.

One behaviour change follows and is accepted rather than hidden: two wakes for
different parents addressed to the same agent inside the window now produce two runs
where one previously absorbed and discarded the other. That is a correction — the
absorbed wake was silently lost. The asymmetric `task_assigned` scoping that causes
it stays; narrowing that belongs to whoever owns `CoalesceRun`.

### Existing identifiers are untouched

`childrenCompletedIdempotencyKey` (P1), `wakeOperationID` (P2, P3) and
`childCompletionOperationID` (P4) keep their definitions and values, and
`runs.idempotency_key` and `idx_run_idempotency` are unchanged.

The wave key is *purely additive* — a new column beside whatever identity a producer
already computed, never a replacement — which keeps the operation ledger stable,
keeps every `parent_child_wake_receipts` row already written meaningful, and removes
any historical-operation-id migration question. None of that would hold if
`wakeOperationID` were redefined to be the wave key, since the wave-member predicate
is narrower than `GetChildSetKey`'s. The two
constraints answer different questions — "same dispatch" and "same wave" — and only
the second can be computed identically by a direct caller and by the engine.

### Receipts

`parent_child_wake_receipts` is unchanged and `GetChildSetKey` keeps its predicate and
state-inclusive format: the backstop uses that key for the two re-reads that keep a
concurrent child update from being masked by a previous generation's receipt, and that
check wants to notice a state change. `wakeOperationID` being unchanged,
`delivery_operation_id` keeps its meaning.

## Producers

All four derive from the wave-member predicate and pass the resulting pair onto
the run they queue.

- **P1 cascade.** Keeps `ListChildStates` and its terminal-state loop as they are —
  narrowing that read would change cascade's readiness check, which .004.7 forbids —
  and gains a separate wave-member read. `RunContext` gains wave-key and wave-string
  fields beside its `IdempotencyKey`, carried through `SchedulerService.QueueRun` into
  the run row. Its no-session fallback is untouched: it still queues directly and
  never consults the engine (.004.2).
- **P2 edge and P3 backstop.** Both gain the same wave-member read.
  `GetChildSetKey` is *not* reused for the derivation — wider predicate,
  state-inclusive — and stays where it is, serving receipts.
- **P4 orchestrator.** No new read and no new filter: its rows are already exactly
  the wave members. It must re-sort by `id` before deriving, because
  `ListChildCompletionRows` orders by `created_at` first.
  `childCompletionOperationID` is unchanged, so the operation ledger, the
  `EvaluateOnly` transition lifecycle, and the reopen-driven step transition all
  behave as today.

### The wave-member read is the last read, and it carries state

Every producer reaches the queue through several separate, untransacted reads: a
readiness check, for P2 and P3 a receipt re-read, and now the wave-member read.
Nothing orders them, so a child can be added, archived, or leave a terminal state
between two. Unconstrained, that records a run against a member set that was **never
simultaneously terminal**, and because deduplication is unbounded (.002.6) that
identity suppresses the real wave *forever* — a lost wake, strictly worse than the
duplicate this design removes.

The rule that closes it (.002.15) needs no new transaction seam:

- the wave-member read returns `(id, state)` per member ordered by `id`, not ids
  alone;
- it is the producer's **last** read of *child task state* before queueing;
- if any returned member is non-terminal, the producer queues nothing and returns.

The recorded identity is then always derived from one statement's rows that the same
statement showed entirely terminal. A change landing *after* the read is harmless: it
moves the wave identity, so the backstop sees a mismatch and queues the new wave
normally. Only the never-terminal set had to be excluded.

The confirmation can only reject, never admit, which keeps it inside .004.7 — the
earlier readiness gates are untouched and strictly wider, so a set that passes them
and fails this defers to the backstop. P4 satisfies it by construction:
`ListChildCompletionRows` returns `state` and `readyChildCompletionRows` establishes
terminality from those same rows.

For P3 this also settles which value is authoritative: `ListStuckParents`' SQL wave
string is an **admission filter only**, never written to a run, and every producer
writes the identity from its own final wave-member read. A candidate row whose wave
string has gone stale needs no special handling — the producer's read is fresher and
its terminality confirmation decides.

A producer deriving an **empty** member set queues nothing and returns (.001.7). This
is live, not theoretical, now the predicate is narrower than the readiness checks: a
parent whose only child is ephemeral or automation-origin passes P1's
`AreAllChildrenTerminal` gate and its `ListChildStates` loop, and only the
wave-member read reveals there is no wave. P4 already guards it
(`readyChildCompletionRows` rejects `len(rows) == 0`); P1, P2 and P3 gain the same.

P1, P2 and P3 read through one new office-repository method returning wave-member
`(id, state)` ordered by `id`. Because P4 derives from `ListChildCompletionRows`
instead, a test asserts the two SQL sources return an identical id set for the same
parent — the equality .001.8 rests on, and the one place two queries must be kept in
step by hand.

The engine seam is narrow: both encodings accompany an `on_children_completed`
trigger to the `queue_run` action and are written to the run row, carried by
`OnChildrenCompletedPayload` (`workflow/engine/payloads.go`). They do not participate
in `idempotencyKey`, `queueActionDigest`, or the operation ledger —
`queueActionDigest` keys off the workflow-authored *action* payload, not the trigger
payload.

## Backstop admission

`ListStuckParents` computes the parent's current wave string in SQL alongside the
existing state-inclusive `child_set_key`, over the wave-member predicate:

```text
p.id || '|' || COALESCE(<ordered group concat of wave-member c.id>, '')
```

The aggregate needs a dialect branch, added to `internal/db/dialect` beside
`JSONExtract`: SQLite's `GROUP_CONCAT(c.id, ',')` takes ordering from an ordered
subquery, the idiom the existing `child_set_key` aggregate uses; PostgreSQL's
`string_agg(c.id, ',' ORDER BY c.id)` takes it inside the aggregate. Writing one and
hoping is how ordering silently differs between dialects.

That expression is evaluated for **every** CTE row, including a parent with no wave
members, where it yields a well-formed `"<parentID>|"` — a pre-gate intermediate, not
a wave string, since per the requirements' *Terminology* a parent with no wave
members has neither. Nothing compares it, because the gate below removes the row
first; note that a gate testing the assembled expression for emptiness would exclude
nothing, as it is never empty.

The gate is a separate `EXISTS` over the wave-member predicate, beside the existing
`EXISTS`:

```sql
AND EXISTS (
    SELECT 1 FROM tasks c
    WHERE c.parent_id = p.id
      AND c.archived_at IS NULL
      AND c.is_ephemeral = 0
      AND COALESCE(c.origin, '') != 'automation_run'
)
```

Without it, a parent whose only child is ephemeral or automation-origin would be
listed every tick, be found to have no wave, queue nothing, and be listed again — the
permanently rejected candidate .003.8 prevents. The existing
`EXISTS (... archived_at IS NULL)` arm keeps its archived-only predicate and is not
replaced: the gate is additive and narrowing, so it can only remove candidates, never
admit one not admitted today, which keeps it inside .004.7.

The second `NOT EXISTS` arm becomes three clauses, each naming its run statuses
explicitly per .003.10, and each status set is the one in force today:

- `status IN ('queued','claimed')` blocks regardless of wave — unchanged, so the
  sweep never races an in-flight wake;
- `status IN ('finished','failed','cancelled')` blocks when its `wake_wave_string`
  equals the parent's current wave string;
- **only when the parent has no `task_children_completed` run carrying a wave
  identity at all, in any status**, a terminal run blocks under the existing
  `requested_at >= newest_child_updated_at` rule.

The second clause keeps its three-status set deliberately, and that is the one place
this design changes *when* a wake is re-delivered. Today a failed or cancelled wake
blocks only until some child's `updated_at` overtakes `requested_at`, so any
unrelated edit silently re-arms it; comparing wave identity removes that churn, so a
failed wake blocks its wave until the wave changes. `ListStuckParents`' doc comment
already states the contract — a terminal failure stays terminal, needs an explicit
user retry, and must not become a cron retry loop — so this is the correct direction.
The escape hatches are an explicit retry and any wave-member change.

The third clause is the compatibility path for pre-change rows. Its guard is
status-agnostic on purpose — it asks about *vintage*, not delivery, and a keyed run
still `queued` blocks through the first clause anyway — and it is scoped to the
**parent**, not the row (.003.6): evaluating per row would let a parent's
pre-upgrade row keep answering long after a keyed run existed. "Carrying a wave
identity" is tested as `wake_wave_key <> ''`, the same predicate the partial index
uses, so a row is inside or outside both the constraint and the compatibility path
together. The path is bounded and self-clearing: an unrelated edit can still make a
pre-upgrade parent look stale and queue one wake, that wake writes a keyed row, and
the parent is judged by wave identity ever after. `newest_child_updated_at` stays
solely for this clause.

The reconciler's two re-reads keep using `GetChildSetKey`: they guard against a child
change between the SELECT and the dispatch, and the state-inclusive key is the
stricter guard.

One asymmetry follows from unbounded deduplication and is why .003.3 is qualified
rather than absolute. Admission can make a parent a candidate again on any
wave-member change, but the constraint still refuses an insert for an identity
already queued, so a set that changes and changes *back* — archive, then unarchive —
lists the parent and queues nothing. That is the id-set-reuse exclusion: admission
promises candidacy, not delivery.

## Wake equivalence between producers

AC-OFFICE-WAKE-WAVE-IDENTITY-002.10 requires the surviving run to deliver the same
wake whichever producer won. It binds only where one run is actually suppressed —
same wave *and* same target agent — so a workflow whose `on_children_completed`
action targets `workspace.ceo_agent` while the cascade wakes the parent's assignee
raises no equivalence question. Two payload sources have to agree for the cases that
do collide.

**Child summaries: equal, and equally empty.** The prompt reads them from the run
payload's `children` key (`enrichChildrenContext`,
`office/service/scheduler_integration.go`); no producer writes that key, so all four
queue a payload with none and that prompt section can never fill — a latent defect
filed separately. A change that starts populating `children` from one producer must
populate it from all four, or this AC breaks and prompt quality becomes a race
outcome.

**Workflow-authored action payload: NOT equal today; this design fixes it.**
`queueRunPayload` copies `in.Action.QueueRun.Payload` onto the run verbatim, so every
engine-path producer carries whatever the step's `on_children_completed` `queue_run`
action declares, and the cascade — never consulting the engine — has no way to. The
shipped `office-default` action declares none, so nothing diverges today; but the
workflow editor allows one, and the moment a workflow authors a payload while
targeting the parent's primary agent, the delivered context depends on which producer
won — and suppressing the loser is what makes that observable.

So, per .002.16, the cascade resolves the parent's current workflow step, reads that
step's `on_children_completed` `queue_run` action payload, and merges it into its own
run payload with the precedence `queueRunPayload` applies. The comment-specific fields
that function also merges do not apply to this trigger and are not reproduced. That
resolution reads workflow configuration, not child task state, so it may sit after the
wave-member read without violating .002.15.

Its failure behaviour is deliberately *not* the wave-member read's: if the authored
payload cannot be resolved, the cascade still queues the wake without it and logs the
omission. Its unconditional wake for a parent with no active workflow session is a
contract in its own right (.004.2) and outranks an optional payload. The degradation
is named rather than left to be discovered.

## Failure and recovery

- **Unique violation on insert — two classification sites, not one.**
  `IsIdempotencyKeyUniqueViolation`
  (`runs/repository/sqlite/idempotency_violation.go`) is deliberately scoped to
  `idx_run_idempotency` by name, so it gains an explicit sibling for
  `idx_run_wake_wave` rather than being widened to "any unique violation". The sibling
  matches PostgreSQL's `pgconn.PgError` code `23505` with constraint name
  `idx_run_wake_wave`, and SQLite's composite message `UNIQUE constraint failed:
  runs.wake_wave_key, runs.agent_profile_id` — a different shape from the
  single-column message the existing constant matches.

  Both queue paths must call it, and this is what a single-site fix gets wrong.
  `office/repository/sqlite.Repository` *embeds* `*runssqlite.Repository`, so P1 and
  the engine path call the very same `CreateRun`, but only `runs/service` wraps it
  with classification: `office/scheduler.SchedulerService.QueueRun` calls `CreateRun`
  directly and wraps the error as `enqueue run: %w`, logged by its caller as
  `reactivity run failed`. Left alone, P1 losing a wave race logs a failure, which
  .002.4 forbids — and P1 is the producer the race test drives. So (1)
  `runs/service`'s `insertRun`/`QueueRun` gains a second sentinel beside
  `errIdempotencyKeyConflict` and returns `QueueOutcomeDeduped`, covering P2–P4; and
  (2) `office/scheduler.SchedulerService.QueueRun` classifies the `CreateRun` error
  and returns `nil` with a debug log, as it already treats a `CheckIdempotencyKey`
  hit, covering P1.
- **Lost dispatch.** Unchanged: the backstop re-derives readiness from current state
  every tick and simply recognises delivery by wave rather than timestamp.
- **Retry.** `ScheduleRetry` updates in place and does not insert, so the constraint
  is never consulted on a retry (.002.11). A future retry-by-reinsert would have to
  carry both columns forward or clear them.
- **Coalescing.** Excluded for wave-carrying requests and rows, per *Coalescing*
  above — the counterpart of the retry note: both are UPDATE paths, and one moving a
  payload without moving the identity breaks .002.14.
- **Wave-member set changes mid-flight.** Handled by the terminality confirmation on
  the final read, not by locking. No producer records an identity for a set it did
  not observe entirely terminal, so unbounded deduplication can never suppress a wave
  that was not delivered.
- **Wave-member read fails.** The producer does not dispatch (.002.12). P2 logs at
  warn and defers to the backstop; P3 retries next tick; P1 already returns without
  queueing on a failed child read and does the same here. P4 cannot hit this case.
- **Upgrade.** No backfill. Existing rows keep the empty defaults, stay outside the
  partial index, and are judged by the parent-scoped compatibility clause.

## Security

No new trust boundary: both encodings contain only task ids the same caller already
reads, and neither is user-supplied or rendered.

## Observability

The existing `parent_wake_*` expvars and their paired `wake.metric.*` logs are
unchanged. One counter is added, `parent_wake_deduped_total`, incremented when a
`task_children_completed` insert is rejected by `idx_run_wake_wave`, at both
classification sites so a P1 dedupe is as visible as an engine-path one. It is the
only direct evidence the constraint is doing work, since a run that never exists
leaves no other trace. Parent task id is unbounded and is not a label,
matching the existing counters; it goes on the structured log.

## Testing

- **Derivation:** terminal-to-terminal edit stable; non-state edit stable; member
  added or removed changes it; archived, ephemeral and automation-origin children
  excluded; independent of insertion order; empty set never keyed; wave key is the
  digest of the wave string.
- **Cross-producer:** all four derive identical encodings for one parent and child
  set, including a parent with an ephemeral or automation-origin child and P4's rows
  in `created_at` order. The new wave-member method and `ListChildCompletionRows`
  return the same id set.
- **Dialect:** the wave-string aggregate is byte-identical on SQLite and PostgreSQL,
  ordering included.
- **Race:** cascade against one backstop tick yields exactly one run row and the
  loser returns success without logging a failure, mirroring the runs service's
  idempotency-index race tests and their PostgreSQL twin. The cascade case must run
  through `office/scheduler`'s own queue path, not only `runs/service` — those are the
  two classification sites.
- **Coalescing:** a second request for a *different* parent, same agent, inside the
  window does not merge into the first parent's queued run; two runs exist, each
  carrying its own identity, neither differing from the wake it delivers.
- **Read skew:** a wave-member read returning a non-terminal member queues nothing
  and records no identity, and a later genuine wave for that parent is still
  delivered — the case otherwise suppressed permanently.
- **Admission:** a non-state child edit after delivery lists no candidate; a
  wave-member change lists one; a parent whose only run predates the change still
  follows the timestamp rule; a parent holding both a pre-upgrade and a keyed run is
  judged by wave identity alone. A failed run whose wave string equals the current one
  keeps blocking across an unrelated child edit, and a wave-member change unblocks it.
- **Gate:** a parent whose only children are ephemeral or automation-origin is not a
  candidate and no producer queues for it; a parent that is a candidate today stays
  one whenever it has a wave member.
- **Payload parity:** the cascade attaches the workflow-authored
  `on_children_completed` action payload when the step declares one, matching the
  engine path byte-for-byte, and still queues the wake without it when it cannot be
  resolved.
- The two identity tests pinning the id-only decision, and the cascade's
  children-completed tests, must continue to pass unmodified.

## Related decisions

- [ADR-0015](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
