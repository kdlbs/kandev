---
status: draft
system: office
requirements:
  - REQ-OFFICE-SEAT-PROVENANCE-001
  - REQ-OFFICE-SEAT-PROVENANCE-002
  - REQ-OFFICE-SEAT-PROVENANCE-003
  - REQ-OFFICE-SEAT-PROVENANCE-004
  - REQ-OFFICE-SEAT-PROVENANCE-005
  - REQ-OFFICE-SEAT-PROVENANCE-006
---

# Office Participant Seat Provenance System Design

## Purpose and boundaries

`participant-seat-provenance.md` contracts what a seat remembers about its
origin and what a second writer may do to what the first left behind. Most of
that contract is already satisfied by shipped code: the `provenance` column,
its idempotent migration, automatic casting stamping `auto`, and manual
registration claiming a single undecided `auto` seat in place.

This design exists for the part that is not, and for one reason above the
others. `REQ-OFFICE-SEAT-PROVENANCE-004` requires the two writers to serialize
on **a shared exclusion**. A requirements document should not name a locking
primitive, but "shared" is only real if both writers derive the **same key
bytes**. Two exclusions, each correctly naming the task and the role, each
acquired faithfully, that never contend with one another, satisfy every word of
the requirement and close nothing. That failure is invisible on the embedded
dialect, where a single-writer pool serializes the writers for free, so it
would pass the entire test suite and fail only on the server dialect.

Pinning that derivation is this document's primary job. It also fixes the
linearization point the requirement leaves to the design, names the two
signature changes the observability and session criteria imply, and settles
what happens to the run the displaced agent had already been given.

It does **not** change automatic casting's choice of agent; that is
`review-participant-seats.md` and its design, unchanged here.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| `REQ-OFFICE-SEAT-PROVENANCE-001` | *Persistence*, *Read model boundary* |
| `REQ-OFFICE-SEAT-PROVENANCE-002` | *Components*, *Control flow* |
| `REQ-OFFICE-SEAT-PROVENANCE-003` | *Control flow* |
| `REQ-OFFICE-SEAT-PROVENANCE-004` | *The shared exclusion*, *Contention* |
| `REQ-OFFICE-SEAT-PROVENANCE-005` | *Failure and recovery*, *The displaced run* |
| `REQ-OFFICE-SEAT-PROVENANCE-006` | *Control flow*, *The displaced run* |

## The shared exclusion

### What exists today

Automatic casting locks. `ensureRoleSeatTx` in
`internal/workflow/repository/phase2_sqlite.go` builds, on the server dialect
only:

```text
strings.Join([]string{participantsLockNamespace, workflowID, taskID, role}, "|")
```

with `participantsLockNamespace = "workflow-participant-role-seat:"`, and
acquires `pg_advisory_xact_lock(hashtextextended(key, 0))` as the transaction's
first statement.

Manual registration does not lock at all. `AddTaskParticipant` in
`internal/office/repository/sqlite/participants.go` opens its transaction and
goes straight to the existence probe.

Two facts make a naive fix wrong rather than merely incomplete.
`participantsLockNamespace` is **unexported**, and `internal/office/` holds no
reference to it. And `AddTaskParticipant` never resolves a workflow identifier
— it resolves only the step. An office-side lock keyed on the task and the role
would read as a faithful reading of `AC-OFFICE-SEAT-PROVENANCE-004.2`, hash to
a different value than the caster's four-part key, and serialize nothing.

### Decision: one exported derivation, keyed on task and role

Both writers call one exported function in `internal/workflow/repository`,
which is the package that owns the seat store:

```text
ParticipantRoleSeatLockKey(taskID, role string) string
```

Two properties matter, and they are separate decisions.

**It is a function, not an exported constant.** Exporting the namespace alone
would leave each caller to reassemble the key, and a drift in separator,
field order or the namespace's trailing colon produces two keys that look
identical in review and hash apart. One derivation admits no drift.

