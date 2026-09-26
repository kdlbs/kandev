---
status: draft
system: office
created: 2026-08-25
owners:
  - kandev
---

# Office Review Participant Seats Requirements

## Overview

An Office task at the Review step needs at least one agent seated as a
`reviewer` before the step's fan-out can wake anyone and before its quorum
guard can ever be satisfied. No shipped code path creates one. A task created
by an agent therefore arrives carrying only a `runner` seat, the fan-out wakes
zero agents, the `all_approve` guard has no seat that can ever be filled, and
the task parks forever with no error surfaced anywhere.

This capability makes those seats exist by default. When a task **enters** a
step that declares it, the system resolves which Office agent should fill the
step's participant role and writes a seat for that task at that step, before
the step's own fan-out reads the slate. Casting is **derived**, not configured:
the reviewer pool uses `ceo` and `specialist`, while other roles use `ceo`.
The runner is the fallback when the pool is empty.

Seats are written for the step being **entered**, never for the step the task
stands on when it is created; that distinction is the whole defect behind the
earlier fan-out slate deadlock, and it is why entry is the write point.

Two further things must be true for any of this to reach a running
installation, and neither holds today. A workspace whose Office workflow was
materialized earlier has to gain the new declaration, which is
REQ-OFFICE-REVIEW-SEATS-005 below. And the step's entry sequence has to
actually execute what it declares, which is not Office's contract at all: it is
`REQ-OFFICE-STEP-ENTRY-001`, stated in the sibling requirement
`step-entry-sequence-execution.md` and depended on here. A build satisfying
everything else and neither of those changes nothing on a live instance while
every test passes.

Office owns this contract because Office owns which of its agents are
accountable for reviewing Office work. The task system owns the participant and
quorum primitives written into here, not the casting decision.

## Terminology

- **Participant seat** - a row in the task system's participant store scoped to
  a step and a task, naming one agent profile in one participant role
  (`reviewer`, `approver`, `watcher`, `collaborator`, `runner`) and carrying a
  `decision_required` flag and an integer `position`. Called a **seat** below.
- **Per-task seat** - a seat whose task identifier is the task's own. Every
  seat this capability writes is a per-task seat.
- **Template seat** - a seat whose task identifier is empty, applying to every
  task at a step. This capability writes none; the term is defined only because
  the store supports them and the reader merges them.
- **Gated step** - a step whose configuration declares the seat-ensuring action
  for a participant role. In the shipped Office workflow these are the `review`
  and `approval` steps.
- **Arrival**, **step entry**, **entry identity** and **redelivery** are
  defined by the sibling requirement `step-entry-sequence-execution.md` and
  used here unchanged. Step entry is the unit the at-most-once criteria below
  are counted over.
- **Fan-out** - the existing step action that queues one run per seat in a
  named participant role.
- **Quorum guard** - the existing transition guard counting decisions from
  decision-required seats in a named role.
- **The task's runner** - the agent the task system resolves as currently
  driving the task at a step, by its existing precedence: the task's `runner`
  seat at that step, else the step's primary agent profile, else the task's
  most recently written `runner` seat anywhere. "Most recently written" is
  resolved by a persisted creation timestamp on the seat row, not by physical
  row order; AC-OFFICE-REVIEW-SEATS-002.10 states why that distinction is a
  requirement rather than an implementation detail.
- **Eligible agent** - an Office agent in the task's workspace whose Office
  agent role is in the participant role's pool (`ceo` and `specialist` for the
  reviewer role; `ceo` alone for the approver role and for every other
  participant role, per AC-OFFICE-REVIEW-SEATS-002.1) and whose status is
  neither `stopped` nor `pending_approval`.
- **Warning record** - the structured record accompanying each counter
  increment in REQ-OFFICE-REVIEW-SEATS-004, carrying the identifiers a counter
  label may not.

## Requirements

### REQ-OFFICE-REVIEW-SEATS-001: Seats are ensured when a gated step is entered

Entering a gated step shall leave that step holding at least one seat in the
declared participant role for the entering task, or shall record that it could
not. Entry is the only write point.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-SEATS-001.1:** When a task enters a gated step, the system
  shall resolve and write seats for the declared participant role, scoped to
  that step and that task, before that step's fan-out for the same role reads
  the slate on that entry. A declaration ordered after that fan-out does not
  satisfy this criterion.
- **AC-OFFICE-REVIEW-SEATS-001.2:** Every seat the system writes shall be
  marked decision-required.
- **AC-OFFICE-REVIEW-SEATS-001.3:** When a task enters the same gated step
  again, the system shall not create a second seat for an already-seated
  combination of step, task, participant role and agent profile; the number of
  seat rows for that combination shall remain one.
