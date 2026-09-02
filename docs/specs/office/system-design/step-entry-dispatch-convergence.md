---
status: draft
system: office
created: 2026-09-02
owners:
  - kandev
requirements:
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-002
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-003
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-004
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-005
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-006
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-007
  - REQ-OFFICE-STEP-ENTRY-DISPATCH-008
---

# Step Entry Dispatch Convergence System Design

Implements
[`step-entry-dispatch-convergence.md`](../requirements/step-entry-dispatch-convergence.md)
and the three requirement files separated from it, each of which governs more
than convergence and is stated once on its own:
[`step-entry-recovery-scan.md`](../requirements/step-entry-recovery-scan.md)
(REQ-004),
[`step-entry-run-coalescing.md`](../requirements/step-entry-run-coalescing.md)
(REQ-005), and
[`step-entry-fanout-reporting.md`](../requirements/step-entry-fanout-reporting.md)
(REQ-007). Builds on
[`step-entry-sequence-execution.md`](../system-design/step-entry-sequence-execution.md)
and the legacy
[`workflow-on-enter-action-dispatch`](../../workflow-on-enter-action-dispatch/spec.md)
spec, which is not edited.

## Current shape

Two dispatchers execute a step's declared entry sequence for one arrival.

| | Ledger dispatcher | Marker dispatcher |
|---|---|---|
| Entry point | `Repository.dispatchStepEntry` after each writer's commit | `processOnEnter` -> `dispatchOnEnterActions` |
| Executor | `Engine.DispatchStepEntry` | `dispatchEngineOwnedOnEnterAction` |
| Kind list | `sessionIndependentActionKinds` | `case OnEnterClearDecisions, OnEnterQueueRunForEachParticipant` |
| Entry identity | `task_step_transitions` row id (string) | `workflow_step_entries` row id (int64) |
| Idempotency key input | `ActionInput.EntryID` | `ActionInput.OperationID` = `step_entry:<id>:<pos>` |
| Claim | none | `ClaimStepEntryMarker` compare-and-set |
| Route coverage | every registered step-transition writer | `on_turn_complete` only (`finalizeStepEnter` passes `entryID=0`) |

`clear_decisions` and `queue_run_for_each_participant` appear in both kind
lists. On the `on_turn_complete` route both dispatchers run for the same
commit: `updateTaskWithWorkflowStepAdmission` allocates the
`workflow_step_entries` row, writes the ledger row, commits, then calls
`dispatchStepEntry`; `applyEngineTransition` then reads the allocated entry id
back through the result holder and calls `launchProcessOnEnter`. Because
`idempotencyKey` prefers `EntryID` and falls back to `OperationID`, the two
paths produce different keys for the same logical action, so neither the
idempotency window nor coalescing collapses the second run.

Three kinds were already reconciled - `queue_run`, `run_code_review`,
`ensure_participant_seat` are dispatched only by the ledger path, with a
comment in `dispatchOnEnterActions` saying so. The two kinds above were not.

## Verified state

Measured on `feature/fix-on-enter-dispatc-77a` at `646ff0063`, 2026-09-02.
These correct the inherited finding list, which predates the ledger dispatcher.
Already satisfied; **no requirement below restates them**:

- The `workflow-on-enter-action-dispatch` AC-F1 stop rule is implemented:
  `dispatchEngineOwnedOnEnterAction` returns `abandon` for a prior
  `in_progress` marker and `dispatchOnEnterActions` breaks its loop.
- The single-session duplicate-turn race no longer reproduces: five runs of
  `go test -race -count=3 -run 'TestProcessOnEnter_|TestProcessOnTurnComplete_Concurrent'
  ./internal/orchestrator/` passed (previously 2 of 7 failed). A third signal,
  `turnCompletionConsumedGeneration`, closed it.
- Participant de-duplication on `(role, agent_profile_id)` is implemented in
  `roleSeatsForFanOut`.