**The workflow identifier leaves the key.** The caster's current key carries
it; the new key does not. This is a deliberate narrowing of inputs and a
widening of the exclusion, taken for one reason: it removes a circular
dependency that has no clean resolution.

The registration path cannot know the workflow before it reads the task's
step, and `AC-OFFICE-SEAT-PROVENANCE-004.10` requires the step to be resolved
*inside* the exclusion. Keeping the workflow in the key would force a
provisional read before the lock to derive the key, then a re-read inside it to
satisfy 004.10, then a comparison and a retry when the two disagree — inventing
exactly the retry that `AC-OFFICE-SEAT-PROVENANCE-004.5` says no caller shall
be required to make. Keying on the task and the role removes it: both values
are caller-supplied parameters, so the key is derivable before any read, and
every read can then happen under the lock.

Widening the caster's exclusion this way is safe in the only direction that
matters. A task belongs to one workflow at a time, so every pair of writers
that contended under `(workflow, task, role)` still contends under
`(task, role)`; the change only adds contention, never removes it. It also
covers the case the narrower key misses, a task whose seats span two workflows
after a move. `AC-OFFICE-SEAT-PROVENANCE-004.2` requires an exclusion covering
**at least** the task and the participant role, which this is exactly.

The four-part key exists only in code. `review-participant-seats-01.md`
describes no lock and no key, and no acceptance criterion in
`review-participant-seats.md` names one; `AC-OFFICE-REVIEW-SEATS-001.4`
requires only that concurrent castings converge, which a coarser exclusion
preserves. No sibling document changes on this work.

### Per dialect

On the **server dialect**, both writers acquire
`pg_advisory_xact_lock(hashtextextended(ParticipantRoleSeatLockKey(...), 0))`
as the first statement of their transaction. Transaction-scoped, so it releases
on commit or rollback with no unlock path to leak.

On the **embedded dialect**, neither writer emits a lock statement, exactly as
the caster does not today. The writer pool is pinned to a single connection and
the engine admits one writer database-wide, which is a coarser shared exclusion
that still covers the task and the role. `AC-OFFICE-SEAT-PROVENANCE-004.6` is
satisfied because the criterion is about the outcome, not the primitive.

This asymmetry is the reason the *Testing* section below exists. The embedded
dialect gives the right answer even when the key derivation is wrong, so a
green suite is not evidence the exclusion is shared.

## Control flow

Manual registration, in order. Each step names the criterion it serves.

1. Reject an empty agent profile identifier and any role other than `reviewer`
   or `approver`, at the surface, before the store is reached
   (`AC-OFFICE-SEAT-PROVENANCE-005.1`, `-005.7`).
2. Derive the lock key from the task and the role. No read precedes this.
3. Begin the transaction. Acquire the exclusion as its first statement.
4. **Inside** the transaction and under the lock, resolve the task's current
   step, using the transaction handle rather than the read-only pool
   (`AC-OFFICE-SEAT-PROVENANCE-004.10`). A step resolved earlier is not used.
   Reading through the read-only pool would escape the serialized writer on the
   embedded dialect as surely as a mismatched key escapes it on the server
   dialect.
5. When the task has no step, or does not exist, commit having written nothing
   and return success (`AC-OFFICE-SEAT-PROVENANCE-005.2`, `-005.6`).
6. Probe for a seat at the exact identity `(step, task, role, agent profile)`.
   On a hit, branch on that seat's provenance
   (`AC-OFFICE-SEAT-PROVENANCE-002.3`). When it is the slate's sole `auto` seat
   and no decision stands against it, promote its provenance to `manual` in
   place — the agent profile and every other column stay as they are — then
   commit. Otherwise write nothing. Either way report *no change*. This
   precedes the claim search so that re-registering an agent that already holds
   a seat never consumes a *different* seat (`-002.4`). *No change* is silent
   on both branches: the service layer writes no activity entry and publishes
   no task-changed notification for it.

   The promotion is why `-002.3` is not merely an idempotency guard. An
   operator who names the agent casting already chose has stated a preference,
   and leaving that seat `auto` would keep it claimable, so a later
   registration naming a different agent would silently replace the operator's
   confirmed choice instead of adding a second seat — the same two operator
   actions yielding one seat here and the two `-002.8` promises when no seat
   had been cast. Promoting in place removes that divergence. It is deliberately
   **not** a claim: no agent leaves a seat, so `-002.9`'s activity entry,
   `-002.12`'s session termination and `-002.13`'s notification do not apply,
   and `-001.7` names this promotion alongside the claim as the only writers of
   `provenance`.