- **AC-OFFICE-REVIEW-SEATS-001.4:** When two entries for the same task and step
  are processed concurrently, exactly one seat row shall exist afterwards for
  each resolved combination of step, task, participant role and agent profile.
- **AC-OFFICE-REVIEW-SEATS-001.5:** When a seat in the declared role already
  exists for that task anywhere in the task's workflow, the system shall write
  no further seats for that role and shall leave the existing seat's agent
  profile, decision-required flag and position unchanged.
- **AC-OFFICE-REVIEW-SEATS-001.6:** The system shall write its seat at
  `position` zero, so that a seat it writes never displaces an
  operator-supplied seat's ordering.
- **AC-OFFICE-REVIEW-SEATS-001.7:** When a step does not declare the
  seat-ensuring action, entering it shall write no seats.
- **AC-OFFICE-REVIEW-SEATS-001.8:** When the seat-ensuring action is declared
  but its supporting capability is unavailable in the running deployment,
  entering the step shall write no seats, not fail the entry, and emit the
  unfillable signal of REQ-OFFICE-REVIEW-SEATS-004.
- **AC-OFFICE-REVIEW-SEATS-001.9:** The system shall write no seat whose task
  identifier is empty.
- **AC-OFFICE-REVIEW-SEATS-001.10:** When two seats in the same role carry the
  same `position`, the system shall order them by agent profile identifier
  ascending.
- **AC-OFFICE-REVIEW-SEATS-001.11:** When a step declares the seat-ensuring
  action with a missing, empty or unrecognized participant role, the system
  shall write no seat for that declaration, shall emit the unfillable signal of
  REQ-OFFICE-REVIEW-SEATS-004 with a distinct outcome, and shall not fail the
  step entry.
- **AC-OFFICE-REVIEW-SEATS-001.12:** When a step declares the seat-ensuring
  action more than once for the same participant role, the entry shall write at
  most one seat for that role and shall emit at most one record for it.

### REQ-OFFICE-REVIEW-SEATS-002: Casting resolution is deterministic

Two entries with the same workspace state shall resolve the same agents in the
same order. Resolution is derived from the workspace's agents; nothing is
configured.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-SEATS-002.1:** The system shall resolve the candidate list
  for a participant role to the task's workspace agents whose agent role is in
  that participant role's pool — `ceo` and `specialist` for the reviewer role;
  `ceo` alone for the approver role and for every other participant role —
  ordered by `created_at` ascending and then by agent profile identifier
  ascending.
- **AC-OFFICE-REVIEW-SEATS-002.2:** The system shall exclude from the candidate
  list any workspace agent whose status is `stopped` or `pending_approval`,
  and shall do so without changing the behavior of any other caller of the
  agent listing it uses.
- **AC-OFFICE-REVIEW-SEATS-002.3:** When the candidate list holds more than one
  eligible agent, the system shall seat the first member that is neither the
  task's runner nor already seated in another participant role on the task.
  This exclusion is best-effort and shall not leave the seat empty: when no
  such member exists, the system shall fall back to seating the first member,
  or the second member when the first is the task's runner.
- **AC-OFFICE-REVIEW-SEATS-002.4:** When the candidate list holds exactly one
  eligible agent and that agent is the task's runner, the system shall seat
  that agent and shall record the self-review.
- **AC-OFFICE-REVIEW-SEATS-002.5:** When the candidate list is empty, the
  system shall seat the task's runner and record both the fallback and, where
  it applies, the self-review.
- **AC-OFFICE-REVIEW-SEATS-002.6:** When the candidate list is empty and the
  task's runner does not resolve, the system shall write no seat and shall emit
  the unfillable signal of REQ-OFFICE-REVIEW-SEATS-004.
- **AC-OFFICE-REVIEW-SEATS-002.7:** The system shall seat exactly one agent per
  gated step entry per participant role, whether or not two entries for that
  task and step resolve different agents.
- **AC-OFFICE-REVIEW-SEATS-002.8:** When resolution fails with an error rather
  than an empty result, the system shall write no seat, emit the unfillable
  signal, and not fail the step entry.
- **AC-OFFICE-REVIEW-SEATS-002.9:** When the candidate list holds exactly one
  eligible agent and that agent is not the task's runner, the system shall
  seat that agent, record the seating as coming from the eligible pool rather
  than the fallback, and record the self-review when that agent is already
  seated in another participant role on the task. This criterion's domain
  (exactly one non-runner candidate) is disjoint from
  AC-OFFICE-REVIEW-SEATS-002.3's (more than one candidate); the two do not
  compete over the same input.
