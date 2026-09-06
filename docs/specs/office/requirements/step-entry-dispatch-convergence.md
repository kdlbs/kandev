---
status: draft
system: office
created: 2026-09-02
owners:
  - kandev
---

# Step Entry Dispatch Convergence Requirements

## Overview

Two dispatchers now execute a step's declared entry sequence on the same
committed arrival, and two action kinds are handled by both: the **ledger
dispatcher** (`Repository.dispatchStepEntry`, every step-transition writer, no
marker) and the **marker
dispatcher** (`processOnEnter`, `on_turn_complete` only, per-action
compare-and-set marker keyed on a
`workflow_step_entries` row id). The system design's Current shape table is
authoritative for both.

Neither is wrong alone. Together they execute those two kinds **twice per
arrival**, and because the two paths derive idempotency keys from different
identifiers, the runs queue does not collapse the duplicate. The shipped
`apps/backend/config/workflows/office-default.yml` declares both kinds on
Review and Approval, so this reaches the default Office workflow.

A duplicate fan-out is a correctness defect, not a cost defect: each queued run
is an agent authoring pass, and two reviewer passes for one round can record
two different decisions that quorum counts both of. See Prior art.

This requirement converges the dispatchers onto one owner per action kind and
closes the diagnostic and boundary criteria
`workflow-on-enter-action-dispatch` stated but no implementation satisfies.
Verified state records what was measured and what is restated. It governs every
workflow's entry sequence, not only Office's, for the reason
[`step-entry-sequence-execution.md`](step-entry-sequence-execution.md) does.

## Terminology

Terms defined in [`step-entry-sequence-execution.md`](step-entry-sequence-execution.md)
- **arrival**, **step entry**, **entry identity**, **redelivery** - carry the
same meaning here.

- **Owning dispatcher:** for one action kind, the single dispatcher permitted
  to execute it for an arrival. Every other dispatcher skips that kind.
- **Marker:** the `workflow_step_entry_markers` row recording that one
  (entry, position) was claimed and how it ended: `in_progress`, `done`,
  `failed`, or `skipped` (AC-OFFICE-STEP-ENTRY-DISPATCH-004.9).
- **Marker-bearing kind:** an action kind whose execution is recorded by a
  marker. Declared alongside ownership per
  AC-OFFICE-STEP-ENTRY-DISPATCH-002.1 and independent of it: a kind can be owned
  and carry no marker. Only a step declaring at least one marker-bearing kind is
  allocated an entry identity.
Recovery from a marker left `in_progress` is stated in
[`step-entry-recovery-scan.md`](step-entry-recovery-scan.md).

## Verified state

Measured on `feature/fix-on-enter-dispatc-77a` at `646ff0063`, 2026-09-02.
Four inherited findings are **already satisfied**: the AC-F1 CAS-loser stop
rule, the single-session duplicate-turn race, fan-out participant
de-duplication, and fan-out failure isolation. Only the last three go
unrestated. AC-F1 is satisfied on the *marker* path this work retires, so
AC-OFFICE-STEP-ENTRY-DISPATCH-002.10 re-imposes it on the ledger path taking
over. Measurements are in the system design's Verified state.

## Requirements

### REQ-OFFICE-STEP-ENTRY-DISPATCH-002: One dispatcher owns each action kind