7. Confirm the named agent profile exists. On a miss, skip the claim search and
   go straight to the insert branch, so an unknown agent can never displace a
   cast seat (`AC-OFFICE-SEAT-PROVENANCE-005.8`). This position is load-bearing:
   after the identity probe, before any claim.
8. Search the role slate **at that step only** for `auto` seats
   (`AC-OFFICE-SEAT-PROVENANCE-003.1`, `-003.3`). Claim when exactly one exists
   and it has no decision against it; the count alone decides, with no ordering
   or tiebreak (`-002.1`, `-002.5`, `-002.6`). A claim updates the agent profile
   and provenance in place, leaving identifier, position, creation time and the
   decision-required flag untouched (`-002.2`).
9. Otherwise insert a new `manual` seat (`-002.7`, `-002.8`).
10. Commit, then report to the service layer: the outcome, the step acted at,
    and the displaced agent profile when a claim happened.

Automatic casting's flow is unchanged apart from the key it locks on. Its
existence check stays workflow-scoped (`AC-OFFICE-SEAT-PROVENANCE-003.2`).

## Components

### The registration writer reports its outcome

`AddTaskParticipant` returns only `error` today
(`internal/office/dashboard/service.go:95`), which discards every fact the
service layer now needs before it can act on them.

It returns a small result value instead: which of *claimed*, *inserted* or
*unchanged* happened; the identifier of the step the write landed at; and, when
a claim happened, the displaced agent profile identifier. Nothing else — the
seat itself is not returned, because no caller needs it and returning it would
invite a second read model.

**The step is carried, not re-read.** `AC-OFFICE-SEAT-PROVENANCE-002.9` requires
the claim's activity entry to name the step, and
`AC-OFFICE-SEAT-PROVENANCE-006.6`'s cancellation selector needs it too, because
a run's task and step live in payload JSON rather than in columns. The step is
resolved inside the transaction and under the exclusion (*Control flow* step 4),
so returning it is the only way the service layer can name the step the write
actually landed at. Re-reading the task's current step after the commit is
forbidden: that is the stale-step hazard `AC-OFFICE-SEAT-PROVENANCE-004.10`
exists to prevent, and a task that moved between commit and re-read would stamp
the wrong step on the activity entry and point the cancellation selector at a
step whose run does not exist.

Two boundaries of that value, so neither is invented. *Unchanged* covers the
identity-probe hit of `AC-OFFICE-SEAT-PROVENANCE-002.3` on **both** its
branches — the promotion included — and the write-nothing path of `-005.2` and
`-005.6`, where the task stands at no step or does not exist; the step
identifier is then empty and, as with any *unchanged*, the service layer does
nothing at all. The displaced agent profile is empty on every outcome except
*claimed*.

The promotion deliberately does not earn a fourth outcome. It writes a column
inside the transaction but produces no service-layer effect, and every
consumer of the result value is a post-commit side effect that `-002.3` denies
it. Reporting it distinctly would invite a caller to act on it and reopen the
gap `-002.3` closes.

The service layer consumes the result value for four things. They are
independent of one another and **no order among them is contracted** — each
observes a slate the transaction has already committed.

**The activity entry** (`AC-OFFICE-SEAT-PROVENANCE-002.9`). A claim writes a
task activity entry distinct from the one a plain registration writes, carrying
the task, step, role, displaced agent profile and claiming agent profile. The
criterion is explicit that an ops-only log line does not satisfy it, so this is
the product-facing task activity log, not the structured-warning-and-counter
idiom `AC-OFFICE-REVIEW-SEATS-004.8` uses. Both idioms exist in this system and
they are not interchangeable; this one is operator-visible by contract.

