---
status: draft
system: office
created: 2026-09-08
owners:
  - kandev
---

# Parent Wake Wave Identity Requirements

## Overview

When every direct child of a parent task reaches a terminal state, Kandev wakes
the parent's agent with a `task_children_completed` run. Four independent
producers can emit that wake for the same parent at the same moment, and each
identifies the completion event differently today. The consequences are opposite
failures of one missing primitive:

- **Duplicate wake.** Two producers describing the same completion event mint
  structurally different run identities, so the run queue's uniqueness constraint
  cannot collapse them. The parent agent runs twice, pays twice, and re-reads the
  same child results.
- **Spurious wake.** The backstop decides "has this parent already been woken for
  its current children" by comparing timestamps, and every task write stamps
  `updated_at` — so editing a finished child's title, priority, or labels makes a
  delivered wake look stale and queues a needless autonomous run.

This capability defines a single **completion wave identity**: one value naming
*which completion event* a wake belongs to, computed the same way by every
producer, persisted on the run, and compared instead of a timestamp.

Office owns this contract because the outcome is an Office wakeup policy
decision: whether an autonomous `task_children_completed` run enters the Office
run queue. It reads and constrains, but does not own, the workflow engine's
`on_children_completed` trigger and operation ledger and the task system's
parent/child relationship and `updated_at` stamping. The task system's
[subtask completion trigger](../../tasks/requirements/subtask-completion-trigger.md)
still owns when the trigger fires and what its payload contains; this capability
constrains only the identity carried by the resulting run.

## Terminology

- **Completion wave:** the event of a parent's full set of wave members being
  terminal at once. Iterative delegation produces one wave per round.
- **Wave member:** a direct child task of the parent that counts towards its
  parent's completion wave: `archived_at` unset, `is_ephemeral` false, and
  `origin` not `automation_run`. Children failing any of these are excluded from
  wave identity throughout. This is the predicate the orchestrator's existing
  children-completed readiness query applies; the design records why that makes it
  the only one all four producers can share.
- **Wave identity:** the value naming a completion wave. Equal values mean the
  same wave. It has the two encodings below, which are two spellings of one
  identity rather than two identities.
- **Wave string:** the canonical encoding of a wave identity: the parent id and
  its wave members' ids in ascending `tasks.id` order, joined by a fixed
  separator. It is what the backstop's candidate query compares, so it must be
  derivable with plain SQL string aggregation. A parent with no wave members has
  no wave and no wave string; an SQL expression of the same shape evaluated per
  row, before that parent is known to have any member, is an intermediate, not a
  wave string, until the parent passes the wave-member gate.
- **Wave key:** the fixed-length digest of the wave string, and the encoding the
  run queue's uniqueness constraint indexes, where an unbounded value would
  eventually exceed an index entry limit.
- **Producer:** any code path that can cause a `task_children_completed` run to
  be queued for a parent. Four exist; the design enumerates them.
- **Target agent:** the agent profile a queued run is addressed to. One wave can
  legitimately fan out to several when a workflow step configures it.
- **Terminal state:** the child task states that count as done, as already
  defined by the `on_children_completed` trigger.

## Requirements

### REQ-OFFICE-WAKE-WAVE-IDENTITY-001: One canonical completion-wave identity

**Intent:** A wave must have exactly one name, so that agreement between
producers is a property of the definition rather than of four code paths
happening to match.

#### Acceptance criteria

- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.1:** Wave identity shall be a pure function
  of the parent task id and the set of its wave-member child task ids. No other
  input shall affect it.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.2:** Child ids shall enter the derivation
  ordered ascending by `tasks.id`, which is unique, so no tiebreak column is
  required and none shall be introduced. A producer holding rows in any other
  order shall re-sort first; arrival order is not sufficient.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.3:** Children that are not wave members
  shall be excluded from the derivation. A child entering or leaving wave
  membership — by archiving, unarchiving, or being created ephemeral or
  automation-origin — shall change the wave identity only when it changes the
  wave-member id set.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.4:** When a child moves between two terminal
  states, `CANCELLED` to `COMPLETED` say, the wave identity shall not change.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.5:** When any child field other than wave
  membership changes — title, description, priority, labels, assignee, workflow
  step, `updated_at` — the wave identity shall not change.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.6:** When a wave member is added or removed,
  the wave identity shall change.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.7:** A parent with no wave members has no
  wave, and no `task_children_completed` run shall be queued for it.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.8:** For the same parent and wave-member id
  set, every producer shall derive a byte-identical wave string and wave key from
  the same wave-member predicate; a producer whose existing row source applies a
  different predicate shall be brought onto the shared one.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.9:** Wave identity shall not depend on the
  wake payload: child-summary truncation, a missing child summary, and missing or
  present pull-request enrichment shall not change it.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.10:** Wave identity derivation shall have no
  failure mode of its own: it shall be computable from values the caller already
  holds, without a further read that can error.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.11:** The wave string and the wave key shall
  be two encodings of one identity: the key shall be a pure function of the
  string, so equal strings always yield equal keys, and both shall be derived once
  per queued run from the same member-id set rather than computed independently at
  two call sites.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.12:** The backstop's candidate query shall
  derive the current wave string using only string aggregation available on both
  SQLite and PostgreSQL; no hash or digest shall be required inside SQL on either
  dialect.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-001.13:** The ascending `tasks.id` order in .2
  shall be the same total order in application code and in SQL on both dialects.
  It rests on task ids being lowercase hexadecimal UUIDs, for which byte order and
  collation order coincide, and shall not depend on any database locale or
  collation setting.

### REQ-OFFICE-WAKE-WAVE-IDENTITY-002: At most one wake per wave per target agent

**Intent:** Producers race by design — the reconciler exists precisely because
an edge-triggered dispatch can be lost — so correctness must come from a durable
constraint the second arrival cannot bypass, not from producers avoiding each
other.

#### Acceptance criteria

- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.1:** Every queued `task_children_completed`
  run shall durably record the wave identity it was queued for, in both encodings.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.2:** The run queue shall enforce durable
  uniqueness over (wave key, target agent).
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.3:** When two producers concurrently queue a
  run for the same wave and target agent, exactly one run row shall exist
  afterwards.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.4:** The producer whose insert loses that
  race shall observe a deduplicated outcome, not an error; the wake shall not be
  logged as a failure or retried. This holds for every producer, including one
  inserting directly rather than through the shared run-admission service —
  reaching the queue by an unclassified path is not an exemption.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.5:** Deduplication shall behave identically
  on SQLite and PostgreSQL.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.6:** Deduplication shall not be time-bounded:
  a second attempt at the same wave shall be deduplicated however long after the
  first it arrives.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.7:** When one wave fans out to two distinct
  target agents, each shall receive exactly one run.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.8:** A run for a different wave of the same
  parent shall not be suppressed by .2. A parent that completes wave 1, is given
  new children, and completes wave 2 shall be woken for both.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.9:** Runs of every other reason shall be
  unaffected: recording no wave identity leaves a run outside this constraint
  entirely.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.10:** The wake the surviving run delivers
  shall be equivalent to the wake the losing producer would have delivered. No
  producer shall depend on being the winner, and no producer's wake shall carry
  context that another producer's wake for the same wave omits. The obligation
  binds whenever two producers would queue for the same wave *and* the same
  target agent, which is when one of the two is suppressed; producers queueing to
  different target agents each deliver their own run and are not compared.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.11:** Retrying an already-queued run shall
  not be rejected by .2: a retry continues an existing run and is not a second
  wake for the wave.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.12:** When a producer cannot read the
  parent's wave members, it shall queue no run and record no wave identity; the
  backstop shall deliver that wake later.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.13:** When one wave fans out across several
  workflow participants resolving to the same target agent profile, that profile
  shall receive exactly one run, not one per participant. Holding two
  roles is not a second wave.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.14:** A run's recorded wave identity shall
  always describe the wake that run delivers. Where the run queue would otherwise
  merge a later request into a queued run by replacing that run's payload, a
  request carrying a wave identity shall not be merged into any run, and a queued
  run carrying one shall not be merged into; both shall be admitted independently
  and reconciled only by .2. One consequence is accepted: two wakes for *different*
  parents addressed to the same target agent inside the merge window now produce
  two runs, where one previously absorbed and discarded the other.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.15:** A producer shall record a wave
  identity only for a wave-member set it has observed entirely terminal in a
  single read. That read shall be the producer's last read of child task state
  before queueing and shall yield each member's state as well as its id; if any
  member is not terminal in it, the producer shall queue no run and the backstop
  shall deliver the wake later. Recording an identity for a set never observed
  simultaneously terminal would permanently suppress the real wave, because .6
  makes deduplication unbounded.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-002.16:** A producer that queues a wake without
  consulting the workflow engine shall attach the same workflow-authored action
  payload the engine would attach for the parent's current workflow step, so that
  .10 holds for a workflow that authors one. Where it cannot be resolved, the
  producer shall still queue the wake without it and record the omission: the wake
  outranks an optional payload, and that wake is unconditional under .004.2.