**Intent:** An arrival executes each declared action once. Today two
dispatchers execute two kinds, so the default Office workflow queues each
reviewer twice per round and clears decisions twice.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.1:** Each step-entry action kind shall
  have exactly one owning dispatcher, recorded in a single declaration both
  dispatchers read. Two independent lists that can disagree shall not satisfy
  this. That declaration shall also record, per kind, whether the kind is
  **marker-bearing**. The two properties are distinct and shall not be
  conflated: a kind can be owned and carry no marker. Every criterion
  quantifying over marker-bearing positions, here and in the sibling files,
  reads that one declaration; inferring the set from the ownership column shall
  not satisfy this. The declaration shall classify every kind either dispatcher
  can reach, in both columns. The marker-bearing kinds are exactly
  `clear_decisions` and `queue_run_for_each_participant`; `queue_run`,
  `run_code_review` and `ensure_participant_seat` are ledger-owned and **not**
  marker-bearing; the five session-shaped kinds named in Out of scope,
  `configure_session` among them, are neither. Leaving a reachable kind
  unclassified in either column shall not satisfy this.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.2:** When a task arrives at a step
  declaring `clear_decisions` and `queue_run_for_each_participant`, the system
  shall execute each of those actions exactly once for that arrival, counted
  across every dispatcher, and shall enqueue exactly one run per matching
  participant that AC-OFFICE-STEP-ENTRY-DISPATCH-007.4 does not skip. "Exactly
  once" governs live dispatch. The startup scan's whole-action retry
  (AC-...-004.7, 004.8) is the one stated exception and does not violate this:
  it re-executes only a position holding no terminal marker, and AC-...-004.6's
  key re-derivation makes an already-enqueued run suppress rather than
  duplicate. A retry triggered by anything else shall not satisfy this.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.3:** A dispatcher that is not the owner
  of an action kind shall skip it without emitting a warning or an error
  record, and without writing a marker for it. The skip is the contract.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.4:** The owning dispatcher for
  `clear_decisions` and `queue_run_for_each_participant` shall retain the
  durability guarantees `workflow-on-enter-action-dispatch` requires: the
  per-action compare-and-set claim (AC-F1), the delete-and-marker single
  transaction (AC-B6), and entry-scoped idempotency keys (AC-B1). Moving
  ownership to a dispatcher that drops these shall not satisfy this criterion.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.5:** A regression test shall assert that
  one arrival at the shipped `office-default` Review step, via
  `on_turn_complete` with both dispatchers wired as in production, enqueues
  exactly one run for a single reviewer. Reading one run off a queue without
  asserting the queue is then empty shall not satisfy this: the existing
  acceptance test does that and passes against today's duplicate. Both
  dispatchers remaining wired satisfies AC-OFFICE-STEP-ENTRY-001.8 per action
  kind through 002.1's single-owner rule. Retiring the marker dispatcher's call site is therefore **not**
  required by 001.8 and shall not be done to satisfy it: it would make 002.3's
  silent-skip contract vacuous and invalidate 006.3.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.7:** Every action one dispatcher owns
  shall execute in declared relative order. Skipping a kind owned by another
  dispatcher shall not reorder those that remain, so a `clear_decisions`
  declared before a fan-out shall commit its delete before that fan-out enqueues
  any run - both share one owner, so the guarantee is unconditional for them. A
  dispatcher shall not batch owned kinds ahead of or behind their declared
  positions. Order **between** actions owned by different dispatchers is not
  guaranteed; see Out of scope.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.8:** The idempotency key of a run from an
  entry action shall be a function of entry identity, the action's position,
  and the canonical action configuration, and of nothing that varies between
  dispatchers or redeliveries. Two dispatches of one entry identity shall
  produce the same key; two distinct entries shall produce different keys.
  Deriving it from the dispatcher's own row id shall not satisfy this. It
  governs runs from **marker-bearing** actions. A step declaring no
  marker-bearing kind allocates no entry, so its actions keep the
  step-transition row id they derive their key from today. A non-marker-bearing
  action inside a step that *does* allocate an entry is the third case: it keeps
  that row id too, because entry identity governs marker-bearing positions
  only.
  Which case applies shall be decided from 002.1's declaration, not by
  inspecting whether an entry row exists.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.10:** When a **marker-bearing** action
  fails or loses its marker claim, the owning dispatcher shall abandon the
  remainder of that entry's sequence and shall execute no later position. This
  restates AC-F1's stop half on the dispatcher taking ownership: the ledger path
  is record-and-continue by contract (AC-OFFICE-STEP-ENTRY-001.4, .6), and
  AC-OFFICE-STEP-ENTRY-001.4 continues "except where another acceptance
  criterion states otherwise" - this is that criterion. It is scoped to
  marker-bearing positions: a non-marker-bearing action that fails keeps
  record-and-continue, so 001.4 is narrowed, not replaced. A marker-bearing
  position retired `skipped` for an unwired adapter (006.4) shall stop the
  sequence on the same terms: its declared work did not happen either, so
  continuing would fan out to reviewers against decisions never cleared. A test
  shall assert that a failed `clear_decisions` at position 0 enqueues zero runs
  from a later fan-out position.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.9:** Moving ownership shall not change an
  action's defaults or its configuration digest. An omitted `reason` shall
  resolve to the same literal, an omitted `payload` shall enqueue SQL NULL not
  an empty object, and the digest shall still be computed after defaults are
  applied, with map keys serialized in sorted order.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-002.6:** Every route that changes a task's
  workflow step shall continue to execute the declared entry sequence, per
  AC-OFFICE-STEP-ENTRY-001.1. Convergence shall not narrow coverage to the
  `on_turn_complete` route, as `finalizeStepEnter` does today.

### REQ-OFFICE-STEP-ENTRY-DISPATCH-003: Turn completion serializes per task

