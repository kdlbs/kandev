---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Quorum participant slate Requirements



## Overview



The quorum evaluator uses the canonical required participants for the task and evaluating step.

## Requirements


### REQ-TASKS-QUORUM-SLATE-001: Quorum participant slate



**Intent:** The quorum evaluator uses the canonical required participants for the task and evaluating step.

#### Acceptance criteria

#### Participant slate

- **AC-TASKS-QUORUM-SLATE-001.1:** WHEN a quorum guard evaluates at a step, THE SYSTEM SHALL count a
  **per-task** participant row (`task_id != ''`) with `decision_required = 1`
  and a matching role even when that row was attached under a different step of
  the same workflow.
- **AC-TASKS-QUORUM-SLATE-001.2:** THE SYSTEM SHALL NOT extend cross-step counting to template-level
  rows (`task_id = ''`). A template row is bound to one step by workflow design,
  so it remains visible only at its own step. Without this, a Work-step template
  reviewer would be pulled into a Review-step quorum.
- **AC-TASKS-QUORUM-SLATE-001.3:** THE SYSTEM SHALL identify a participant by the natural key
  `(task_id, role, agent_profile_id)`, and SHALL select one **canonical row**
  per key: the row whose `step_id` equals the evaluating step when such a row
  exists, otherwise the row with the lowest `id` in ASCII order among that key's
  rows. `workflow_step_participants` carries no timestamp column, so `id` ASC is
  the tiebreak — "most recent" is not expressible here. The same rule SHALL be
  used by slate construction (AC-TASKS-QUORUM-SLATE-001.1), by role resolution (AC-TASKS-QUORUM-RECORDING-001.5), and by the
  `participant_id` written on a decision (AC-TASKS-QUORUM-RECORDING-001.2), so that the read side and the
  write side cannot disagree. AC-TASKS-QUORUM-SLATE-001.3 is step 3 of AC-TASKS-QUORUM-SLATE-001.9; the id that finally
  identifies a participant is the one surviving AC-TASKS-QUORUM-SLATE-001.9 step 4, because AC-TASKS-QUORUM-SLATE-001.5's
  template-versus-per-task collapse keys on `(role, agent_profile_id)` and can
  still discard an AC-TASKS-QUORUM-SLATE-001.3 canonical row.
- **AC-TASKS-QUORUM-SLATE-001.4:** WHEN both a step-specific row and a differently-stepped row exist
  for the same `(task_id, role, agent_profile_id)`, THE SYSTEM SHALL count the
  participant exactly once, as the AC-TASKS-QUORUM-SLATE-001.3 canonical row. This shape is produced
  in normal operation: `AddTaskParticipant`'s idempotency probe is scoped to
  `step_id`, so re-attaching the same agent after a step change inserts a second
  row rather than matching the first.
- **AC-TASKS-QUORUM-SLATE-001.5:** THE SYSTEM SHALL continue to honor template-level rows
  (`task_id = ''`) with per-task rows taking precedence on
  `(role, agent_profile_id)`, per `phase2_sqlite.go:20-27`.
- **AC-TASKS-QUORUM-SLATE-001.6:** WHEN the required slate for the guard's role is empty AND the
  guard's threshold is an approve-style one (`all_approve`, `all_decide`,
  `majority_approve`, `n_approve:<N>`), THE SYSTEM SHALL NOT fire the
  transition, and SHALL report the empty slate distinctly from an unmet
  threshold per AC-TASKS-QUORUM-DIAGNOSTICS-001.1. The scoping to approve-style thresholds is required by
  AC-TASKS-QUORUM-SLATE-001.11; an unscoped empty-slate short-circuit makes the AC-TASKS-QUORUM-VERDICT-001.3 veto unreachable.
- **AC-TASKS-QUORUM-SLATE-001.7:** WHEN a decision is on file for a participant who WAS in the required
  slate and has since been removed, THE SYSTEM SHALL ignore that decision for
  the counting thresholds, preserving the documented mid-flight removal
  behavior in `applyThreshold`. This governs removed slate members only; it does
  NOT govern deciders who never held a seat, whose rejections are handled by
  AC-TASKS-QUORUM-VERDICT-001.3 and whose approvals are excluded by AC-TASKS-QUORUM-VERDICT-001.6.

