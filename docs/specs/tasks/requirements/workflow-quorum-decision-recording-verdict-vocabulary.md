---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Quorum verdict vocabulary Requirements



## Overview



Human and agent verdicts use compatible meanings so approvals and rejections produce the expected quorum results.

## Requirements


### REQ-TASKS-QUORUM-VERDICT-001: Quorum verdict vocabulary



**Intent:** Human and agent verdicts use compatible meanings so approvals and rejections produce the expected quorum results.

#### Acceptance criteria

#### Verdict vocabulary

- **AC-TASKS-QUORUM-VERDICT-001.1:** THE SYSTEM SHALL treat `changes_requested` as a rejection wherever
  `rejected` is treated as one, so that `any_reject` is satisfied by either.
- **AC-TASKS-QUORUM-VERDICT-001.2:** WHEN a human records a decision through
  `POST /tasks/:id/request-changes`, THE SYSTEM SHALL produce a stored verdict
  that satisfies `any_reject` for the guard evaluating at that step, whatever
  role that guard names. AC-TASKS-QUORUM-VERDICT-001.1 alone does not achieve this; AC-TASKS-QUORUM-VERDICT-001.3 and AC-TASKS-QUORUM-VERDICT-001.7 are
  the other halves.
- **AC-TASKS-QUORUM-VERDICT-001.3:** THE SYSTEM SHALL satisfy `any_reject` when, among decisions
  recorded at the evaluating step for the guard's role, the LAST row per decider
  identity `(decider_type, decider_id)` under the AC-TASKS-QUORUM-CONCURRENCY-001.1 ordering is a rejection
  — whether or not that decider's `participant_id` is in the required slate. A
  rejection is a veto: one is sufficient and it does not require a quorum seat.
  This is what makes a singleton-user rejection countable, since that decider
  has no `agent_profile_id` and therefore no participant row, and is written
  with the `userParticipantSentinel` participant id.
- **AC-TASKS-QUORUM-VERDICT-001.4:** THE SYSTEM SHALL key the AC-TASKS-QUORUM-VERDICT-001.3 veto scan on the latest verdict per
  decider, NOT on the presence of any rejection row. A decider who rejects and
  then records an approval at the same step SHALL NOT continue to veto. Stated
  because the naive reading — scan for a rejection — reintroduces the defect
  named under `## Persistence guarantees`: the engine's `DecisionInfo`
  projection carries no `superseded_at`, so "non-superseded" is not a predicate
  the evaluator can express, and only last-row-wins is.
- **AC-TASKS-QUORUM-VERDICT-001.5:** THE SYSTEM SHALL extend the engine's `DecisionInfo` projection to
  carry `decider_type`, `decider_id`, `role`, and `comment`. AC-TASKS-QUORUM-VERDICT-001.3 needs decider
  identity, AC-TASKS-QUORUM-BINDING-001.7 needs `role`, and AC-TASKS-QUORUM-BINDING-001.3 needs `comment`; the projection today
  carries only `participant_id`, `decision`, and `note`, so none of the three is
  satisfiable without this. The underlying columns already exist and
  `ListStepDecisions` already selects them.
- **AC-TASKS-QUORUM-VERDICT-001.6:** THE SYSTEM SHALL count only slate members, per AC-TASKS-QUORUM-SLATE-001.9, toward
  `all_approve`, `all_decide`, `majority_approve`, and `n_approve:<N>`. An
  approval is a quorum contribution and does require a seat. AC-TASKS-QUORUM-VERDICT-001.3 is scoped to
  `any_reject` alone, so a non-slate approval never advances a task.
- **AC-TASKS-QUORUM-VERDICT-001.7:** THE SYSTEM SHALL evaluate the AC-TASKS-QUORUM-VERDICT-001.3 veto over decisions with
  `decider_type = user` WITHOUT filtering on the guard's role, while continuing
  to filter agent decisions by role per AC-TASKS-QUORUM-BINDING-001.7. The singleton human's stored
  `role` SHALL remain whatever `resolveDeciderRole` returns for a user caller —
  today the constant `approver`, which `## Out of scope` leaves unchanged — and
  the veto SHALL NOT depend on it.
  Without this, AC-TASKS-QUORUM-VERDICT-001.2 is unsatisfiable at every reviewer-guarded step, which is
  the step the motivating card in `## Why` is stuck at: `resolveDeciderRole`
  sets `hasApprover = true` unconditionally for `decider_type = user` and
  returns on the approver-wins branch, so a human clicking **Request changes**
  at `Office Default`'s Review — whose guards name `role: reviewer` — writes
  `role = approver`, and AC-TASKS-QUORUM-BINDING-001.7 discards it before AC-TASKS-QUORUM-VERDICT-001.3 ever sees it.
  The alternative, stamping the human's decision with the evaluating guard's
  role, is rejected rather than merely unchosen: a decision is recorded once
  while a step may configure several guards, so "the guard's role" is not well
  defined at write time. The human is the operator of the board rather than a
  seated participant, so a human rejection is a veto over the step, not a
  role-scoped quorum contribution. AC-TASKS-QUORUM-VERDICT-001.6 already denies a seatless approval any
  effect, so this asymmetry touches rejections only.
  Boundary, stated so it is a decision rather than an accident: when a step
  configures `any_reject` guards under two DIFFERENT roles, one human rejection
  satisfies both at once, because the veto is role-agnostic by construction.
  That is intended — the human is the operator of the board and is rejecting the
  step, not a role's share of it — and AC-TASKS-QUORUM-REEVALUATION-001.6 still means only the first
  satisfied transition in configured order is applied. `Office Default` does not
  configure this shape; nothing forbids it.
- **AC-TASKS-QUORUM-VERDICT-001.8:** THE SYSTEM SHALL continue to accept and store verdict strings
  outside the recognized set without error, counting them toward `all_decide`
  and toward neither `all_approve` nor `any_reject` — preserving the documented
  free-form behavior at `workflow/engine/quorum.go:22-24`.
- **AC-TASKS-QUORUM-VERDICT-001.9:** THE SYSTEM SHALL preserve the existing wire value written by the
  human `request-changes` path, so the frontend DTO union
  `"approved" | "changes_requested"` continues to typecheck unchanged.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.