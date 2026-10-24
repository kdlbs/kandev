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
  to the named step without being completed. When the final step's WIP limit
  queues the move, the user shall be told that the task is queued for that step
  and is waiting for room, without promising that admission will complete it.
- **AC-OFFICE-HUMAN-DONE-001.3:** When the move is refused (pending approvers,
  an unmet completion gate, an active session that also blocks a manual move, a
  pending move, or any other move refusal), the task's status, step, decisions and queued runs shall be
  unchanged, and no reviewer or approver shall be woken. The card shall return
  to its original column and the user shall see the reason. A pending-approver
  refusal shall name the pending approvers.
- **AC-OFFICE-HUMAN-DONE-001.4:** An agent request to set `done` from a
  non-final step shall keep redirecting the task to `in_review`. A human Done request
  on the final step, including a step that does not complete automatically,
  and a legacy request with no workflow step shall respect the same atomic
  approver refusal as other human completions. A refusal shall not redirect
  the human task to in_review.
- **AC-OFFICE-HUMAN-DONE-001.5:** The board and the status picker shall give
  the same outcomes on desktop and phone. On a phone, a press-and-hold drag and
  the status picker shall both reach Done. Every outcome message shall be
  visible text, not hover-only content.

### REQ-OFFICE-HUMAN-DONE-002: Human completion respects Office approvers

**Intent:** No human completion of an Office task bypasses its approvers,
including when approval eligibility changes concurrently.

#### Acceptance criteria

- **AC-OFFICE-HUMAN-DONE-002.1:** A genuine human completion of an Office
  task, whether a step move, same-step state completion, session approval or
  Office Done status request, shall be refused before any write when the task
  has at least one pending approver at completion, counting the destination
  step's template approvers. The refusal shall be HTTP 409 with code
  `task_approvals_pending` and shall list the pending approvers.
- **AC-OFFICE-HUMAN-DONE-002.2:** A refused human completion shall leave the task's step,
  state, decisions and step-entry actions unchanged, and shall publish no task
  event.
- **AC-OFFICE-HUMAN-DONE-002.3:** Moves by the workflow engine, agents and
  automations shall keep their existing approver handling. Moves of tasks that
  are not Office tasks, and moves into steps that do not complete the task,
  shall be unaffected.
- **AC-OFFICE-HUMAN-DONE-002.4:** A user whose move is refused for pending
  approvers shall see a translated message saying that approval is pending, on
  desktop and phone.

- **AC-OFFICE-HUMAN-DONE-002.5:** An approver assignment, template change or
  approval revocation that takes effect before completion shall be included
  in that completion decision. A change that follows an accepted completion
  shall not retroactively undo it. A task that actually remains queued without
  completing shall retain its existing queued behavior.

## Exclusions

- The existing generic task completion-criteria exclusion for the Office
  direct Done status path is retained. Office approver checks are atomic on
  that path; this exclusion does not exempt it from REQ-OFFICE-HUMAN-DONE-002.
- The board does not reorder workflow steps or skip steps' entry actions: the
  move runs the final step's entry actions as any human move does. The Review
  and Approval agents do not run for steps the task skips; approvers still
  block completion through REQ-OFFICE-HUMAN-DONE-002.

## System design

[Office human Done system design](../system-design/human-done-final-step.md).
