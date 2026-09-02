---
status: draft
system: office
created: 2026-09-02
owners:
  - kandev
---

# Step Entry Recovery Scan Requirements

## Overview

A step entry that is claimed but never completed strands permanently.
`ClaimStepEntryMarker` only inserts, and `CompleteStepEntryMarker` only updates
rows already `in_progress`. A process that dies after claiming, or an atomic
clear whose transaction rolls back, leaves the marker `in_progress` forever;
every later dispatch then abandons the entry, so it never completes and the
task stalls with no error and no operator signal. There is no lease and no
takeover path today.

This requirement is separated from
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md),
which discovered it, for the reason
[`step-entry-sequence-execution.md`](step-entry-sequence-execution.md) is
separate: the contract is not specific to convergence. Any dispatcher that
claims a marker can strand one, so recovery governs every workflow's entry
sequence and is stated once, here. Its acceptance criteria keep the
`AC-OFFICE-STEP-ENTRY-DISPATCH-004.*` identifiers they were authored under, so
existing citations continue to resolve.

## Terminology

Terms defined in
[`step-entry-sequence-execution.md`](step-entry-sequence-execution.md) -
**arrival**, **step entry**, **entry identity**, **redelivery** - carry the same
meaning here.

- **Marker:** the `workflow_step_entry_markers` row recording that one
  (entry, position) was claimed and how it ended.
- **Terminal marker state:** one from which no further execution of that
  position follows.
- **Marker-bearing kind:** an action kind whose execution is recorded by a
  marker, as declared by the single ownership table
  AC-OFFICE-STEP-ENTRY-DISPATCH-002.1 requires. Not every declared action is
  marker-bearing: a step's entry sequence may also declare kinds that execute
  without a marker, and those positions are never resumed by this scan and
  never hold an entry open.
- **Allocated position set:** the ordered list of marker-bearing positions
  recorded on the entry row when it was allocated. It is stored, not
  recomputed, so every rule below is decidable from the store alone.
- **Reclaim:** the scan's own claim, which supersedes an existing non-terminal
  marker for a `(entry, position)`. It is distinct from the live-dispatch
  claim, which never supersedes an existing row.
- **Declaration digest:** the shape-only digest stored in
  `workflow_step_entries.digest`, covering action order and kind but **not**
  action config. It is a different value from the per-action *configuration*
  digest of AC-OFFICE-STEP-ENTRY-DISPATCH-002.9.

## Requirements

### REQ-OFFICE-STEP-ENTRY-DISPATCH-004: A stuck entry recovers

