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

# Office Unattended Launch Safety System Design Part 2

## Purpose and boundaries

This is the second half of one design. Part 1
([Office Unattended Launch Safety System Design](unattended-launch-safety-01.md))
carries the purpose, the requirement mapping, the components, and the data and
contracts: the run columns, the launch ledger, the actor contract, the task-boundary
carrier, the configuration keys and the priority-class mapping. This part carries the
control flow that uses them, and what happens when it fails. Read Part 1 first; every
identifier used here is defined there.

## Control flow

### Enqueue

`runs/service.Service.QueueRun` gains a guard, in this order, inside one transaction
as AC-OFFICE-LAUNCH-SAFETY-003.8 requires. On Postgres the transaction also takes an
advisory lock keyed on the woken `agent_profile_id`, for the same reason the claim
gate takes one: each self-trigger check is a *count over other rows* that the insert
does not lock, so `READ COMMITTED` lets two concurrent enqueues both read the last
remaining allowance and both pass. The claim gate's lock is instance-wide because its
broadest limit is; the enqueue gate's narrowest scope is the agent profile, which is
also the self-trigger key, so per-agent is sufficient and keeps unrelated enqueues
parallel.

1. Resolve the actor from the path's declared source
   (AC-OFFICE-RUN-CAUSATION-001.23). The field is required, so there is no
   omitted case; an unrecognized *declared* value becomes `system`, never `user`.
2. **Resolve idempotency and coalescing.** If this request merges into an existing
   run, merge and return: the merge is counted once against the surviving run, which
   keeps its own actor, routine and priority class
   (AC-OFFICE-RUN-CAUSATION-001.22), and **no gate below is applied to it**.
3. Resolve the workspace from the woken agent's profile. Empty refuses
   (AC-OFFICE-RUN-CAUSATION-001.20).
4. Resolve causation. A human actor roots a new chain regardless of what the
   originating task or comment carried. Otherwise, if `CausingRunID` is set, read
   that run: **if it cannot be read, refuse** per AC-OFFICE-RUN-CAUSATION-001.21 —
   do not root the wake, because rooting an unreadable ancestor lets any chain reset
   its own depth by outliving one row. Otherwise inherit its causation id, set
   `parent_run_id` to it, and set depth to its depth plus one; if it carries the
   legacy empty causation id, adopt its own run id instead. With no causing run, and
   when the carrier's creating run id is empty, this is a root: depth `0`, empty
   parent, causation id set to the new run's own id
   (AC-OFFICE-RUN-CAUSATION-001.24).
5. Inherit `human_rooted` from the causing run or the carrier, or set it from the
   actor at a root. Resolve `routine_id` from the request or the carrier.
6. If depth exceeds the configured maximum, refuse.
7. If the wake is self-caused, apply both self-trigger window counts against
   persisted rows, per AC-OFFICE-LAUNCH-SAFETY-004.7, in the order
   AC-OFFICE-LAUNCH-SAFETY-004.8 fixes: first the per-reason count for that
   `(agent_profile_id, reason)` pair, then the reason-independent count for that
   `agent_profile_id` alone. Refuse at the first spent allowance, so a wake over both
   is refused once and recorded as a per-reason refusal. Both counts read the same
   window and the same actor predicate and differ only in whether `reason` is matched,
   so they are two statements over one pair of indexes, not two mechanisms.
8. A refusal at step 3, 4, 6 or 7 returns a typed error, writes an activity record,
   and increments the refusal counter, **without recording an idempotency key**.
9. Compute and stamp `priority_class`, then insert.

**Coalescing precedes every refusal gate**, which is AC-OFFICE-LAUNCH-SAFETY-003.10.
An earlier draft moved only the self-trigger count behind the merge, leaving the
workspace and causing-run refusals in front of it, and had no reason for the split. The
four refusals are one class: a merged request creates no run row, so none of them has
anything to decide about it, and a duplicate whose causing run happened to age out
between the original and the retry would otherwise be refused instead of merging into
the run it belongs to. Reading the idempotency key to find a merge target is not
recording one, so AC-OFFICE-LAUNCH-SAFETY-003.4 still holds: a refused request never
burns a key, and a later legitimate request carrying it still enqueues.

The retry, recovery-sweep, and routing re-dispatch paths do not pass through this
gate as new work; they reset an existing row, so depth is untouched by construction,
which is how AC-OFFICE-LAUNCH-SAFETY-003.7 is satisfied without a special case. They
do re-stamp `priority_class`, per AC-OFFICE-BACKPRESSURE-001.7.