- Fan-out failure isolation is collect-and-continue, not abort-on-first-error.

## Decision: the ledger dispatcher is the single owner

The ledger path becomes the owning dispatcher for every session-independent
kind, including the two currently shared. The marker path stops executing them.

Chosen because the ledger path already satisfies AC-OFFICE-STEP-ENTRY-001.1 on
every route, while the marker path reaches only `on_turn_complete`. Moving
ownership the other way would mean re-wiring the four remaining routes onto the
result-holder mechanism, which is the work
`step-entry-sequence-execution.md` already did once.

The cost is that the ledger path holds no marker, so it does not satisfy
AC-OFFICE-STEP-ENTRY-DISPATCH-002.4 as it stands. The marker machinery is
therefore **kept and moved**, not deleted: `workflow_step_entries`,
`workflow_step_entry_markers`, `ClaimStepEntryMarker`,
`CompleteStepEntryMarker`, `GetStepEntryMarkerState`, and
`ClearStepDecisionsAndCompleteMarker` stay, and the ledger dispatcher claims
through them before executing a kind that requires durability.

This keeps one entry identity **where there is one**. For a step declaring at
least one marker-bearing kind, the `workflow_step_entries` row is the entry
identity for markers and for idempotency keys on every route, and the ledger row
id is not used as a key input. For a step declaring none, no entry is allocated
and its runs keep deriving their key from the
step-transition row id as they do today - the `work` step's
`auto_start_agent`-only sequence is the shipped example. `idempotencyKey`
therefore keeps both branches rather than collapsing to `EntryID`, and which
branch applies is read from the ownership declaration's marker-bearing column,
not from whether an entry row happens to exist
(AC-OFFICE-STEP-ENTRY-DISPATCH-002.8). Allocation already happens inside the
transition transaction whenever a result holder is attached; it becomes
unconditional for a step declaring at least one marker-bearing kind.

### Ownership declaration

One exported table in `internal/workflow/stepentry` maps action kind to two
independent properties: its **owning dispatcher**, and whether it is
**marker-bearing**. Both `Engine.DispatchStepEntry` and
`dispatchOnEnterActions` read the
table; neither keeps a private list. This satisfies
AC-OFFICE-STEP-ENTRY-DISPATCH-002.1, and it is what makes 002.3's silent skip
safe - a kind with an owner elsewhere is not a discard.

**Each column has its own seed, and they are different functions.** The
ownership column is seeded from `sessionIndependentActionKinds` (five kinds);
the marker-bearing column is seeded from `IsEngineOwnedOnEnter` (two kinds).
`IsEngineOwnedOnEnter` reads like an ownership predicate because of its name,
but its doc comment and body are explicit that it reports what the **marker**
system dispatches, and it returns true for exactly `clear_decisions` and
`queue_run_for_each_participant`. Seeding ownership from it would silently drop
`queue_run`, `run_code_review` and `ensure_participant_seat`; seeding
marker-bearing from `sessionIndependentActionKinds` would promise a marker for
three kinds that have never carried one. Either substitution inverts the table.

This is the settled membership, and it is the set every criterion quantifying
over marker-bearing positions reads:

| Action kind | Owning dispatcher | Marker-bearing |
| --- | --- | --- |
| `clear_decisions` | ledger | **yes** |
| `queue_run_for_each_participant` | ledger | **yes** |
| `queue_run` | ledger | no |
| `run_code_review` | ledger | no |
| `ensure_participant_seat` | ledger | no |
| `enable_plan_mode` | marker | no |
| `auto_start_agent` | marker | no |
| `reset_agent_context` | marker | no |
| `set_session_mode` | marker | no |
| `configure_session` | marker | no |

