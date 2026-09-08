---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Quorum step binding and thresholds Requirements



## Overview



A verdict remains bound to the validated step and threshold arithmetic stays explicit at boundary conditions.

## Requirements


### REQ-TASKS-QUORUM-BINDING-001: Quorum step binding and thresholds



**Intent:** A verdict remains bound to the validated step and threshold arithmetic stays explicit at boundary conditions.

#### Acceptance criteria

#### Step binding and threshold boundaries

- **AC-TASKS-QUORUM-BINDING-001.1:** THE SYSTEM SHALL bind a decision to the task's `workflow_step_id`
  read during validation, and SHALL write the row inside the same transaction
  that supersedes the participant's prior verdict for that step.
- **AC-TASKS-QUORUM-BINDING-001.2:** WHEN the task's step changes between validation and write, THE
  SYSTEM SHALL persist the decision against the **validated** step — not reject
  it, and not re-target it at the new step. The recorded row SHALL NOT be
  counted at the new step, the call SHALL report success, `transition_applied`
  SHALL be `false`, and the returned `step_id` SHALL be the validated step so
  the caller can see its verdict landed on a step the task has left. Persisting
  is chosen over rejecting because the reviewer's verdict is real work and
  discarding it would silently lose a review; the returned `step_id` is what
  makes the outcome detectable.
- **AC-TASKS-QUORUM-BINDING-001.3:** THE SYSTEM SHALL write the caller's reason to
  `workflow_step_decisions.comment`.
- **AC-TASKS-QUORUM-BINDING-001.4:** THE SYSTEM SHALL satisfy `majority_approve` only when approvals
  strictly exceed half the required slate, so a slate of 2 requires 2 approvals
  and a slate of 3 requires 2 — preserving `approveCount*2 > totalRequired`
  (`workflow/engine/quorum.go:113`).
- **AC-TASKS-QUORUM-BINDING-001.5:** WHEN `n_approve:<N>` names an N greater than the required slate size,
  THE SYSTEM SHALL never fire that transition and SHALL report AC-TASKS-QUORUM-DIAGNOSTICS-001.1 reason
  `threshold_unsatisfiable` rather than an error. It is deliberately NOT
  `threshold_not_met`: no future decision can satisfy it, so it is a stuck card,
  and AC-TASKS-QUORUM-DIAGNOSTICS-001.6 requires stuck to look different from waiting.
- **AC-TASKS-QUORUM-BINDING-001.6:** WHEN `n_approve:<N>` names a non-numeric or non-positive N, THE
  SYSTEM SHALL not fire the transition and SHALL report AC-TASKS-QUORUM-DIAGNOSTICS-001.1 reason
  `threshold_unrecognized`. The threshold family is recognized here and only its
  parameter is malformed, so this is not a guard-variant failure;
  `guard_variant_unrecognized` is reserved for the case AC-TASKS-QUORUM-DIAGNOSTICS-001.8 names.
- **AC-TASKS-QUORUM-BINDING-001.7:** WHEN a decision exists for a role other than the guard's role at the
  same step, THE SYSTEM SHALL ignore it for that guard — EXCEPT for the
  `decider_type = user` veto of AC-TASKS-QUORUM-VERDICT-001.7, which is deliberately role-agnostic.
  AC-TASKS-QUORUM-BINDING-001.7 governs seated, role-scoped quorum contributions; the human veto is not
  one.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.