### Claim

`ClaimNextEligibleRun` becomes a short transaction rather than a bare statement,
because two requirements need more than one statement can give.

```text
BEGIN
  -- serialize: SQLite's single writer already provides this;
  -- Postgres takes ONE instance-wide transaction-scoped advisory lock
  UPDATE runs SET status='claimed', claimed_at=:now
   WHERE id = ( SELECT w.id FROM runs w
                WHERE w.status='queued'
                  AND (w.scheduled_retry_at IS NULL OR w.scheduled_retry_at <= :now)
                  AND w.routing_blocked_status IS NULL
                  AND <agent ceiling> AND <workspace ceiling> AND <instance ceiling>
                  AND (w.human_rooted = 1 OR (<workspace budget> AND <routine budget>))
                ORDER BY <promoted class> ASC, w.requested_at ASC, w.id ASC
                LIMIT 1 )
   RETURNING *
  INSERT INTO office_launch_ledger (...) VALUES (...)
COMMIT
```

Five things in that sketch are load-bearing.

**Serialization is explicit, and the lock is instance-wide.**
AC-OFFICE-LAUNCH-SAFETY-001.6 promises that two concurrent claims can never observe
the same free slot. The outer `WHERE id = (...)` protects only the row being updated;
the ceiling and budget subqueries count *other* rows the update does not lock.
SQLite's single-writer lock makes that safe. Postgres under `READ COMMITTED` does
not: two transactions claiming two different candidate rows each read the same
pre-commit count and both pass. Postgres therefore takes a transaction-scoped
advisory lock, and **its key is a single fixed instance-wide constant**, not the
workspace or the agent profile. A per-workspace key would serialize the workspace
ceiling while leaving the instance ceiling racing between workspaces, which is the
broadest limit and the one that protects the machine. Precedent:
`pg_advisory_xact_lock` is already used in `internal/secrets` and
`internal/workflow/repository`. At `maxRunsPerTick = 10` contention is negligible,
and a lock beats `SERIALIZABLE` because it needs no retry contract.

**The agent ceiling is dialect-aware.** `MAX(x, y)` is SQLite's two-argument scalar
form and is not portable; Postgres spells it `GREATEST`. Postgres is a live driver
and `internal/runs` has a Postgres suite, so the expression is built through
`internal/db/dialect`, which already carries a `GREATEST`/`MAX` helper in `time.go`.
A missing `agent_profiles` row makes the subquery `NULL` and the predicate false,
which defers the run per AC-OFFICE-LAUNCH-SAFETY-001.8 — not the same thing as an
unbounded ceiling. The effective ceiling is clamped to the workspace and instance
values per AC-OFFICE-LAUNCH-SAFETY-001.9, so an agent raising
`max_concurrent_sessions` through `ModifyAgent` cannot raise the real bound.

**The workspace comes from the run, not a join.** All three of the workspace
ceiling, the workspace budget and the ledger insert read `w.workspace_id`, the
column stamped at enqueue. The claim statement therefore joins `agent_profiles` for
the agent ceiling only, and the ledger row is built entirely from the `RETURNING *`
projection, which would otherwise not carry a workspace at all.

**The budget exemption tests `human_rooted`, not the priority class.** The `human`
class is assigned from the *current* run's actor and reason, so it would exempt an
agent-caused `approval_resolved` while missing a genuinely human-rooted run at depth
1 or deeper. AC-OFFICE-LAUNCH-SAFETY-005.6 asks about the root of the chain, and
`human_rooted` is the only field that answers that at every depth.

**Promotion clamps at `recovery`, and its boundary is strict.** With `human = 0` and
`recovery = 1`, a clamp at `0` would promote a `recovery` run into `human`, which
AC-OFFICE-BACKPRESSURE-002.2 forbids. The comparison is `<`, not `<=`, because
AC-OFFICE-BACKPRESSURE-002.1 promotes a run queued *strictly* longer than the period,
and `:promotionCutoff` is computed from the one declared clock of
AC-OFFICE-LAUNCH-SAFETY-005.8 rather than from database `now()`:

```text
CASE WHEN w.requested_at < :promotionCutoff
     THEN MAX(w.priority_class - 1, 1)   -- GREATEST on Postgres
     ELSE w.priority_class END
```