`configure_session` is the tenth row, produced by neither seed and written in by
hand. `CompileOnEnterAction` has no case for it, so the engine has
no `ActionKind` for it and `DispatchStepEntry` can never reach it; only
`processOnEnter` executes it, inline before `dispatchOnEnterActions`. It is
marker-owned and not marker-bearing, like the other four session-shaped kinds.
**Do not express that by adding it to `sessionShapedActionKinds`**:
`entrydispatch_test.go` asserts both engine maps list only kinds
`CompileOnEnterAction` emits, so that edit fails the suite. The ownership table
is wider than either engine map: AC-OFFICE-STEP-ENTRY-DISPATCH-002.1 classifies
every kind *either* dispatcher can reach, not every kind the engine compiles.

Marker-bearing stays at exactly the two kinds that carry a marker today. This
convergence moves ownership; it does not extend durability to kinds that never
had it, which would be new work and is not required by
AC-OFFICE-STEP-ENTRY-DISPATCH-002.4. The three ledger-owned kinds that are not
marker-bearing keep deriving their idempotency key from the step-transition row
id exactly as they do now, including inside a step that allocates an entry for
some other position.

The two columns must stay separate because they genuinely differ. The shipped
`office-default` Review and Approval steps declare
`[clear_decisions, ensure_participant_seat, queue_run_for_each_participant]`:
after convergence the ledger dispatcher owns all three, but
`ensure_participant_seat` is executed without a marker. Reading marker-bearing
off the ownership column would make position 1 of every production entry a
position that can never hold a marker, and every rule quantifying over "the
entry's positions" would then be evaluating a set it can never satisfy.

### Ordering within and across owners

Within one owner, the dispatcher walks the declared sequence in position order
and skips kinds it does not own, so relative order among the kinds it does own
is preserved - this is AC-OFFICE-STEP-ENTRY-DISPATCH-002.7, and it is what
guarantees a declared `clear_decisions` commits its delete before the fan-out
that follows it, both being ledger-owned after convergence.

**That ordering alone is not enough, and the stop rule has to move with the
ownership.** `Engine.DispatchStepEntry` is record-and-continue by contract - its
doc comment cites AC-OFFICE-STEP-ENTRY-001.4 and .6 - while the marker path
being retired breaks its loop on a failed or claim-lost action, which is AC-F1.
Verified state lists AC-F1 as already satisfied, but it is satisfied *on the
path convergence retires*, so moving ownership silently drops it. On the shipped
Review and Approval steps `clear_decisions` is position 0 and the fan-out is
position 2: without the stop rule a failed or claim-lost decision clear would be
recorded and the fan-out would still run, waking reviewers against decisions
that were never cleared, and quorum could then count a stale decision beside a
new one. That is the precise harm REQ-OFFICE-STEP-ENTRY-DISPATCH-002's Overview
exists to remove, so the ledger dispatcher abandons the remainder of an entry's
sequence when a marker-bearing position fails or loses its claim
(AC-OFFICE-STEP-ENTRY-DISPATCH-002.10). AC-OFFICE-STEP-ENTRY-001.4 admits this:
it continues-on-error "except where another acceptance criterion states
otherwise". The narrowing is deliberate and minimal - it is scoped to
marker-bearing positions, so a failing `ensure_participant_seat` still lets the
rest of the sequence run, exactly as record-and-continue intends.

Across owners there is no ordering guarantee and none is added. The ledger
dispatcher runs synchronously after `tx.Commit()`; the marker dispatcher runs on
the goroutine `launchProcessOnEnter` starts afterwards. Interleaving them would
mean making one wait on the other - either blocking the transition writer on a
goroutine or moving the marker path back inline - and both are larger changes
than this design carries. No shipped workflow declares a step whose entry
sequence mixes owners, so nothing in the tree depends on the guarantee that is
absent; the requirement's Out of scope states this as a known limit rather than
an oversight.

## Task-scoped turn completion