### REQ-OFFICE-WAKE-WAVE-IDENTITY-003: Backstop admission compares waves, not time

**Intent:** The reconciler must re-deliver a genuinely lost wake and not
re-deliver a genuinely delivered one. Deciding that by timestamp makes the answer
depend on unrelated edits; deciding it by wave identity makes it depend on the
thing the wake is about.

#### Acceptance criteria

- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.1:** When a terminal
  `task_children_completed` run exists for the parent whose recorded wave identity
  equals the parent's current one, the parent shall not be a candidate for
  re-delivery.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.2:** When a wake has been delivered for the
  current wave and a user then edits a terminal child's title, priority, or
  labels, the parent shall not become a candidate and no run shall be queued.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.3:** When a wake has been delivered and the
  parent's wave-member set subsequently changes to one whose wave identity has not
  already been queued for that parent and target agent, the parent shall become a
  candidate and a run for the new wave shall be queued. A change returning the
  parent to an already-queued identity is the id-set-reuse exclusion under *Out of
  scope*, suppressed by .002.6, and is not required to be re-delivered.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.4:** While a `task_children_completed` run
  for the parent is queued or claimed, the parent shall not be a candidate,
  regardless of which wave that run was queued for.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.5:** A parent with no
  `task_children_completed` run carrying a wave identity shall continue to be
  judged by the timestamp rule its runs were written under, so an upgrade queues
  no re-wake storm.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.6:** Once any `task_children_completed` run
  carrying a wave identity exists for a parent, that parent's admission shall be
  decided by wave identity alone and .5 shall never be consulted for it again,
  whichever of its runs is oldest. The compatibility rule is scoped to the parent,
  not the individual run row.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.7:** A parent whose assignee cannot accept a
  run, and a parent whose delivery evidence is missing, shall be admitted or
  rejected exactly as today. This capability changes only how delivery *for the
  current wave* is recognised, on the terms .10 states.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.8:** Admission shall decide staleness inside
  the candidate query. A parent permanently ineligible because its current wave
  was already delivered shall not be returned and then rejected in the caller,
  where it would occupy a row of the query's fixed result limit on every tick.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.9:** A parent with no wave members shall not
  be returned as a candidate: no producer will ever queue for it (.001.7), so
  returning it creates exactly the permanently rejected candidate .8 forbids. This
  gate shall only remove candidates, never admit one not admitted today.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-003.10:** Each admission clause shall name the
  run statuses it applies to, and those sets shall be the ones in force today: a
  queued or claimed run blocks under .4; a finished, failed, or cancelled run is
  the delivery evidence .1 and .5 compare; a run of any status counts when deciding
  whether a parent has left the compatibility rule in .5. A failed or cancelled
  wake shall therefore keep blocking its parent and, because wave identity no
  longer moves on an unrelated child edit, shall block that wave permanently rather
  than until the next such edit. This is intended: the runtime contract already
  requires an explicit user retry after a terminal execution failure, and the
  edit-driven re-arming it replaces was an artefact of the churn being removed.
  The escape hatches are an explicit retry and any wave-member change.

