---
status: draft
system: office
created: 2026-10-20
owners:
  - kandev
---

# Office Human Done Requirements

## Overview

A user finishes an Office task by dropping its card on the **Done** column of
the Office status board, or by choosing **Done** in the task's status picker.
The task's workflow decides when work is complete: a task becomes complete only
by entering the workflow's final step with **Complete task when entering this
step** enabled (`REQ-TASKS-COMPLETION-001`).

Before this capability, a human Done request on a task that was not on the final
step was handled like an agent's: the task was redirected to `in_review` and
every workflow-seated reviewer and approver was woken, while the task stayed on
its earlier step. The request then failed with "Cannot mark done: move the task
to the final workflow step first". A user who wanted to finish the task had to
move it to the final step by hand.

A step move into a completing step had a separate gap: it marked an Office task
complete in the same write as the move, so pending approvers never blocked it.

This capability makes a human Done request carry the task to the final step,
and makes every human move into a completing step respect Office approvers.

## Terminology

- **Human request** - a request with no authenticated agent caller. Board
  drags, status-picker choices and task-page step moves are human requests.
- **Final step** - the workflow step with the highest position.
- **Completing step** - a final step with **Complete task when entering this
  step** enabled (`AC-TASKS-COMPLETION-001.2`).
- **Pending approver** - an approver seated anywhere on the task's workflow
  without an approval for the current review round
  (`AC-OFFICE-SEAT-READ-SCOPE-001.1`).

## Requirements

### REQ-OFFICE-HUMAN-DONE-001: Human Done follows the workflow

**Intent:** A user who marks an Office task Done finishes it through its
workflow instead of being refused.

#### Acceptance criteria

- **AC-OFFICE-HUMAN-DONE-001.1:** When a human request sets an Office task's
  status to `done` and the task's workflow step is not the final step, the task
  shall be moved to the final step through the human step-move path. The task
  shall not be redirected to `in_review`.
- **AC-OFFICE-HUMAN-DONE-001.2:** After that move, the task shall show `done`
  only when the final step is a completing step. Otherwise the task shall show
  the status the move produced, and the user shall be told that the task moved
  to the named step without being completed.
- **AC-OFFICE-HUMAN-DONE-001.3:** When the move is refused (pending approvers,
  an unmet completion gate, an active session, a pending move, or any other
  move refusal), the task's status, step, decisions and queued runs shall be
  unchanged, and no reviewer or approver shall be woken. The card shall return
  to its original column and the user shall see the reason. A pending-approver
  refusal shall name the pending approvers.
- **AC-OFFICE-HUMAN-DONE-001.4:** An agent request to set `done` from a
  non-final step shall keep redirecting the task to `in_review`. A Done request
  on a task already on the final step, and on a task with no workflow step,
  shall keep the existing approver gate.
- **AC-OFFICE-HUMAN-DONE-001.5:** The board and the status picker shall give
  the same outcomes on desktop and phone. On a phone, a press-and-hold drag and
  the status picker shall both reach Done. Every outcome message shall be
  visible text, not hover-only content.

### REQ-OFFICE-HUMAN-DONE-002: Human step moves respect Office approvers

**Intent:** No human step move completes an Office task that its approvers
have not approved.

#### Acceptance criteria

- **AC-OFFICE-HUMAN-DONE-002.1:** A human move of an Office task into a
  completing step shall be refused before any write when the task has at least
  one pending approver. The refusal shall be HTTP 409 with code
  `task_approvals_pending` and shall list the pending approvers.
- **AC-OFFICE-HUMAN-DONE-002.2:** A refused move shall leave the task's step,
  state, decisions and step-entry actions unchanged, and shall publish no task
  event.
- **AC-OFFICE-HUMAN-DONE-002.3:** Moves by the workflow engine, agents and
  automations shall keep their existing approver handling. Moves of tasks that
  are not Office tasks, and moves into steps that do not complete the task,
  shall be unaffected.
- **AC-OFFICE-HUMAN-DONE-002.4:** A user whose move is refused for pending
  approvers shall see a translated message saying that approval is pending, on
  desktop and phone.

## Exclusions

- A human Done request on a task already on the final step still writes the
  status directly and does not evaluate the completion gate. That path is
  unchanged here.
- The board does not reorder workflow steps or skip steps' entry actions: the
  move runs the final step's entry actions as any human move does. The Review
  and Approval agents do not run for steps the task skips; approvers still
  block completion through REQ-OFFICE-HUMAN-DONE-002.

## System design

[Office human Done system design](../system-design/human-done-final-step.md).