Promotion changes the `ORDER BY` only; the persisted class is untouched, so it
cannot accumulate across ticks, satisfying AC-OFFICE-BACKPRESSURE-002.3 and 002.4.
`ScheduleRetry` and `RecoverStale` leave `requested_at` alone, so a re-queued run
keeps accruing age — harmless, because AC-OFFICE-BACKPRESSURE-001.7 re-stamps it
`recovery` and promotion from `recovery` is a no-op.

A run that no gate admits is simply not selected. It stays `queued` with every column
untouched, which is AC-OFFICE-LAUNCH-SAFETY-001.7 and 005.4 by construction rather
than by an explicit reset.

#### Attributing a deferral

Because the claim statement returns nothing when a gate blocks, the statement alone
cannot say which gate decided. `SchedulerIntegration.tick` therefore runs one
diagnostic read **on any claim attempt that returns no row while eligible queued runs
exist** — not only on a tick that claimed nothing. A tick drains up to
`maxRunsPerTick = 10` runs and stops at its first empty attempt, so keying the
diagnostic on "the whole tick claimed nothing" would report nothing at all under
partial saturation, which is the common case and the one an operator most needs to
see. This is what AC-OFFICE-BACKPRESSURE-003.7 counts: one increment per blocked
claim attempt, not one per blocked run, because a set-based query cannot enumerate
the rows it did not select.

The diagnostic selects the highest-priority queued row under the **effective** claim
order — AC-OFFICE-BACKPRESSURE-001.2 with the promotion `CASE` applied, exactly the
`ORDER BY` the claim statement uses. Ranking by the persisted class alone would name
a different row than the one that would actually have gone next, precisely when
promotion is active. It then evaluates each gate against that row *individually*, in
a stated precedence, rather than relying on where a predicate sits in the query: SQL
predicate order is not evaluation order, and a row can fail several gates at once.

There are **two** precedences, because refusal and deferral see disjoint gate sets
(AC-OFFICE-BACKPRESSURE-003.2):

- **Deferral**, most specific first: `agent_ceiling`, `workspace_ceiling`,
  `instance_ceiling`, `routine_budget`, `workspace_budget`.
- **Refusal**, at enqueue: `causation_depth`, `self_trigger`, `self_trigger_total`,
  `causing_run_unreadable`, `workspace_missing`.

`causation_depth`, `self_trigger`, and `self_trigger_total` are deliberately **absent**
from the deferral precedence. An earlier draft listed the first two first, as the most
specific gates; but all three are evaluated at enqueue and can only prevent a row
existing, so a row sitting in `queued` has by construction already passed all three.
Those three slots could never fire, and a precedence with unreachable entries invites a
builder to implement them as no-ops and wonder what they mean. Narrower gates are still
reported before broader ones within each list, because a run blocked by both its agent
ceiling and the instance ceiling is more actionable when attributed to the agent.

This read is diagnostic only and never gates a launch, per
AC-OFFICE-BACKPRESSURE-003.4, so a failure in it cannot deny work. It is also a
**snapshot, not a transaction**: it runs after the claim attempt and after that
attempt's advisory lock is released, so a concurrent claimant may change occupancy or
take the selected row in between. Attribution is therefore best-effort by contract
(AC-OFFICE-BACKPRESSURE-003.11) and a test must not assert an exact gate label against
a concurrent claim. Serializing the diagnostic with the claim was rejected: it would
put a non-gating read inside the one lock that bounds launch throughput, to buy
accuracy in a counter whose worst error is one misleading increment.

## Failure and recovery

- **Ceiling or budget input unreadable.** "Unreadable" is defined, not left to
  judgement: the query errored, timed out, or returned a value outside its declared
  type or range. The gate then fails closed — no claim, the run stays queued,
  `office_launch_check_failed_total` is incremented with the gate label, and the
  gate's consecutive-failure counter advances. This is deliberately opposite to
  `SchedulerIntegration.checkBudget`, which returns `true` on a checker error. A cost
  check that fails open risks overspend a human can see afterwards; a concurrency
  check that fails open risks the unbounded fan-out this capability exists to prevent.
- **Shutdown, specifically, is not a gate failure.** A context cancelled because the
  process is stopping defers the run without incrementing any failure counter and
  without advancing the consecutive-failure count. Counting it would make every
  restart look like a gate that is failing closed, and would eventually escalate a
  clean deployment as an outage.
