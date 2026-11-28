---
status: active
system: tasks
created: 2026-10-20
owners:
  - kandev
---

# Stale CREATED auto-start requirements

## Overview

A workflow step's `auto_start_agent` action launches a prepared session that is
still CREATED. The dispatch carries a session snapshot, and the launch persists
STARTING only while the session row is still in the state the launch observed.
Another path can move the same session out of CREATED first: a manual step move
whose entry handling leaves the session WAITING_FOR_INPUT, or a dispatch replayed
after a capacity wait. The tasks system owns this contract because the outcome
decides the task and session states users see on the board.

## Terminology

- **Stale CREATED dispatch:** An auto-start dispatch whose session snapshot says
  CREATED while the stored session has already left CREATED.
- **Lost CREATED race:** A launch whose CREATED-guarded STARTING persistence is
  rejected because another path moved the session to a non-terminal state that
  owns no agent (CREATED or WAITING_FOR_INPUT).
- **Waiting-session prompt path:** The existing auto-start delivery for a session
  that is not CREATED, which prompts the session through `PromptTask` and queues
  the prompt when the session is busy.

## Requirements

### REQ-TASKS-STALE-CREATED-AUTOSTART-001: A lost CREATED race is not a launch failure

**Intent:** When another path advances a session before an auto-start launch can
claim it, the launch stops without failing the session or its task, and the
step prompt is delivered at most once, only while it still belongs to the
task's current step.

**User story:** As a user who moves a task while its auto-start agent is still
being prepared, I want the task to follow my move instead of turning FAILED, so
that I do not have to recover a task whose agent never failed.

#### Acceptance criteria

- **AC-TASKS-STALE-CREATED-AUTOSTART-001.1:** When an auto-start launch loses
  the CREATED race, the system shall not mark the session FAILED and shall not
  mark the task FAILED.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.2:** When an auto-start dispatch
  reloads its session and finds it WAITING_FOR_INPUT, the system shall not
  launch the session through the CREATED start path.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.3:** When a lost CREATED race or a
  stale CREATED dispatch finds the session WAITING_FOR_INPUT while the task is
  still on the dispatching step, the system shall deliver the step prompt
  exactly once through the waiting-session prompt path. A fresh STARTING or
  RUNNING state shall retain the complete step prompt through the existing
  busy-queue policy, including attachments, reference order, plan mode,
  recorded-message identity and the already claimed handoff.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.4:** When a lost CREATED race or a
  stale CREATED dispatch finds that the task has left the dispatching step, the
  system shall not deliver the step prompt and shall return a taken hand-off
  message to the session queue. The original step and session incarnation
  shall remain current at queue insertion and provider admission, including
  preparation waits, retries and composed fresh fallback.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.5:** The losing launch shall not start
  an agent process.
- **AC-TASKS-STALE-CREATED-AUTOSTART-001.6:** A start failure other than a lost
  CREATED race shall keep its existing failure handling, including FAILED
  session and task states.

## Why

On 2026-10-20 a queue promotion dispatched a planning step's auto-start for a
CREATED session with a prepared workspace. About 1.5s later a manual move left
the planning step; its entry handling could not switch the session profile and
set the session WAITING_FOR_INPUT. The launch then rejected its CREATED-guarded
STARTING write with an unmarked "state changed from CREATED to
WAITING_FOR_INPUT" error, and the generic launch-failure path marked the session
and the task FAILED although no agent had failed.

## Failure modes

- A terminal session keeps its existing handling: the persistence refusal reports
  a superseded session and the dispatch reports a terminalized session.
- A session that reached STARTING or RUNNING keeps the existing already-running
  handling, which queues the prompt.
- If the task row cannot be read to decide whether the prompt still belongs to
  the current step, the prompt is not delivered and a taken hand-off message is
  returned to the queue.

## Out of scope

- The manual-move entry handling that sets a CREATED session WAITING_FOR_INPUT
  when its profile switch is refused.
- Delivery-ledger, queue-admission and capacity policy.
- User-initiated starts other than the shared persistence classification.

## Implementation plan

[Stale CREATED auto-start](../../../plans/stale-created-auto-start/plan.md)
