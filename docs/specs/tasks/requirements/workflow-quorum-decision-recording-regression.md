---
status: draft
system: tasks
created: 2026-08-19
owners:
  - kandev
---


# Quorum regression behavior Requirements



## Overview



Representative approval, rejection, waiting, and human request-changes flows produce the expected task states.

## Requirements


### REQ-TASKS-QUORUM-REGRESSION-001: Quorum regression behavior



**Intent:** Representative approval, rejection, waiting, and human request-changes flows produce the expected task states.

#### Acceptance criteria

#### Regression

- **AC-TASKS-QUORUM-REGRESSION-001.1:** GIVEN a task at Review with exactly one reviewer participant
  (`decision_required = 1`) and one recorded `approved` decision from that
  reviewer, WHEN quorum is evaluated, THE SYSTEM SHALL move the task to
  Approval.
- **AC-TASKS-QUORUM-REGRESSION-001.2:** GIVEN the same task with one recorded rejection instead, WHEN quorum
  is evaluated, THE SYSTEM SHALL move the task to Work.
- **AC-TASKS-QUORUM-REGRESSION-001.3:** GIVEN two reviewer participants and one approval, WHEN quorum is
  evaluated under `all_approve`, THE SYSTEM SHALL leave the task at Review and
  report it as awaiting one further decision.
- **AC-TASKS-QUORUM-REGRESSION-001.4:** GIVEN a workflow whose steps carry no guard, THE SYSTEM SHALL
  transition exactly as it does today.
- **AC-TASKS-QUORUM-REGRESSION-001.5:** GIVEN a task at Review, whose guards name `role: reviewer`, and a
  human recording **Request changes** through `POST /tasks/:id/request-changes`,
  WHEN quorum is evaluated, THE SYSTEM SHALL move the task to Work — even though
  that decision is stored with `role = approver`. This is the regression that
  makes AC-TASKS-QUORUM-VERDICT-001.2 and AC-TASKS-QUORUM-VERDICT-001.7 observable end to end, and it is the exact path that is
  broken in production today.

- **AC-TASKS-QUORUM-REGRESSION-001.6:** GIVEN a task moved off a guarded step by `any_reject` under AC-TASKS-QUORUM-REGRESSION-001.2
  and later returned to that step, WHEN quorum is next evaluated there, THE
  SYSTEM SHALL evaluate against zero decisions from the prior round. This AC
  exists to make AC-TASKS-QUORUM-CONCURRENCY-001.9's premise observable rather than assumed: `clear_decisions`
  on `on_enter` is depended upon but verified by no other AC, and the only
  `on_enter` trigger dispatch in the tree is inside `SwitchWorkflowCallback` —
  the orchestrator's callback registry wires a `DispatchTriggerFn` for
  `switch_workflow` only. If this AC fails, this card is blocked on the sibling
  `on_enter` action-dispatch feature and SHALL be reported as blocked; it SHALL
  NOT be closed by adding a second decision-clearing path inside this feature.
  Without it, a rejection persists at the guarded step and re-fires `any_reject`
  on the next completed turn, so AC-TASKS-QUORUM-REGRESSION-001.1 and AC-TASKS-QUORUM-REGRESSION-001.2 cannot both hold across a round
  trip.

## Out of scope



Behavior outside this acceptance-criteria group is defined by the core quorum requirement and the other quorum capability requirements.