- **Ledger append fails.** The transaction rolls back, so the claim does not happen
  and the run stays `queued`. This is the same fail-closed posture: a launch that
  cannot be counted is a launch that does not occur.
- **Persistent fail-closed stall.** Because a fail-closed gate can stall the loop,
  a durable operator-visible record is the escalation path. It is written when the
  same gate has failed closed on `3` consecutive evaluations
  (AC-OFFICE-BACKPRESSURE-003.5), and the counter is reset by one successful
  evaluation of that same gate and by nothing else (`003.8`) — not by a restart, an
  empty queue, or another gate succeeding, any of which would let a genuinely stuck
  gate defer its own escalation indefinitely. **Successful means readable, not
  permissive** (`003.10`): a gate that read its input and then blocked the launch has
  succeeded, so a saturated pool resets the counter rather than escalating as an
  outage. The set-based claim statement need not report per-gate outcomes, because a
  statement that completes without error has by construction read every deferral gate
  it contains; a gate an attempt never evaluated keeps its count unchanged. The record is rate-limited to once per
  hour per `(workspace, gate)` pair, not per workspace (`003.9`): keyed on the
  workspace alone, the first failing gate would mask a second, independently failing
  one for an hour. No automatic reopening is performed; reopening a safety gate
  because it keeps failing would invert its purpose.
- **Depth refusal.** Terminal for that request. It is not retried, because retrying
  would produce the same depth.
- **Self-trigger refusal.** Either allowance, bounded by the rolling window and clears
  without intervention. The refusal record names which allowance refused, per
  AC-OFFICE-LAUNCH-SAFETY-004.8, so an operator can tell one agent looping on a single
  reason from one spreading the same loop across the registry.
- **Unregistered wake reason.** Rejected at the runtime-action boundary before the
  enqueue is called (AC-OFFICE-LAUNCH-SAFETY-004.3), so it burns no idempotency key
  and consumes no allowance. Terminal for that request: the reason set is fixed, so
  retrying the same reason produces the same rejection. This does not narrow the
  unmapped-reason fallback of AC-OFFICE-BACKPRESSURE-001.6, which still has to absorb
  reasons the system itself records and reasons read off historical rows.
- **Coalescing.** The surviving run keeps its own causation triple, actor, routine
  and priority class, per AC-OFFICE-RUN-CAUSATION-001.7 and `001.22`. A merged
  request contributes none of them, because the surviving run's depth already
  reflects a real chain and adopting a shallower one would let a chain reset itself
  by racing a merge. A merged request is also never refused by a gate, having been
  resolved before the gates run.
- **Causing run unreadable.** Refused, not rooted
  (AC-OFFICE-RUN-CAUSATION-001.21). Terminal for that request; the caller sees a
  typed error. A malformed value that arrived on the *task* carrier is different and
  is resolved value-by-value to its most restrictive reading
  (AC-OFFICE-RUN-CAUSATION-001.10), because there the creating run was never named
  and refusing would strand a legitimate task wake.
- **Stale-claim recovery.** `RecoverStale` returns a run claimed longer than
  `staleClaimedRunAge` to `queued` with no liveness check, so a live process can
  outlive its own claim and free a slot it is still using. This design does not
  compensate for that inside the counting rule; the requirement records the resulting
  one-per-agent overshoot as a known open hole owned by the launch lease work. Note
  that the ledger is unaffected: the original launch stays counted, which is correct.

## Persistence

```sql
CREATE TABLE office_gate_failure_state (
  workspace_id        TEXT      NOT NULL,
  gate                TEXT      NOT NULL,
  consecutive_failures INTEGER  NOT NULL DEFAULT 0,
  last_escalation_at  TIMESTAMP NULL,
  updated_at          TIMESTAMP NOT NULL,
  PRIMARY KEY (workspace_id, gate)
);
```

AC-OFFICE-BACKPRESSURE-003.8 requires the consecutive-failure count to be held per
`(workspace, gate)` and to **survive a restart**, and `003.9` requires an
at-most-once-per-hour record per the same pair, which needs a last-written timestamp.
Neither can live in the `expvar` maps below: those are in-process and reset on every
restart, which is the precise condition `003.8` exists to detect. `office_activity_log`
cannot serve either — it is append-and-list, with no keyed increment or reset. The
table above is the declared home, keyed on the pair so one failing gate cannot mask
another. Its precedent is `agent_profiles.consecutive_failures`, which already does
increment-and-reset in `office/repository/sqlite/failure.go`, keyed on agent profile
rather than on `(workspace, gate)`. The row is upserted inside the same transaction as
the claim attempt that observed the gate, so concurrent attempts cannot lose an
increment.