- **AC-TASKS-QUORUM-SLATE-001.8:** THE SYSTEM SHALL extend the engine's `ParticipantStore` port with a
  task-scoped read — `ListTaskParticipants(ctx, taskID)`, returning every
  per-task row (`task_id = ?`) for that task irrespective of `step_id` — and
  SHALL leave `ListStepParticipants(ctx, stepID, taskID)` unchanged. AC-TASKS-QUORUM-SLATE-001.1's
  cross-step counting is not expressible through the existing port, whose only
  production query is `WHERE step_id = ? AND (task_id = '' OR task_id = ?)`;
  widening that predicate in place would also pull template rows across steps,
  which AC-TASKS-QUORUM-SLATE-001.2 forbids. A task with no per-task rows SHALL yield an empty list, not an
  error, matching `ListStepParticipants`'s documented empty-is-valid contract.
  This is the participant-side twin of AC-TASKS-QUORUM-VERDICT-001.5 — the projection and the port both
  have to be named, or the builder invents them.
- **AC-TASKS-QUORUM-SLATE-001.9:** THE SYSTEM SHALL build the required slate for a guard, at the
  evaluating step, in exactly this order:
  1. **Gather** — per-task rows for the task via AC-TASKS-QUORUM-SLATE-001.8 (any step), plus
     template rows (`task_id = ''`) at the evaluating step only, per AC-TASKS-QUORUM-SLATE-001.2.
  2. **Filter** — keep rows whose `role` matches the guard and whose
     `decision_required` is true, per `requiredParticipants`.
  3. **Canonicalize** — collapse to one row per `(task_id, role,
     agent_profile_id)` by AC-TASKS-QUORUM-SLATE-001.3.
  4. **Collapse** — collapse across `task_id` to one row per
     `(role, agent_profile_id)`, the per-task row winning, per AC-TASKS-QUORUM-SLATE-001.5.

  The row surviving step 4 is THE **seat**; its `id` is the slate id that AC-TASKS-QUORUM-RECORDING-001.2
  writes and AC-TASKS-QUORUM-CONCURRENCY-001.1 counts. The order is stated because AC-TASKS-QUORUM-SLATE-001.3 keys on
  `(task_id, role, agent_profile_id)` while AC-TASKS-QUORUM-SLATE-001.5 keys on
  `(role, agent_profile_id)`: running AC-TASKS-QUORUM-SLATE-001.3 alone seats one agent twice when it
  holds both a template row and a per-task row, inflating `totalRequired` so
  `all_approve` can never be met, and running AC-TASKS-QUORUM-SLATE-001.5 alone discards AC-TASKS-QUORUM-SLATE-001.3's
  cross-step canonicalization. Boundary: a row whose `agent_profile_id` is empty
  has no identity to canonicalize on and SHALL be kept as its own seat, keyed by
  its row `id`, at both step 3 and step 4 — collapsing two such rows into one
  seat would under-count `totalRequired` and let `all_approve` fire on a single
  approval.
- **AC-TASKS-QUORUM-SLATE-001.10:** WHEN counting decisions against the slate, THE SYSTEM SHALL map each
  decision to at most one AC-TASKS-QUORUM-SLATE-001.9 seat: by `participant_id` when that id is a seat
  id, otherwise by the seat whose `(role, agent_profile_id)` equals the
  decision's `(role, decider_id)` when `decider_type` = `agent`. A decision that
  maps to no seat is not counted toward any approve-style threshold, per AC-TASKS-QUORUM-VERDICT-001.6;
  `any_reject` does not use this mapping at all, per AC-TASKS-QUORUM-VERDICT-001.3. The fallback exists
  because a participant's AC-TASKS-QUORUM-SLATE-001.3 canonical row can CHANGE after that participant
  has already decided — AC-TASKS-QUORUM-SLATE-001.4 states the two-row shape arises in normal
  operation, so attaching a current-step row for an agent who decided while only
  an earlier-step row existed flips the seat id, and an id-only match would
  silently discard a real verdict, which is the discard class AC-TASKS-QUORUM-RECORDING-001.2 exists to
  prevent. AC-TASKS-QUORUM-SLATE-001.7 is unaffected: a participant removed from the slate entirely has
  no seat under either match, so its decision is still ignored.
- **AC-TASKS-QUORUM-SLATE-001.11:** WHEN the required slate is empty, THE SYSTEM SHALL still evaluate an
  `any_reject` guard and SHALL NOT short-circuit it. The empty-slate fail-closed
  of AC-TASKS-QUORUM-SLATE-001.6 exists because `all_approve` is `approveCount == totalRequired`,
  which is vacuously TRUE at `0 == 0` — an empty slate would otherwise advance a
  task nobody reviewed. `any_reject` is `rejectCount > 0` and cannot fire
  vacuously, so the same guard rail is unnecessary there and actively harmful:
  it makes the AC-TASKS-QUORUM-VERDICT-001.3 seatless veto unreachable in exactly the case where every
  decider is seatless. Implementation consequence, stated because this is not a
  free change: `evaluateWaitForQuorum` returns at `len(required) == 0` before
  `ListStepDecisions` is called, so the empty-slate check must become
  threshold-aware rather than remaining a precondition.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.