**Intent:** Return a stranded entry to a terminal state exactly once per
process start, without re-running an agent pass that already ran.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.1:** On backend start, once the store is
  open and before the workflow engine serves triggers, the system shall select
  every non-terminal step entry and resume it, subject to
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.3. This shall be one query executed once
  per process start, not periodic. The selection shall read only stored state -
  the allocated position set and its markers - and shall not require a workflow
  declaration to be loaded, so an entry whose declaration is unreadable is still
  selected and still reaches AC-OFFICE-STEP-ENTRY-DISPATCH-004.3's
  `unresolvable` rule rather than being silently omitted.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.2:** The scan shall not prevent the
  backend from starting under any condition. A record that fails to process
  shall be logged at ERROR, left non-terminal for a later start, and the scan
  shall continue; a failure of the scan's own query shall be logged at ERROR
  and startup shall proceed.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.3:** The scan shall skip and mark
  terminal, without executing: an entry whose `step_id` is not the task's
  current step (`step_left`); any entry for a surviving `(task_id, step_id)`
  pair other than the greatest `entry_seq` (`superseded`); an entry whose
  `clear_decisions` position holds a marker in any state other than `done` -
  `failed`, or a terminal `skipped` written under
  AC-OFFICE-STEP-ENTRY-DISPATCH-006.4 for an unwired decision adapter
  (`clear_decisions_failed`); an entry whose stored declaration digest does not
  match one freshly computed from the step's current declaration
  (`digest_changed`); and an entry whose task, step, workflow, or declaration
  cannot be loaded (`unresolvable`, with a WARNING naming task id and step id).
  Because the declaration digest is shape-only, a config-only edit shall not
  make an entry stale under this criterion. Marking an entry terminal shall mean
  writing a terminal marker for every position in its allocated position set
  that does not already hold one, each carrying that entry's skip reason.
  Because that set is stored, this shall remain possible for `unresolvable` and
  `digest_changed`, where the current declaration is respectively unreadable and
  no longer shape-identical; a rule that required re-reading the declaration to
  enumerate the positions to retire would leave those two entries permanently
  non-terminal and re-selected at every process start.
  A `skipped` `clear_decisions` counts under the third predicate even though
  AC-OFFICE-STEP-ENTRY-DISPATCH-006.4 is explicit that `skipped` is not
  `failed`. The gate is the *outcome* of that position, not the marker state
  name: AC-OFFICE-STEP-ENTRY-DISPATCH-002.10 already stops the live sequence on
  that outcome, which leaves every later marker-bearing position holding no
  marker row and therefore non-terminal under
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.9. Reading the predicate as `failed`-only
  shall not satisfy this criterion: the scan would then select such an entry,
  resume the orphaned fan-out, and wake reviewers against decisions never
  cleared - the harm REQ-OFFICE-STEP-ENTRY-DISPATCH-002 exists to remove,
  reintroduced through the restart path. The widening is confined to
  `clear_decisions`: a `skipped` marker on a later position, with
  `clear_decisions` `done`, is not this case and shall not retire the entry. A
  `clear_decisions` position holding **no** marker at all is likewise not this
  case - nothing ran, so the entry is resumed from that position under
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.4's ascending order.
  **Precedence.** An entry can satisfy several of these at once, and the marker
  carries exactly one `skip_reason`, so the predicates shall be evaluated in the
  order they are named above - `step_left`, `superseded`,
  `clear_decisions_failed`, `digest_changed` - and the first that matches shall
  be the entry's single recorded reason. `unresolvable` shall be recorded
  instead, in preference to all four, whenever evaluating one of them requires a
  task, step, workflow, or declaration that cannot be loaded, since none of the
  four is decidable in that case. AC-OFFICE-STEP-ENTRY-DISPATCH-004.12's
  `idempotency_window_expired` shall be evaluated only after all five have
  failed to match, so an entry retired for a structural reason records that
  reason and emits no 004.12 ERROR: it would have been retired at any age, so
  its age is not why it is being retired and no work is being abandoned that an
  operator must decide about. Recording more than one reason, or resolving the
  overlap by any other rule, shall not satisfy this.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.4:** Records shall be processed in
  `(task_id, step_id, entry_seq)` order, and the positions within one resumed
  entry in ascending `position` order, so a resumed `clear_decisions` still
  commits its delete before a later fan-out enqueues any run, as
  AC-OFFICE-STEP-ENTRY-DISPATCH-002.7 requires of a live arrival.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.5:** A marker found `in_progress` by the
  startup scan shall be treated as failed, not as live work. The scan runs
  before the engine serves triggers, so no in-process dispatcher can hold one,
  and no lease shall be inferred from wall-clock age. For a `clear_decisions`
  position the entry is then terminal under 004.3 and is not re-executed.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.6:** Resuming an entry shall re-derive
  each run's idempotency key from the same entry identity the original dispatch
  used, so that a run already enqueued is suppressed by the runs queue rather
  than duplicated. Recovery shall not re-enqueue an agent pass that already ran.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.7:** A participant whose enqueue failed
  shall be retried only by a whole-action retry under this scan. No
  per-participant retry path shall be added.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.8:** The scan shall resume a position by
  reclaiming it - superseding the existing non-terminal marker for that
  `(entry, position)` and returning it to `in_progress` under the scanning
  process. A claim that only inserts, and so refuses any position already
  holding a row, shall not satisfy this criterion: it would make the scan a
  no-op for exactly the entries it exists to rescue. A marker in `done` or in
  any skipped-terminal state shall never be reclaimed, so a position that
  already completed is not executed a second time. This reclaim is a distinct
  path from the live-dispatch claim, which AC-OFFICE-STEP-ENTRY-DISPATCH-008.4
  governs and which continues to refuse every position already holding a row.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.9:** An entry's terminality shall be
  durable across process restarts and shall carry its reason. A position
  retired without executing shall end in a terminal marker state distinct
  from both `done` and `failed`, so it is never read as work that ran nor as an
  enqueue that was attempted, and the reason - one of `step_left`,
  `superseded`, `clear_decisions_failed`, `digest_changed`, `unresolvable`,
  `adapter_unwired` (AC-OFFICE-STEP-ENTRY-DISPATCH-006.4) or
  `idempotency_window_expired` (AC-OFFICE-STEP-ENTRY-DISPATCH-004.12) - shall be
  recorded against it. The list is closed: a position retired for a reason not
  named here shall not satisfy this. Two writers produce that state and both are
  governed: this scan, for a position it skips, and the owning dispatcher at
  live dispatch, for the `adapter_unwired` case
  AC-OFFICE-STEP-ENTRY-DISPATCH-006.4 requires. A criterion reading this state
  shall not assume the scan wrote it. An entry is non-terminal for
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.1 exactly while at least one position in its
  **allocated position set** has no terminal marker. Terminality shall be
  derived from that stored set and its markers alone. Deriving it from the
  positions a step's declaration currently declares shall not satisfy this
  criterion: a declaration can name positions that are not marker-bearing and so
  can never hold a marker, which would leave every such entry non-terminal
  forever, and a declaration that cannot be loaded or has since changed shape
  would leave terminality undecidable for exactly the entries
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.3 must be able to retire. An entry whose
  stored allocated position set cannot be read as a position list shall be
  treated as terminal and logged at ERROR naming the entry: there is no position
  to resume and none to retire, and treating it as non-terminal would re-select
  it at every process start with no rule able to close it.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.10:** This requirement's guarantees are
  stated under an explicit precondition: one backend process operates the store,
  as AC-OFFICE-STEP-ENTRY-DISPATCH-003.1 also assumes. No criterion here
  requires the system to detect or prevent a second concurrent process, and no
  advisory lock, lease, or election is introduced (see Out of scope). Should
  that precondition cease to hold, this requirement is void rather than
  violated, and reinstating it is new work.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.11:** Before the scan issues any statement
  naming a column added by migration, the system shall probe that the column
  exists. When it does not, the system shall log one ERROR naming the table and
  the column and shall skip the scan for that process start, leaving entries
  non-terminal for a later start under AC-OFFICE-STEP-ENTRY-DISPATCH-004.2. It
  shall not issue a statement against a missing column. This is required because
  the repository's migration helper records a failed `ALTER` at WARNING and
  continues, so a column can be absent at runtime without startup having
  failed.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-004.12:** AC-OFFICE-STEP-ENTRY-DISPATCH-004.6
  relies on the runs queue suppressing a re-derived key, and that suppression is
  bounded by the 24-hour idempotency window
  [`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md)
  freezes in its Out of scope. The scan shall therefore compare each candidate
  entry's allocation time against that window. **Candidate** means an entry
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.3 did not already retire, per that
  criterion's precedence rule. The allocation time shall be read from
  `workflow_step_entries.created_at`, the column the allocating INSERT writes,
  and shall be compared against a single instant captured once at the start of
  the scan, so every entry in one scan is judged against the same instant and
  the scan's own duration cannot change a verdict. An entry is older than the
  window when that comparison is **strictly** greater than 24 hours; an entry
  whose age is exactly 24 hours is **not** expired and shall be resumed
  normally. An entry older than the window
  shall not have its unfinished marker-bearing positions re-executed; the scan
  shall retire each of them terminally with reason
  `idempotency_window_expired` and emit one ERROR carrying task id, step id and
  entry id as discrete fields. Re-executing such a position shall not satisfy
  this: its re-derived key can no longer suppress the run enqueued before the
  restart, so the retry would queue a second agent authoring pass - exactly what
  AC-OFFICE-STEP-ENTRY-DISPATCH-004.6 forbids and what Prior art names as a
  correctness defect. Retiring it silently shall not satisfy this either: the
  entry is being abandoned with work possibly unfinished, which is the operator's
  decision to make and shall be visible. A backend stopped overnight and
  restarted the next day is the ordinary case this governs, not an exotic one.

## Out of scope

- **A lease or reaper for live markers.** Recovery runs once at startup, before
  the engine serves triggers, so no wall-clock lease is introduced and no
  periodic sweep is added.
- **Resuming session-shaped actions.** `enable_plan_mode`, `auto_start_agent`,
  `reset_agent_context`, `set_session_mode`, and `configure_session` hold no
  marker and are not resumed: re-running one would launch or prompt an agent
  twice, or re-apply a session configuration the operator has since changed.
- **A per-participant retry path.** Per AC-OFFICE-STEP-ENTRY-DISPATCH-004.7,
  this scan's whole-action retry is the only retry trigger.

## Prior art

Carried from
[`step-entry-dispatch-convergence.md`](step-entry-dispatch-convergence.md),
whose Prior art section records the wiki and saas-kb legs in full. The load
-bearing position is `concepts/agent-replay-non-idempotence.md` (0.91,
`lifecycle: draft`): re-running an *author* is not idempotent. That is why
recovery is bound to the original entry identity (AC-004.6) and why a `done`
position is never reclaimed (AC-004.8).