**The displaced session** (`AC-OFFICE-SEAT-PROVENANCE-002.12`). Session
termination runs today only on the removal branch. A claim removes an agent
from a role as surely as a removal does, so it reuses that same termination
path for the displaced agent profile, leaving the task's live participant
indicators consistent with the slate.

**The displaced run** (`AC-OFFICE-SEAT-PROVENANCE-006.6`). It needs the
displaced agent profile and the step; *The displaced run* below settles the
rest.

**The silent no-change branch.** The task-changed notification (`-002.13`)
already fires on the claim and insert branches and needs no change there. It
must stop firing on the *no change* branch: `-002.3` now makes that branch
silent, in the activity log and on the wire alike. Today
`addOrRemoveParticipant`
(`internal/office/dashboard/service_tasks.go:342`) publishes and logs
unconditionally on the add path, at `:377-378`, so this is a real behaviour
change.

## The displaced run

A claim arrives after the fan-out has already run, and the fan-out addresses a
snapshot.

`config/workflows/office-default.yml` orders the gated step's `on_enter` as
`clear_decisions`, then `ensure_participant_seat`, then
`queue_run_for_each_participant`, all synchronously in one entry sequence.
`QueueRunCallback.Execute` copies the resolved agent identifier into
`QueueRunRequest.AgentProfileID`; it does not hold a reference to the seat row.
So by the time an operator registers and the claim reassigns the seat, a run
addressed to the agent about to be displaced already exists, and reassigning the
seat does not redirect it. `AC-OFFICE-SEAT-PROVENANCE-002.12` does not reach
this: it ends a *live session*, and a queued run is not one.

`AC-OFFICE-SEAT-PROVENANCE-006.6` therefore requires the claim to cancel it.
"Runnable", in `-006.3`, is exactly the two non-terminal run statuses this
system has: a run is `queued`, then `claimed`, then terminal. There is no
separate `running` state to account for.

The mechanism exists. `CancelRunsWhere` in the runs repository is the single
writer of the terminal cancel state, and it already guards every write with
`status IN ('queued', 'claimed')` — precisely the window in which cancelling is
correct. Callers add a selector; `CancelRunsForTasks` in
`internal/office/repository/sqlite/tree_holds.go` is the existing thin-selector
example to copy. A run that already finished is outside the guard and is left
with its recorded outcome, which is what `-006.6` requires.

Two things about the selector.

`agent_profile_id` is a real column on `runs`, so the displaced agent is matched
directly. The task and step are **not** columns: `runs/service` writes them into
the run's `payload` JSON, so the selector must extract them. That is the dialect
trap `tree_holds.go` already documents for `CancelRunsForTasks` — `json_extract`
is SQLite-flavoured and the server dialect needs `dialect.JSONExtract`, which is
why the selector stays in the office repository rather than in the shared
writer. `AC-OFFICE-SEAT-PROVENANCE-004.6` applies here as everywhere.

Cancelling cannot retract a decision that was already recorded, and does not
need to: `AC-OFFICE-SEAT-PROVENANCE-002.5` forbids claiming a decided seat at
all, so a seat whose agent has finished deciding is never displaced.

### When the cancellation fails

The cancellation runs after the claim has committed (*Control flow* step 10), so
it cannot be rolled back into the claim's transaction. It is **best-effort: log
a warning and continue, returning success to the caller.** It does not fail the
registration, and it is not retried.

`AC-OFFICE-SEAT-PROVENANCE-006.6` now contracts exactly this, so the position is
the requirement's rather than the design's: a failed cancellation still returns
success, the failure is recorded, and one run may remain runnable for the
displaced agent profile. `-006.3` defers to `-006.6` for the same reason, rather
than asserting an absolute the failure path cannot honour. Nothing later sweeps
that run up — re-entering the step re-queues against the current slate, it does
not cancel a stale run — so the residual is real and bounded, not a deferred
repair.