`turnCompletionLocks` is re-keyed from session id to task id, and so is the
staleness decision. `turnCompletionConsumedGeneration` **stays session-keyed** -
it is not re-keyed, and it is not deleted; the paragraph below states why, and
AC-OFFICE-STEP-ENTRY-DISPATCH-003.2 requires exactly that split.
`acquireTurnCompletionCriticalSection` takes the task id it already has in
scope.

The generation guard's meaning changes with the key and must be restated:
keyed by session, it compared the caller's session snapshot `UpdatedAt` against
the last consumed one for that session. Keyed by task, a snapshot from session
B is not comparable to one recorded for session A. The guard therefore stores
the winning transition's identity for the task - the destination step id and
the commit time - and a caller is rejected when its own observed step already
differs from the task's current step. The step comparison already in the
critical section carries this; the generation map's role narrows to rejecting a
redelivery of the same session snapshot, which stays session-keyed **inside** a
task-keyed critical section rather than replacing it.

AC-OFFICE-STEP-ENTRY-DISPATCH-003.2 is satisfied by that split, and the AC now
states it explicitly: the *staleness* guard - the one deciding whether a
caller's observed step is still current - is task-keyed, and the retained
session-keyed map is the narrower *redelivery* check, which the criterion does
not govern. Both are required; neither substitutes for the other. Deleting
`turnCompletionConsumedGeneration` would lose same-session redelivery
de-duplication and is not what 003.2 asks for.

Both maps are in-process (`sync.Map`), so REQ-OFFICE-STEP-ENTRY-DISPATCH-003's
guarantee is scoped to one backend process, which the requirement's Out of
scope now states. No advisory or row lock is introduced.

`processOnChildrenCompleted` keeps `lockChildCompletionOperation` for its own
per-operation de-duplication and additionally takes the task lock before
reaching `applyEngineTransition`. Lock order is task lock first, then operation
lock, everywhere, so the two cannot deadlock.

`allocateStepEntryIfPending` computes `entry_seq` as `SELECT COUNT(*)` then
`INSERT`. Under the task lock of
AC-OFFICE-STEP-ENTRY-DISPATCH-003.1 two callers for one task cannot interleave
that read and write, so within a single backend process the race is closed by
serialization, not by the constraint. It is left as is rather than rewritten to
`MAX(entry_seq)+1` because the `UNIQUE(task_id, step_id, entry_seq)` constraint
remains a correct backstop: a lost race surfaces as a failed transition, which
is the behavior AC-OFFICE-STEP-ENTRY-DISPATCH-008.3 asserts and
AC-OFFICE-STEP-ENTRY-DISPATCH-008.4's sibling test at the repository layer
covers. The constraint is defence in depth here, not the primary guarantee.

## Startup recovery

Implements
[`step-entry-recovery-scan.md`](../requirements/step-entry-recovery-scan.md),
which states REQ-OFFICE-STEP-ENTRY-DISPATCH-004 in full. A single scan runs in
backend start, after the store opens and before the engine serves triggers,
alongside the existing startup recovery sweeps. Its selection is a join over
`workflow_step_entries` and its markers alone - no workflow declaration is
loaded, so an entry whose declaration is unreadable is still selected and still
reaches 004.3's `unresolvable` rule. It orders by
`(task_id, step_id, entry_seq)`, applies the skip rules in
AC-OFFICE-STEP-ENTRY-DISPATCH-004.3, and re-dispatches what survives through
the same owning dispatcher a live arrival uses. Loading the declaration is
needed only to re-dispatch a surviving entry; a failure to load it there is
itself the `unresolvable` skip.

Because the scan runs before the engine serves triggers, an `in_progress`
marker cannot belong to a live dispatcher, so it is read as failed with no
lease and no wall-clock threshold. That is what makes the atomic-clear
rollback recoverable: `ClearStepDecisionsAndCompleteMarker` leaving the marker
`in_progress` proves the delete did not commit, and the entry is marked
terminal rather than re-run, per 004.5 and the legacy spec's AC-C2.

Every failure inside the scan is logged and skipped. The scan never blocks
startup (004.2).