**Intent:** `turnCompletionLocks` is keyed on session id, and the fan-out
creates several sessions per task, so two sessions transition one task
concurrently, each bypassing the other's guard. Serialization and the staleness
decision must move to task identity;
`turnCompletionConsumedGeneration` stays session-keyed for its narrower
redelivery role, per AC-OFFICE-STEP-ENTRY-DISPATCH-003.2.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-003.1:** When two turn-completion signals for
  two different sessions of the same task are processed concurrently, the
  system shall serialize them so that at most one applies a step transition for
  that task. The guarantee is within one backend process (see Out of scope).
- **AC-OFFICE-STEP-ENTRY-DISPATCH-003.2:** The serialization, and the
  *staleness* guard deciding whether a caller's observed step is still current,
  shall both be keyed on task identity; a session-keyed staleness guard shall
  not satisfy either. This does not forbid an additional session-keyed
  *redelivery* check rejecting a repeat of one session's own snapshot: that is a
  narrower guard inside the task-keyed critical section, not the staleness guard
  this criterion governs.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-003.3:** `processOnChildrenCompleted` reaches
  the same step-entry allocation write path under its own separate lock and
  shall be routed through the same task-keyed serialization.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-003.6:** When the read that establishes the
  task's current step inside the critical section fails, the caller shall be
  rejected and shall apply no transition. Proceeding on a failed read shall not
  satisfy this: two concurrent callers whose reads both fail would both
  proceed. A rejected caller
  is not an error - the next turn-completion signal re-evaluates the trigger -
  so the rejection shall be logged at WARNING with task id and cause and shall
  not fail the turn.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-003.4:** Serializing by task shall not
  suppress a transition for a different task, nor a legitimate later
  transition for the same task once the first has completed.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-003.5:** A test shall drive two distinct
  sessions of one task through concurrent turn completion and assert exactly
  one `workflow_step_entries` row is allocated. The existing concurrency test
  uses one session and passes today.

