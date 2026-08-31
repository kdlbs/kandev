---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Agent decision recording Requirements



## Overview



An Office participant can submit a validated verdict with a reason, and the result is tied to the task and workflow step used for validation.

## Requirements


### REQ-TASKS-QUORUM-RECORDING-001: Agent decision recording



**Intent:** An Office participant can submit a validated verdict with a reason, and the result is tied to the task and workflow step used for validation.

#### Acceptance criteria

#### Recording a decision (agent surface)

- **AC-TASKS-QUORUM-RECORDING-001.1:** WHEN an Office agent session is attached to the task's current step
  with role `reviewer` or `approver` and `decision_required = 1`, THE SYSTEM
  SHALL expose an MCP tool that records a decision, accepting a verdict of
  `approved` or `rejected` and a required non-empty reason.
- **AC-TASKS-QUORUM-RECORDING-001.2:** WHEN that tool is called with a valid verdict, THE SYSTEM SHALL write
  one row to `workflow_step_decisions` with `task_id` = the calling session's
  task, `step_id` = that task's current `workflow_step_id`, `role` = the
  caller's resolved role, `decider_type` = `agent`, `decider_id` = the caller's
  agent profile id, and `participant_id` = the id of the **seat** the participant occupies, as
  defined by AC-TASKS-QUORUM-SLATE-001.9 — that is, AC-TASKS-QUORUM-SLATE-001.3 canonicalization followed by AC-TASKS-QUORUM-SLATE-001.5 collapse,
  not AC-TASKS-QUORUM-SLATE-001.3 alone. The written `participant_id` SHALL be the same id
  the quorum evaluator uses for that participant in the required slate; a
  decision whose `participant_id` is not the canonical id is a defect, because
  `latestDecisionsPerParticipant` silently discards it.
- **AC-TASKS-QUORUM-RECORDING-001.3:** WHEN the caller is not a participant on the task with
  `decision_required = 1` for `reviewer` or `approver`, THE SYSTEM SHALL reject
  the call with a permission error and write no row.
- **AC-TASKS-QUORUM-RECORDING-001.4:** WHEN the caller holds both `reviewer` and `approver` seats at
  the task's current `workflow_step_id`, THE SYSTEM SHALL record the decision
  under `approver`. WHEN the caller holds those two roles at different steps,
  THE SYSTEM SHALL record the decision under the role whose seat sits at the
  task's current `workflow_step_id` (which is guaranteed to be one of the two
  roles by the precondition of AC-TASKS-QUORUM-RECORDING-001.1), and SHALL NOT
  apply approver-wins across steps.
- **AC-TASKS-QUORUM-RECORDING-001.4a:** THE SYSTEM SHALL apply
  AC-TASKS-QUORUM-RECORDING-001.4 to the agent decision surface only. The human decision path
  resolves the role step-blind and retains unconditional approver-wins, so the
  two surfaces MAY record a caller holding both roles under different roles for
  the same task and step. This divergence is deliberate and is recorded here
  rather than left implicit.
- **AC-TASKS-QUORUM-RECORDING-001.5:** THE SYSTEM SHALL resolve the caller's role over the same
  participant population that AC-TASKS-QUORUM-SLATE-001.9 builds, not over rows scoped to the
  task's current step alone. `ListAllTaskParticipants` is step-scoped today, so
  an approver attached at an earlier step is currently unresolvable at the step
  where the approval is due; role resolution and slate membership SHALL NOT
  disagree about who participates.
- **AC-TASKS-QUORUM-RECORDING-001.6:** WHEN the verdict is any value other than `approved` or `rejected`,
  THE SYSTEM SHALL reject the call with a validation error and write no row.
- **AC-TASKS-QUORUM-RECORDING-001.7:** WHEN the reason is empty or whitespace-only, THE SYSTEM SHALL reject
  the call with a validation error and write no row.
- **AC-TASKS-QUORUM-RECORDING-001.8:** WHEN the task has no `workflow_step_id` bound, THE SYSTEM SHALL
  reject the call with an error naming the unbound step and write no row.
- **AC-TASKS-QUORUM-RECORDING-001.9:** THE SYSTEM SHALL NOT expose the decision tool to sessions whose
  profile does not enable `SurfaceOfficeTask`.

- **AC-TASKS-QUORUM-RECORDING-001.10:** THE SYSTEM SHALL validate the decision tool's preconditions in this
  order: (1) the task has a bound `workflow_step_id` (AC-TASKS-QUORUM-RECORDING-001.8), (2) the caller
  resolves to a participant with `decision_required = 1` for `reviewer` or
  `approver` (AC-TASKS-QUORUM-RECORDING-001.3), (3) the verdict is `approved` or `rejected` (AC-TASKS-QUORUM-RECORDING-001.6), (4) the
  reason is non-empty (AC-TASKS-QUORUM-RECORDING-001.7). The step check precedes the permission check
  because `AddTaskParticipant` silently no-ops when a task has no bound step
  (`office/repository/sqlite/participants.go`), so an unbound-step task has zero
  participant rows **by construction**; a permission-first implementation would
  report AC-TASKS-QUORUM-RECORDING-001.3's opaque permission error on the most likely real trigger for AC-TASKS-QUORUM-RECORDING-001.8,
  a task created before a workflow step was assigned. The order is stated because
  both preconditions are false at once in that case and the two errors differ in
  diagnostic value.

- **AC-TASKS-QUORUM-RECORDING-001.11:** WHEN the decision tool returns successfully, THE SYSTEM SHALL return
  the seven fields enumerated in `## Tool contract` — `decision`, `role`,
  `step_id`, `decision_id`, `decided_at`, `transition_applied` and `guards` —
  and SHALL NOT return a scalar `required_count` or `received_count`.
  `guards` SHALL equal the AC-TASKS-QUORUM-REEVALUATION-001.14 snapshot's `Guards` for the validated step,
  in the same order, so the tool and the AC-TASKS-QUORUM-DIAGNOSTICS-001.4 endpoint report the same
  arithmetic. `guards` SHALL be an empty list, not an error and not omitted,
  when the validated step configures no guarded transition.
  WHEN the post-write AC-TASKS-QUORUM-REEVALUATION-001.14 snapshot cannot be computed — the AC-TASKS-QUORUM-REEVALUATION-001.3 case, where
  the write succeeded and the re-evaluation errored — THE SYSTEM SHALL return
  `guards` as an empty list and `transition_applied` as `false`, and SHALL still
  report the call as successful per AC-TASKS-QUORUM-REEVALUATION-001.3, the failure being surfaced through
  AC-TASKS-QUORUM-DIAGNOSTICS-001.2 rather than to the agent. A diagnostic read that failed SHALL NOT put the
  recorded verdict at risk.
  This AC exists because without it only two of the returned fields — `step_id`
  and `transition_applied`, via AC-TASKS-QUORUM-BINDING-001.2 and AC-TASKS-QUORUM-CONCURRENCY-001.4 — were observable by any
  acceptance criterion, so a wrong `role` or a wrong count could ship untested.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.