### Reclaim: how a stranded position is re-executed

The scan cannot reuse the live-dispatch claim. `ClaimStepEntryMarker` is
INSERT-only: it returns `claimed=false` for **any** existing
`(entry_id, position)` row, whatever its state, and
`CompleteStepEntryMarker` only UPDATEs rows already `in_progress`. A scan built
on those two would abandon every entry it was written to rescue, satisfying
AC-OFFICE-STEP-ENTRY-DISPATCH-008.4 and failing 004.7. The two paths are
therefore distinct, per AC-OFFICE-STEP-ENTRY-DISPATCH-004.8:

- **Live dispatch** keeps `ClaimStepEntryMarker` unchanged. Losing to an
  existing row still means "do not execute", which is what 008.4 asserts.
- **Recovery** adds `ReclaimStepEntryMarker(ctx, entryID, position,
  operationID, claimedAt) (bool, error)`, a state-predicated compare-and-set:
  `UPDATE workflow_step_entry_markers SET state='in_progress', operation_id=?,
  claimed_at=?, error=NULL WHERE entry_id=? AND position=? AND state IN
  ('in_progress','failed')`. It reports `reclaimed` from the affected row
  count, so a `done` or `skipped` row is never taken over and a completed
  position is never executed twice. A position with no row at all is not
  stranded and takes the ordinary `ClaimStepEntryMarker` insert.

Reclaim is safe without a lease only because of the ordering guarantee: the
scan runs before the engine serves triggers, so no in-process dispatcher can
hold the row it supersedes. That ordering is load-bearing, not incidental.

### Where terminality lives

`workflow_step_entries` has no state column and does not gain one: entry
terminality stays derived, per AC-OFFICE-STEP-ENTRY-DISPATCH-004.9, which avoids
an entry-level state that could disagree with its own markers. What it is
derived *from* changes.

It cannot be derived from the positions the step's declaration currently
declares. Two cases break that reading, and both are live:

- A declaration can name a position that is not marker-bearing -
  `ensure_participant_seat` at position 1 of the shipped Review and Approval
  steps - so "every declared position holds a terminal marker" is unsatisfiable
  for every production entry, and the scan would re-select them all at every
  process start, forever.
- `AC-OFFICE-STEP-ENTRY-DISPATCH-004.3` must retire `unresolvable` and
  `digest_changed` entries, whose declaration is respectively unreadable and no
  longer shape-identical. Enumerating positions by re-reading the declaration is
  impossible in the first case and wrong in the second.

The entry row therefore stores its **allocated position set**: the ordered list
of marker-bearing positions, computed once from the ownership declaration at
allocation time and written inside the same transaction as the entry row.

    r.migrate.Apply("workflow_step_entries.marker_positions",
        `ALTER TABLE workflow_step_entries ADD COLUMN marker_positions TEXT NOT NULL DEFAULT ''`)

The value is the comma-separated ascending position list. `""` is the legal
representation of "no marker-bearing positions"; `BuildPendingAllocation`
already declines to allocate in that case, so only a pre-existing row carries
it, and such a row reads as already terminal rather than as stranded work:
those rows predate the scan and have no marker to resume. `BuildPendingAllocation`
already computes this set to decide whether to allocate at all; it now returns
it, and the repository persists it.

A value that does not parse as a position list is not repaired: the entry is
terminal by definition and one ERROR names it, per
AC-OFFICE-STEP-ENTRY-DISPATCH-004.9. There is no position to resume and none to
retire, so any other reading re-selects it at every process start forever - the
same deadlock deriving from the declaration produced.

Terminality is then decidable from the store alone: an entry is non-terminal
exactly while some position in its stored `marker_positions` has no terminal
marker. No workflow declaration is loaded to answer the question, which is what
lets AC-OFFICE-STEP-ENTRY-DISPATCH-004.1 select an `unresolvable` entry at all,
and what lets 004.3 retire it by writing a terminal marker for each position in
that stored set.