Cancelling nothing is not a failure. The selector matches no run whenever the
fan-out queued none for that agent — a seat registered outside a step entry is
the ordinary case — and zero rows cancelled is a success like any other.

That is the established idiom for this writer, not a new one. Both existing
`CancelRunsForTasks` callers warn and continue
(`internal/office/service/tree_controls.go:58-60` and `:102-104`), as does the
removal branch's session termination
(`internal/office/dashboard/service_tasks.go:369-375`) that
`AC-OFFICE-SEAT-PROVENANCE-002.12` reuses.

`AC-OFFICE-SEAT-PROVENANCE-005.5` is not violated. Its guarantee is scoped to
the role slate, which the claim committed correctly and which a failed
cancellation does not touch. The residual is one run left runnable for an agent
no longer seated in that role, and the alternative is worse: returning an error
against a slate that did change is the outcome `-005.5` exists to prevent.

### What this does not do

It does not wake the claiming agent, and no criterion here asks it to. That
entry's fan-out has already read the slate, and `review-participant-seats.md`
owns the consequence: `AC-OFFICE-REVIEW-SEATS-001.1` fixes the fan-out as the
point the slate is read, `AC-OFFICE-REVIEW-SEATS-003.5` owns waking a seat the
manual path wrote, and that document's *Out of scope* records the recovery in
its own words — a seat added after the entry sequence "wakes nobody", and "such
a task is recovered by re-entering the step."

This is a real operator-visible consequence and it is deliberate, not an
oversight: after a claim, the registered agent is woken when the step is next
entered, not retroactively. Building a wake into the claim branch here would
also leave the plain-insert branch — a registration where no `auto` seat existed
— behaving differently for no principled reason. That asymmetry is a defect in
the registration surface as a whole, not in this contract, and belongs to
`review-participant-seats.md` or a follow-up.

`REQ-OFFICE-SEAT-PROVENANCE-006`'s other criteria do not depend on it: the
quorum count, the seat the guard names, and the participant listing are all
satisfied by the seat mechanics the moment the claim commits.

## Read model boundary

`AC-OFFICE-SEAT-PROVENANCE-001.5` requires provenance to be readable as a typed
value and forbids inferring it, while ending with "no participant API response
carries provenance". `Participant` in the Office repository is simultaneously
the domain struct and, through its json tags, the API projection, which makes
the boundary ambiguous unless it is named.

It is named here. The typed value lives on the workflow model
`WorkflowStepParticipant.Provenance`, which is the seat store's own read shape
and already carries it. Office's `Participant` is the API projection and
**does not gain a provenance field**. Office code that needs provenance queries
the column explicitly, as the claim search already does, rather than inferring
it from position, identifier or creation time.

`REQ-OFFICE-SEAT-PROVENANCE-006` is what the operator sees: one seat, one
decision, naming the registered agent. The mechanism stays behind the API.

## Contention

No retry loop is added to registration.

Contention on the exclusion is resolved by waiting on it, which is the whole
point of blocking rather than repairing, and matches the prior-art position the
requirement cites. A caller that waits and then proceeds observes success, so
`AC-OFFICE-SEAT-PROVENANCE-004.5` holds without a retry.

Two residual cases are handled without one:

- `AC-OFFICE-SEAT-PROVENANCE-004.4` is satisfied by the identity probe, not by
  a caught constraint violation. Both registrations serialize on the exclusion,
  so the second probes only after the first has committed, sees its row, and
  reports *no change* without reaching its insert. The unique index remains as
  a defensive backstop against a writer outside the exclusion; were it ever to
  fire, the violation is treated as *no change* rather than retried. No test
  should need to reach that path.
- A seat selected for claim can be removed before the claim applies. The
  update then matches no row, and the registration falls through to inserting
  its own `manual` seat rather than completing having written nothing
  (`AC-OFFICE-SEAT-PROVENANCE-004.8`).

Automatic casting keeps its existing bounded retry on the natural-key
violation as a backstop, unchanged.