### REQ-OFFICE-WAKE-WAVE-IDENTITY-004: Every producer, no other behaviour change

**Intent:** A constraint one producer can route around is decorative; at the same
time, three of the four carry behaviour unrelated to wave identity that must
survive untouched.

#### Acceptance criteria

- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.1:** All four producers shall record the
  canonical wave identity on every run they queue.

- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.2:** The Office scheduler cascade shall
  continue to queue a wake for a parent with no active workflow session, and its
  wake shall not become conditional on the engine accepting a trigger.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.3:** The orchestrator's workflow operation
  ledger identifier shall not change, and parent step transitions driven by
  `on_children_completed`, including on a reopened and recompleted child set,
  shall fire exactly as today.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.4:** A non-Office workflow that configures
  `on_children_completed` shall observe no behaviour change other than those
  .002.7, .002.13 and .002.14 define.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.5:** Upgrading shall require no backfill of
  historical runs and shall queue no run as a direct consequence of the upgrade.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.6:** The parent wake receipt shall keep its
  state-inclusive child-set key and its delivery-evidence fields. This capability
  adds a wave identity to runs; it does not redefine the receipt.
- **AC-OFFICE-WAKE-WAVE-IDENTITY-004.7:** The definition of when a parent's
  children are *terminal enough* for a wake shall not change. Which children count
  towards it, and the deliberate archived-child divergence between the
  edge-triggered readiness check and the backstop's candidate query, are outside
  this capability. Readiness and wave identity may disagree about which children
  they count, and the wave-member predicate shall not be propagated into any
  terminality predicate. Exactly two readiness-adjacent additions are in scope and
  both only remove wakes: .001.7 with .003.9, and .002.15.

## Out of scope

- **Re-waking a parent after a child is reopened and completed again.** A child
  going `COMPLETED` → `IN_PROGRESS` → `COMPLETED` presents an identical id set and
  an identical state set at both wave moments, so no function of the current child
  set distinguishes them. Closing this needs a monotone per-child completion epoch
  on `tasks`, a separate capability with a different owner, filed as a follow-up.
  Parent *step transitions* on reopen are unaffected, per .004.3; only the
  redundant run is suppressed.

- **Re-waking a parent whose wave-member id set returns to a previously delivered
  value.** A child that leaves the wave-member set and later rejoins it — plainly,
  archived then unarchived — returns the identity to a delivered value, which the
  unbounded deduplication in .002.6 suppresses. Same root limit and same deferred
  follow-up as the bullet above; .003.3 is qualified to match. Exposure is small:
  unarchiving is deliberate and rare, and any later membership change still wakes
  the parent.

- **The task system's `updated_at` stamping.** Every task write stamps it
  unconditionally and will continue to. This capability removes the wake
  pipeline's dependence on it rather than changing it.

- **The whole-second comparison in the backstop's in-flight run arm**, tracked as
  its own card. This capability replaces only the *staleness* arm; the
  queued-or-claimed arm and its resolution behaviour are untouched.

- **The backstop candidate query's SQLite-only SQL.** Its aggregate child-set
  expression and raw JSON extraction have no PostgreSQL branch; tracked as its own
  follow-up. Work here shall not widen that gap — new SQL carries a dialect branch
  — but closing it is not this capability's outcome.

- **Any change to the `on_children_completed` trigger's firing conditions,
  payload shape, or workflow-editor surface.** Those belong to the task system's
  subtask completion trigger contract. Carrying an extra value alongside a trigger
  to the action it dispatches, and *reading* an already-authored action payload,
  are not changes to that contract and are not excluded.

- **User interface.** No new control, view, or setting. The effect is visible
  only as the absence of a duplicate or spurious run.