- **AC-OFFICE-REVIEW-SEATS-002.10:** Resolution of the task's runner shall
  produce the same result on every database engine the product supports, and
  its final precedence tier shall order candidate seats by a persisted column
  that exists on all of them, with a named tiebreak when two rows carry the
  same value. Ordering by a physical row identifier, or by an identifier whose
  value carries no ordering, shall not satisfy this criterion.

### REQ-OFFICE-REVIEW-SEATS-003: Woken and counted seats agree

Every seat this capability writes shall be visible to both readers. A seat that
is woken and never counted stalls a gate more quietly than an empty one does.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-SEATS-003.1:** Every seat the system writes at a gated
  step shall be woken by that step's fan-out for its role and shall be counted
  by that step's quorum guard for the same role.
- **AC-OFFICE-REVIEW-SEATS-003.2:** A redelivery of a step entry shall not
  queue a second run for a seat whose run for that entry is already queued.
- **AC-OFFICE-REVIEW-SEATS-003.3:** The fan-out shall wake seats in ascending
  `position` order and then ascending agent profile identifier order, and that
  order shall hold in the sequence the fan-out iterates, not only in the order
  the underlying query returns.
- **AC-OFFICE-REVIEW-SEATS-003.4:** Clearing prior decisions on step entry
  shall continue to run before seats are woken.
- **AC-OFFICE-REVIEW-SEATS-003.5:** A seat written by the existing manual
  per-task participant path shall be woken once and counted once, and shall not
  be duplicated by the seat-ensuring action, including when that seat was
  written while the task stood at a different step of the same workflow.
- **AC-OFFICE-REVIEW-SEATS-003.6:** When a task leaves a gated step and later
  enters it again, that second entry shall queue a fresh run for each seat in
  the declared role, so that a rejected and reworked task is reviewed again.

### REQ-OFFICE-REVIEW-SEATS-004: An unfillable or stalled gate is observable

Whatever remains unfillable after this capability ships shall announce itself.
The failure being removed here was invisible; the residual cases must not be.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-SEATS-004.1:** When a gated step entry resolves no agent
  to seat, the system shall emit a warning record and increment a counter,
  identifying the workspace, the task, the step and the participant role.
- **AC-OFFICE-REVIEW-SEATS-004.2:** When a quorum guard resolves no
  decision-required seat in its role, the system shall emit a warning record
  and increment a counter, identifying the task, the step and the role.
- **AC-OFFICE-REVIEW-SEATS-004.3:** When a seat exists but its agent profile no
  longer resolves, the system shall emit a warning record and increment a
  counter identifying the task, the step, the role and the unresolved agent,
  and shall continue evaluating the remaining seats rather than fail the
  guard.
- **AC-OFFICE-REVIEW-SEATS-004.4:** When the seated agent is the task's runner,
  the system shall record the self-review.
- **AC-OFFICE-REVIEW-SEATS-004.5:** When the seated agent came from the empty
  candidate list fallback rather than from an eligible agent, the system shall
  record that provenance.
- **AC-OFFICE-REVIEW-SEATS-004.6:** Counter labels shall be drawn only from
  bounded dimensions; the system shall not label a counter with a task, step or
  agent identifier.
- **AC-OFFICE-REVIEW-SEATS-004.7:** A single gated step entry shall emit at
  most one record per participant role for each condition detected while that
  entry's declared actions run - AC-OFFICE-REVIEW-SEATS-004.1, and the
  malformed-declaration and repeated-declaration conditions of
  AC-OFFICE-REVIEW-SEATS-001.11 and AC-OFFICE-REVIEW-SEATS-001.12.
- **AC-OFFICE-REVIEW-SEATS-004.10:** The conditions of
  AC-OFFICE-REVIEW-SEATS-004.2 and AC-OFFICE-REVIEW-SEATS-004.3 are detected
  when the quorum guard is evaluated, and the guard is evaluated repeatedly
  within one step entry. For those two conditions the system shall emit at most
  one record per participant role per guard evaluation, and shall not emit a
  second record for the same role and the same unresolved agent within a single
  evaluation.
- **AC-OFFICE-REVIEW-SEATS-004.11:** No counter label shall take an
  operator-supplied value. Where a condition reports that a declared
  participant role is missing, empty or unrecognized, the role label shall take
  a single fixed value reserved for that case, and the operator-supplied string
  shall appear only as a typed field on the warning record.
- **AC-OFFICE-REVIEW-SEATS-004.8:** Every warning record required above shall
  be emitted through the backend's structured logger at warning level, carry a
  stable event name identifying which condition fired, and carry that
  condition's identifiers as separate typed fields rather than interpolated
  into a message.
- **AC-OFFICE-REVIEW-SEATS-004.9:** When a condition arises from an error
  rather than from an empty result, its warning record shall carry the error
  and its counter shall use an outcome value distinct from the empty-result
  outcome.