The marker states become `in_progress`, `done`, `failed`, and a new
**`skipped`** - terminal, and distinct from both `done` (ran, succeeded) and
`failed` (enqueue attempted, did not succeed). Without the fourth value a
skipped position would have to lie in one direction or the other. 004.3 reads
the two alike only on a `clear_decisions` position, and apart everywhere else.

The reason is stored in a new nullable `skip_reason` column, added the way this
repository adds columns - `r.migrate.Apply` in
`task/repository/sqlite/base_migrations.go`, whose `MigrateLogger` swallows
"duplicate column name" so the call is replay-safe:

    r.migrate.Apply("workflow_step_entry_markers.skip_reason",
        `ALTER TABLE workflow_step_entry_markers ADD COLUMN skip_reason TEXT`)

`CREATE TABLE IF NOT EXISTS` alone would not add it to an existing database, so
the ALTER is required, not optional. Writing a skip needs to work from absent,
`in_progress` and `failed` alike, which neither existing method does, so the
scan uses `SkipStepEntryMarker(ctx, entryID, position, kind, reason, at)`:
an upsert on `(entry_id, position)` setting `state='skipped'` and
`skip_reason=?`, guarded by `WHERE state <> 'done'` on the update branch. The
seven reason values are exactly 004.9's list.

## Coalescing exclusion

Two halves, and only the first is in-memory. `QueueRunRequest` gains
`EntryTriggered bool` in both declarations - `internal/runs/service` and
`internal/workflow/engine`, which must stay identical per
AC-OFFICE-STEP-ENTRY-DISPATCH-005.3 - and the engine sets it for every run
enqueued by a step-entry action. `shouldCoalesceRun` returns false when it is
set, so an entry-triggered run never merges into an existing row.

That alone is not sufficient. `CoalesceRun(ctx, agentInstanceID, reason,
windowSecs, payload)` takes neither the request nor its idempotency key, and
its subquery picks a **pre-existing** queued row as the merge target; the
comment-key exclusion it already has works only because
`idempotency_key` is a persisted column it can read back. A request-scoped
boolean is invisible to that subquery, so the flag must also be persisted on
the run row (AC-OFFICE-STEP-ENTRY-DISPATCH-005.2):

    r.migrate.Apply("runs.entry_triggered",
        `ALTER TABLE runs ADD COLUMN entry_triggered INTEGER NOT NULL DEFAULT 0`)

The `runs` DDL and its ALTER migrations are owned by the office repository
(`office/repository/sqlite/base.go`), which is where `runs.outcome` was added
the same way; `runs/repository/sqlite/base.go` says so explicitly. `DEFAULT 0`
gives every pre-existing row "not entry-triggered", which is the behaviour
005.2 requires and is also the safe direction: an old row stays a legal merge
target.

The insert path writes the column from the request, and `CoalesceRun`'s
subquery gains `AND entry_triggered = 0` so an entry-triggered queued row is
never chosen as a target either. Both directions are needed: the predicate
stops an entry run from being merged *into* something, the SQL stops it from
being merged *onto*.

The idempotency key is unchanged and the task-comment prefix is not reused, so
idempotency suppression is unaffected (005.4).

## Migrated columns are probed before use

Three columns are added by `r.migrate.Apply`:
`workflow_step_entries.marker_positions`,
`workflow_step_entry_markers.skip_reason`, and `runs.entry_triggered`.
`MigrateLogger.Apply` logs a failed `ALTER` at WARNING and returns
(`internal/db/migratelog.go`), so a column can be absent at runtime without
startup having failed. SQL naming a missing column fails the whole statement,
not just the new clause - for `runs.entry_triggered` that means every run
enqueue, not only the exclusion.

Each reader therefore probes with the `columnExists` shape `runs.outcome`
already uses, once at startup, and degrades explicitly rather than issuing the
statement:

| Column | Probe fails -> |
|---|---|
| `runs.entry_triggered` | one ERROR; `CoalesceRun` and the insert path fall back to pre-requirement coalescing, exclusion inactive (AC-...-005.6) |
| `workflow_step_entries.marker_positions` | one ERROR; the startup scan is skipped for that process start, entries left non-terminal for a later start (AC-...-004.11, 004.2) |
| `workflow_step_entry_markers.skip_reason` | shares the scan's probe; the scan cannot record a skip reason without it, so it is skipped under the same rule |

A probe that errors for any other reason is treated as "column absent" and takes
the same degraded path. The probe exists to avoid issuing a statement that
cannot succeed; a probe whose own answer is unknown gives no more license to
issue it than a definite "no" does.

Degrading is chosen over failing startup because both requirements state the
scan and the exclusion as improvements over today's behaviour, not as
preconditions for serving: a backend that starts without them behaves as the
one shipping now does.

## Diagnostics

The warning helper takes workflow id, step id, step name, and action type, and
is called from all four discard points named in
AC-OFFICE-STEP-ENTRY-DISPATCH-006.2. The fan-out callback gains a logger and
emits per-participant ERROR records plus one INFO summary; it currently returns
a joined error and logs nothing itself, which is why the summary cannot be
produced by its caller.

`engineOnEnterCallback` returning a callback that fails with
`ErrActionNotYetWired` is replaced by an unwired check ahead of the claim, so an
unwired adapter produces the warning and no **failed** marker (006.4).

For a **marker-bearing** position the check does not stop there: it writes a
terminal `skipped` marker with reason `adapter_unwired` before returning. This
is required, not cosmetic. Terminality is derived from the allocated position
set (AC-OFFICE-STEP-ENTRY-DISPATCH-004.9), a position with no row at all is not
stranded and takes the ordinary insert on the next pass, and the unwired check
would fire again and again write nothing - so the entry would never become
terminal and the startup scan would re-select it at every process start,
forever. A `skipped` marker is terminal and is not a `failed` marker, so it
satisfies 006.4's prohibition rather than weakening it, and 004.9's reason field
tells an operator the adapter was absent rather than that the work ran. A
non-marker-bearing position keeps the warning-and-no-marker behaviour, because
it holds no position in the allocated set and so cannot strand an entry.

## Enumeration order

`roleSeatsForFanOut` sorts by `Position ASC, AgentProfileID ASC`, and that
order is **kept unchanged**. `AgentProfileID` is already unique across the set
`collapseByRoleAgent` returns, so it is a valid final tiebreak and
AC-OFFICE-STEP-ENTRY-DISPATCH-007.7's unique-last-column rule is met without any
change.

An earlier draft moved the tiebreak to the participant row id; that is
**withdrawn**. `review-participant-seats.md` AC-OFFICE-REVIEW-SEATS-003.3 binds
this same fan-out to ascending `position` then ascending **agent profile
identifier**, it is in force, and nothing here amends it. The swap would have
violated that sibling contract wherever row-creation and agent-profile-id order
diverge, and the existing fixture is lexically pre-sorted so it would not have
caught the change - hence AC-OFFICE-STEP-ENTRY-DISPATCH-007.9's fixture whose
two orders disagree. Role is constant after filtering, so the legacy `role ASC`
leading key is not reintroduced.

## Test seams

- The production-shape regression test needs both dispatchers wired.
  `review_participant_seats_acceptance_test.go` already builds that environment
  and is the place to assert an empty queue after the expected run.
- The atomic-clear test needs a failure injected between delete and marker
  completion. `dispatchClearDecisionsAtomic` only takes the atomic path for a
  concrete `*workflowadapters.DecisionAdapter`, so the seam has to be inside the
  repository method rather than a store double.
- The task-scoped concurrency test needs two sessions on one task, which the
  existing single-session concurrency test does not build.