A wait that exceeds the store's own timeout is a store failure surfacing under
`AC-OFFICE-SEAT-PROVENANCE-005.5`, not a race outcome, which is what the
criterion's second sentence separates.

## Failure and recovery

Every write above happens in one transaction, so a failure at any point leaves
the role slate as it was found: no half-applied claim, and no seat whose
provenance moved without its agent profile moving
(`AC-OFFICE-SEAT-PROVENANCE-005.5`).

A failure reading the decision store while a claim is being evaluated aborts
the transaction and surfaces. It does not fall through to writing a second
seat, which would be the one outcome worse than failing
(`AC-OFFICE-SEAT-PROVENANCE-005.4`).

A registration naming an agent profile that does not exist claims nothing and
displaces nothing (`AC-OFFICE-SEAT-PROVENANCE-005.8`). Existence is checked
before the claim search, and a miss skips straight to the insert branch, which
preserves the surface's shipped silence on unknown identifiers while denying an
unknown agent the ability to take over a cast seat and leave a slate the
fan-out cannot wake.

A seat whose stored provenance is neither `auto` nor `manual` is not claimable,
because the claim search filters on `auto` positively, and reading it does not
fail (`AC-OFFICE-SEAT-PROVENANCE-005.3`).

## Persistence

The column and its idempotent migration already exist and are unchanged:
`provenance TEXT NOT NULL DEFAULT 'manual'` in the table definition, plus an
`ADD COLUMN` migration applied through the same guarded path.

The `NOT NULL DEFAULT 'manual'` pair is what makes
`AC-OFFICE-SEAT-PROVENANCE-001.3` and `-001.4` structural rather than a
convention a future writer can forget: a writer that states no provenance gets
`manual`, and every row that predates the column reads `manual` after upgrade,
so no pre-existing seat becomes claimable by the upgrade alone. Applying the
migration twice is safe, and it preserves each existing seat's agent profile,
decision-required flag and position (`-001.6`).

Two statements write `provenance` and both write only `manual` (`-001.7`): the
claim, and `-002.3`'s in-place promotion of an `auto` seat the named agent
already holds. Nothing writes `auto` outside automatic casting's own insert.

## Testing

The suite must be able to fail. Three things it would otherwise miss:

- **The key derivation.** A test that both writers produce byte-identical keys
  for the same task and role belongs at the unit level, because no
  embedded-dialect integration test can distinguish a shared exclusion from two
  private ones.
- **The concurrency criteria.** `REQ-OFFICE-SEAT-PROVENANCE-004` is about
  interleaving. Ordering two calls in a test proves the sequential paths, which
  are already green, and proves nothing here.
- **The promotion branch, and the property it exists for.** A test that
  registering the agent that already holds the sole undecided `auto` seat leaves
  the seat count at one and flips its provenance to `manual` is necessary but
  weak on its own — it passes against an implementation that promotes the wrong
  seat. The assertion that catches the defect is the convergence one: from a
  slate holding one `auto` seat for agent A, registering A then B, and
  registering B then A, must both end at **two `manual` seats naming A and B**.
  Before the promotion exists the first ordering ends at one seat, so that pair
  fails loudly on a regression and is the direct test of
  `AC-OFFICE-SEAT-PROVENANCE-002.3` against `-002.8`.
- **The end-to-end outcome.** `REQ-OFFICE-SEAT-PROVENANCE-006` is assertable at
  the API: the quorum projection exposes the required count, and the
  participant listing exposes the slate. An existing quorum-transition
  end-to-end test already walks auto-first then manual-second, but asserts on
  the quorum's role rather than its seat count, so it would not catch a
  regression to two seats until the guard stalled. Asserting the count directly
  is what closes that gap. Its registration deliberately happens after the step
  move, because seats bind to the task's current step; that ordering is the
  scenario and is not a defect to correct.

## Related decisions

- `review-participant-seats.md` and `review-participant-seats-01.md` own
  automatic casting's choice of agent. Only the lock key changes there.
- `step-entry-sequence-execution.md` owns whether a step's entry actions run at
  all. If they do not, none of this is reachable.