### REQ-OFFICE-REVIEW-SEATS-005: Agent-created tasks reach a fillable Review

This is the outcome the capability exists for, stated without reference to how
it is achieved.

#### Acceptance criteria

- **AC-OFFICE-REVIEW-SEATS-005.1:** An Office task created by an agent, in a
  workspace holding at least one agent, shall arrive at the Review step with at
  least one decision-required `reviewer` seat, with no manual database work.
- **AC-OFFICE-REVIEW-SEATS-005.2:** That reviewer shall have a run queued for
  it on that step entry.
- **AC-OFFICE-REVIEW-SEATS-005.3:** The Review step's quorum guard shall count
  at least one decision-required seat for that task.
- **AC-OFFICE-REVIEW-SEATS-005.4:** The same shall hold for the `approver` role
  at the Approval step.
- **AC-OFFICE-REVIEW-SEATS-005.5:** A task already parked at a gated step
  before this capability ships shall gain seats when it next enters one, and
  shall not be seated retroactively while it stands still.
- **AC-OFFICE-REVIEW-SEATS-005.6:** A workspace whose Office workflow was
  materialized before this capability shipped shall, without any manual
  database work and without being re-onboarded, have its gated steps declaring
  the seat-ensuring action, so that AC-OFFICE-REVIEW-SEATS-005.1 holds there
  and not only in a workspace created afterwards.
- **AC-OFFICE-REVIEW-SEATS-005.7:** Bringing an existing workspace's gated
  steps up to date shall preserve every other action already declared on those
  steps, shall be safe to repeat, and shall not modify a workflow the operator
  has taken ownership of.
- **AC-OFFICE-REVIEW-SEATS-005.8:** Bringing a gated step up to date shall
  place the seat-ensuring action immediately before the first fan-out action
  declaring the same participant role, or at the head of the entry sequence
  when that step declares no such fan-out; and for a step whose other actions
  are unmodified the resulting sequence shall equal, in content and in order,
  the one a workspace materialized afterwards receives.
- **AC-OFFICE-REVIEW-SEATS-005.9:** When a step's entry sequence is changed by
  another writer between the moment it is read and the moment it is written
  back, the system shall not overwrite that change; it shall re-read and apply
  its insertion on top of the observed state, or leave the step unchanged and
  record that it did. Neither outcome shall prevent the system from
  starting.

## Out of scope

Each exclusion below is a contract, not an omission.

- **A configurable review slate, and any settings surface for it.** Casting is
  derived here. A per-workspace ordered list of reviewers, its API, and the
  settings screen with its operator authorization model and translated copy is
  a separate capability, deliberately not bundled into the fix for a parked
  gate. The derived rule is the default that surface would later override.
- **Template seats.** This capability writes no seat with an empty task
  identifier, and does not activate the store's unused template-seat writer.
- **Retroactively seating tasks that are already parked.** A task that already
  entered a gated step has already run that step's entry sequence; seating it
  afterwards wakes nobody. Such a task is recovered by re-entering the step.
  There is no sweep over tasks. This exclusion is about **tasks** only:
  bringing an existing workspace's step *configuration* up to date is required
  by AC-OFFICE-REVIEW-SEATS-005.6 and is not a sweep, because it touches
  workflow rows once at startup and touches no task.
- **Widening either pool beyond the roles AC-OFFICE-REVIEW-SEATS-002.1 names.**
  The reviewer pool is `ceo` and `specialist`; the approver pool, and every
  other participant role's pool, stays `ceo` alone — a per-role ruling, not a
  general relaxation. Office agent roles and participant roles remain disjoint
  vocabularies otherwise: the shipped `security` role's description does
  mention approving or blocking at review gates, so admitting it is a live
  choice rather than a vacuous one; widening either pool further belongs to
  the configurable-slate capability.
- **The `backlog` step declaring no events.** A task started there has no
  turn-complete and no children-completed handler and parks permanently. Real,
  and not fixed by seats.
- **Human reviewers.** Seats reference agent profiles only.
- **Quorum thresholds.** `all_approve` and `any_reject` stay as configured.
- **Reviewer instruction content.** What the woken reviewer is told to do is
  the run's prompt, not this contract.
- **Non-Office workflows.** A step in another workflow declaring the action
  behaves identically, but no other shipped workflow declares it and none is
  changed here.
- **Changing Office agent roles**, including adding a reviewer role.
- **Entry-sequence execution itself.** This capability depends on
  `REQ-OFFICE-STEP-ENTRY-001` and does not restate it. Its scope, and the
  exclusion of every trigger other than step entry, belong to that
  requirement.