Nine columns are added to `runs`, two tables are added, and seven reserved keys are
written to `tasks.metadata`. All nine columns are `NOT NULL` with defaults, so the
additive migration converges an existing database in place with no data movement and
no backfill pass; existing rows read back as legacy roots. Both new tables are created
with `CREATE TABLE IF NOT EXISTS`, the repository-wide idiom, so the migration is
replayable per ADR 0027. Per the repository's
migration rule, each column arrives through an idempotent `ADD COLUMN` in
`runMigrations()`, and the new claim-ordering index is created *after* those columns
in the same migration sequence rather than in schema init, which runs first and would
fail on `no such column`.

Restart behavior is unchanged: the ceilings are computed from `runs.status`, which is
already the durable record the scheduler restarts against, so a backend restart
mid-run leaves claimed rows in place and the ceilings correctly occupied on the first
tick after restart. Budgets are computed from the ledger, which is equally durable.

## Security

The limits are operator configuration and are not settable by an agent. The runtime
action surface exposes causation as read-only context: an agent receives its
causation identifier and depth but cannot set, reset, or lower them, because the
enqueue gate derives every causation field and the actor from the server-side run
record rather than from the request body, per AC-OFFICE-RUN-CAUSATION-001.17. An
agent that could set its own depth, or declare its own actor as `user`, would defeat
every limit here in one call — which is why `ActorKind` is resolved from the
authenticated caller and not accepted from an agent-supplied field.

`ModifyAgent` already lets a sufficiently privileged agent write
`max_concurrent_sessions` on another agent in its workspace. Making that column
load-bearing turns an existing write into a safety-relevant one, which is what
AC-OFFICE-LAUNCH-SAFETY-001.9's clamp addresses.

Causation identifiers are opaque run identifiers and carry no user content, so they
are safe to log and to expose on the run API.

## Observability

`expvar` maps under `/debug/vars`, following the label idiom already used by the
Office stall detectors:

- `office_launch_deferred_total`, labelled by `gate` (`agent_ceiling`,
  `workspace_ceiling`, `instance_ceiling`, `workspace_budget`, `routine_budget`).
- `office_launch_refused_total`, labelled by `gate` (`causation_depth`,
  `self_trigger`, `self_trigger_total`, `causing_run_unreadable`, `workspace_missing`).
- `office_launch_check_failed_total`, labelled by `gate`, so a gate that is failing
  closed is visible rather than looking like a quiet system.
- `office_launch_priority_unmapped_total`, labelled by `reason`.
- `office_launch_actor_missing_total`, labelled by `reason`.
- `office_launch_causation_invalid_total`, labelled by `reason`, for the malformed
  task-boundary causation of AC-OFFICE-RUN-CAUSATION-001.10.

Each is also emitted as a structured log entry. A **deferral** entry names the run
identifier, the causation identifier and the gate. A **refusal** entry cannot: no run
row exists, so per AC-OFFICE-BACKPRESSURE-003.1 it names the gate, the agent profile
the wake was for, the wake reason, and the causing run identifier the request supplied,
and it adds the causation identifier only when causation had already been resolved —
which it has not been for `workspace_missing`, refused at enqueue step 3, one step
before causation is resolved. A field unavailable at the point of refusal is omitted
rather than filled with an identifier the system never assigned.

The causation identifier is added to the existing run lifecycle events appended by
`AppendRunEvent`, so the run detail timeline can link a run to its parent without a
separate query.

## Related decisions

- [ADR 0005 — agent model unification](../../../decisions/0005-agent-model-unification.md),
  which defines `agent_profiles.max_concurrent_sessions`.
- [ADR 0018 — runtime settings overrides](../../../decisions/0018-runtime-settings-overrides.md),
  which governs how the operator settings above are resolved.
- [ADR 0027 — replayable schema migrations](../../../decisions/0027-replayable-schema-migrations.md),
  which governs the additive migration above.

Two new ADRs are expected during planning: one recording why the instance is the
authoritative ceiling scope and why the launch-count budget fails closed while the
cost budget fails open, and one recording the consolidation of four enqueue paths
onto a single authoritative API, since that is a boundary change other subsystems
will need to respect.