- The reclaim test must distinguish the two claim paths on the same row:
  `ClaimStepEntryMarker` must still return `claimed=false` against an
  `in_progress` and a `failed` marker (008.4), while
  `ReclaimStepEntryMarker` returns `reclaimed=true` for both and `false` for
  `done` and `skipped`. A test that only exercises the scan end-to-end cannot
  tell a working reclaim from a scan that silently found nothing to do.
- The allocated position set needs a test that would fail if it were recomputed
  from the declaration instead of read back: allocate an entry at a step whose
  sequence declares a non-marker-bearing kind, then assert `marker_positions`
  omits that position and that the entry reaches terminality without it. A test
  over a step whose every kind is marker-bearing cannot tell the two
  implementations apart, and that is the shape the shipped Review step is not.
- Retiring an `unresolvable` entry needs a test that removes or corrupts the
  step declaration and asserts the scan still writes a terminal marker for every
  stored position. Without it a scan that quietly skips such entries looks
  identical to one with nothing to do.
- The `skipped` state needs a test asserting a scan-skipped position is
  readable as neither `done` nor `failed` after restart, since that distinction
  is the whole point of the fourth value.
- The coalescing test must cover the *target* direction, not only the incoming
  one: a queued entry-triggered row must not be selected by `CoalesceRun` for a
  later ordinary request. Asserting only that an entry run does not coalesce
  passes against a service-side-only implementation, which is the half that is
  easy to ship by itself.
- The stop rule (AC-OFFICE-STEP-ENTRY-DISPATCH-002.10) needs a test that fails
  if it is removed: fail `clear_decisions` at position 0 of the shipped Review
  sequence and assert the position-2 fan-out enqueues nothing. Asserting only
  that the failure was recorded passes against a dispatcher that continued,
  which is exactly today's ledger behaviour.
- The unwired-adapter case needs a test that a marker-bearing position reaches
  a terminal `skipped` marker with reason `adapter_unwired`, and that a second
  process start does **not** re-select the entry. Asserting only the warning
  passes against the no-marker implementation that strands the entry forever.
- The window bound (AC-OFFICE-STEP-ENTRY-DISPATCH-004.12) needs an entry
  allocated older than 24 hours: assert its unfinished positions are retired
  `idempotency_window_expired` with an ERROR, and that no run is enqueued. A
  fixture inside the window cannot distinguish the bound from its absence.
- The enumeration-order fixture must place two participants whose
  agent-profile-id order and row-creation order disagree
  (AC-OFFICE-STEP-ENTRY-DISPATCH-007.9). The existing fixture is lexically
  pre-sorted, so it passes under either tiebreak and would not have caught the
  row-id swap this design withdrew.

## Risks

- **Moving ownership changes idempotency keys.** A run enqueued before the
  change and one enqueued after carry different keys for the same entry, so a
  deployment that lands mid-round can duplicate one round's runs once. Strictly
  better than duplicating every round; no migration is proposed.
- **Unconditional allocation adds one row per arrival** at a step declaring a
  marker-bearing kind, on routes that previously allocated none.
- **The task lock widens a critical section** that currently admits concurrent
  sessions of one task. Turn completion already serializes per session; the
  change makes contention visible where it was previously silent corruption.
- **Three ALTER migrations run on existing databases.** `MigrateLogger.Apply`
  swallows failures at WARN, which "Migrated columns are probed before use"
  handles. The residual risk is that a probe failure degrades silently from the
  operator's view except for one ERROR line: a backend whose
  `marker_positions` ALTER failed runs indefinitely with no startup recovery
  and no second signal.
- **`marker_positions` is a denormalized copy** of what the ownership
  declaration would say at allocation time. That is deliberate - it is what
  makes `unresolvable` decidable - but it means an entry allocated before a
  kind's marker-bearing property changes keeps the old set. The digest check in
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.3 retires such an entry as
  `digest_changed` rather than resuming it against a stale set.