**Three contracts live in sibling files**, each keeping its
`AC-OFFICE-STEP-ENTRY-DISPATCH-*` identifiers so citations still resolve:
recovery of a stuck entry in
[`step-entry-recovery-scan.md`](step-entry-recovery-scan.md) (REQ-004 - it
governs any dispatcher claiming a marker, and its scan is the retry trigger the
fan-out relies on); coalescing of entry-triggered runs in
[`step-entry-run-coalescing.md`](step-entry-run-coalescing.md) (REQ-005 - it
changes the runs queue, not either dispatcher); and the fan-out's reporting,
declaration-fault and marker-outcome contract in
[`step-entry-fanout-reporting.md`](step-entry-fanout-reporting.md) (REQ-007 - it
governs one callback's contract).

### REQ-OFFICE-STEP-ENTRY-DISPATCH-006: A discarded action is visible

**Intent:** Silent discard is the defect class this work exists to remove, and
several discard points are still silent.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-006.1:** When a declared entry action does
  not reach execution, the system shall emit a WARNING carrying workflow id,
  step id, step name, and action type as discrete fields. The current record
  omits workflow id and step name.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-006.2:** The warning shall cover every
  discard point, naming each: an unrecognized action type; an action the
  dispatcher cannot compile; an action whose callback is not registered; and a
  **marker-bearing** action that reaches a dispatcher with no entry identity.
  All but the first are silent today. The last is deliberately restricted: an
  action carrying no marker has no entry identity to be missing, so it shall not
  warn.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-006.3:** A kind skipped because another
  dispatcher owns it shall not warn, per
  AC-OFFICE-STEP-ENTRY-DISPATCH-002.3. Whether a kind is a warning candidate
  shall be decided against the owning dispatcher for that kind, not globally. A
  dispatcher that loses a marker claim shall likewise not warn: the claim's
  holder is executing the declared action, which is the contended outcome
  AC-OFFICE-STEP-ENTRY-DISPATCH-008.4 asserts, not a discard.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-006.5:** A step declaring an empty entry
  sequence, or one whose every action is owned but not marker-bearing, shall
  produce no warning, no marker, and no allocation, and shall behave as it does
  today. A dispatcher evaluating a step whose every action **another**
  dispatcher owns shall likewise produce no warning and no marker - but that
  clause governs warnings and markers only, never allocation. Allocation is not
  dispatcher-relative: it happens once, inside the transition transaction,
  before either dispatcher runs, and is decided solely by whether the step
  declares a marker-bearing kind (002.8). Reading it as suppressing allocation
  would deny an entry identity to every shipped Review and Approval arrival,
  whose three actions are all ledger-owned and so "owned by another dispatcher"
  from the marker dispatcher's side. Only a declared action reaching no owner is
  a discard.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-006.4:** Where the run-queue, participant,
  and decision adapters are unwired, a step declaring an action requiring one
  shall produce the 006.1 warning and shall not produce a failed marker or a
  failed transition. Today the callback returns a not-wired error, marking the
  action failed. For a **marker-bearing** kind the dispatcher shall additionally
  write a terminal `skipped` marker for that position, with reason
  `adapter_unwired` (AC-OFFICE-STEP-ENTRY-DISPATCH-004.9). Leaving the position
  with no marker row at all shall not satisfy this: the position would take the
  ordinary insert on the next pass and be discarded again, so the entry could
  never become terminal and the startup scan would re-select it at every process
  start. `skipped` is terminal and is not a *failed* marker, so this does not
  weaken the prohibition above.

### REQ-OFFICE-STEP-ENTRY-DISPATCH-008: Guarantees are tested distinguishably

**Intent:** Three guarantees have no test that can fail if removed.

#### Acceptance criteria

- **AC-OFFICE-STEP-ENTRY-DISPATCH-008.1:** A test shall assert that the
  decision delete and its completion marker commit in one transaction, by
  observing that a failure between them leaves neither applied. A test that
  passes equally against a non-atomic implementation shall not satisfy this.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-008.2:** A test shall assert that two entries
  into the same step for the same task produce two runs with **different**
  idempotency keys. Asserting call count alone shall not satisfy this: it
  passes when the keys collide.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-008.3:** A test shall assert that a failed
  step-entry allocation fails the transition as a unit, leaving neither the
  entry row nor the step change, and emits an ERROR naming task id and step id.
- **AC-OFFICE-STEP-ENTRY-DISPATCH-008.4:** A test shall assert that a
  dispatcher losing a marker claim enqueues zero runs, for a prior marker in
  `failed` state and for one in `in_progress` state. The existing test covers
  only `failed`. This governs the live-dispatch claim; the startup scan's
  reclaim (AC-OFFICE-STEP-ENTRY-DISPATCH-004.8) is a separate path.

## Out of scope

- **The 24-hour idempotency window** keeps its current value and does not
  become configurable. The coalescing window is
  [`step-entry-run-coalescing.md`](step-entry-run-coalescing.md)'s subject.
- **Prompt-delivery mechanics per route.** Routes legitimately differ; only
  action coverage is unified.
- **Session-shaped actions.** `enable_plan_mode`, `auto_start_agent`,
  `reset_agent_context`, `set_session_mode`, and `configure_session` keep their
  owner, fixed positions, and at-most-once-per-entry behavior. They hold no
  marker and are not resumed by recovery.
- **Ordering between actions owned by different dispatchers.** Declared order
  holds within one owner's actions (AC-OFFICE-STEP-ENTRY-DISPATCH-002.7); it is
  not guaranteed across owners, and no barrier is introduced. The two run at
  different times by construction, so a cross-owner order would mean
  re-sequencing one of them. No shipped workflow mixes owners in one entry
  sequence, and the sequencing this initiative protects - `clear_decisions`
  before its fan-out - is wholly within the ledger owner. A workflow needing it
  is unsupported; making it so is new work.
- **Serializing across backend processes.** REQ-003's guarantees hold within
  one backend process. Kandev runs a single backend against its store, and no
  database-level or advisory-lock scheme is introduced.
- **Making `on_turn_start`, `on_turn_complete`, `on_exit`, or the event
  triggers continue past a failed action.** These keep abort-on-first-error.
- **Retiring the legacy spec.** `workflow-on-enter-action-dispatch` remains the
  record of the criteria converged here; it is not edited.

## Prior art

**Our own reasoning (wiki).** Vault `/Users/henry/Documents/henry/wiki`; QMD
collection `wiki`, queried lex + vec + hyde on exactly-once dispatch and
duplicate execution. The decisive hit is
`concepts/agent-replay-non-idempotence.md` (0.91): re-running an *author* is not
idempotent, and only a *deterministic materializer* is safe to replay. This
requirement follows it twice - a duplicate fan-out is a correctness defect
(REQ-002), each duplicate run being a second agent pass that can record a
second, different decision; and recovery is bound to the original entry identity
(AC-004.6) rather than re-dispatching freely.
`concepts/optimistic-vs-pessimistic-concurrency.md` (0.30) supports keeping the
per-action compare-and-set claim over optimistic retry.

**What others shipped (saas-kb).** `search_fsm_docs`, `category: "ai_sdlc"`, on
duplicate entry-action dispatch and on exactly-once agent-run de-duplication:
every hit scored below 0.01 and described webhook triggers, prompt queueing or
task lifecycle states. No vendor prior art is carried in.
