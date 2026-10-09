---
status: active
system: office
created: 2026-09-17
owners:
  - kandev
---

# Taskless Office Run Sessions

## Overview

Office owns lightweight routine execution and coordinator wakeups. These wakes
must execute without inventing task records. This makes explicit the taskless
behavior already described in the [scheduler design](../system-design/scheduler-01.md).
The user confirmed this behavior on September 17, 2026.

## Requirements

### REQ-OFFICE-TASKLESS-001: Managed taskless execution

**Intent:** A coordinator can inspect and manage its workspace on a periodic or
event wake without creating a task merely to host its own execution.

#### Acceptance criteria

- **AC-OFFICE-TASKLESS-001.1:** An eligible lightweight wake with no task shall start a real agent session, deliver its assembled prompt and permitted tools, and reach a visible terminal outcome without creating any task or task session.
- **AC-OFFICE-TASKLESS-001.2:** Every fire and retry shall use a fresh session. The existing routine-scoped or agent-scoped continuation summary shall carry context; a failed attempt shall not replace the last successful summary. A summary write that fails shall leave the last successful summary unchanged, shall leave the completed run complete, and shall be visible on the run.
- **AC-OFFICE-TASKLESS-001.3:** Concrete and provider-routed profiles shall both work. Existing budget admission, capabilities, retry/backoff, coalescing and idle-skip rules shall apply. Periodic idle skipping shall not consume a manual or webhook wake.
- **AC-OFFICE-TASKLESS-001.4:** Run history shall show the exact session, runtime outcome, actual adapter/model and attributed usage. Duplicate or delayed events from an older attempt shall not complete, charge twice, or clear a newer attempt.
- **AC-OFFICE-TASKLESS-001.5:** Workspace pause, explicit run cancellation, agent disable/removal and workspace removal shall prevent new taskless launches and stop live taskless executions. Partial stop failures shall remain visible and retryable, including when another execution stopped successfully.
- **AC-OFFICE-TASKLESS-001.6:** After backend restart, unfinished attempts shall be reconciled against runtime evidence before replacement launch. A possibly live predecessor shall block a replacement until it is stopped or proven absent. Interrupted work shall have an explicit outcome; no taskless attempt shall remain claimed indefinitely solely because it has no task.
- **AC-OFFICE-TASKLESS-001.7:** A taskless session shall retain its workspace and capability boundaries. Task-specific decision tools shall reject it without a task context. Failed ownership lookup shall reject launch or access, never broaden authority.
- **AC-OFFICE-TASKLESS-001.8:** Existing task-bound launches and their task/session lifecycle shall retain their behavior. Historical unsupported taskless failures shall remain history and shall not be automatically replayed.

The implementation and coverage in this change focus on `.1` to `.4`, `.7` and
`.8`. Criteria `.5` and `.6` remain requirements and are outstanding follow-up
work; this change does not implement their lifecycle or recovery behavior.

## Out of scope

Interactive taskless chat, synthetic tasks, new periodic producers, changing
coordinator cadence, and cross-fire ACP session resumption.

Pruning run-session records when run-history retention deletes their run is also
out of scope. Retention removes the run and its events; the run-session record
survives until its workspace is deleted. No criterion here reads a run session
after its run is gone, and the cost projections join run sessions by session ID.
A follow-up that changes this needs a retention pass over the run-session store
and a decision on whether attributed usage must outlive the run it belongs to.

### Outstanding: stop controls and restart recovery

Two flows were in this requirement as `AC-OFFICE-TASKLESS-001.5` and
`.6`. They remain outstanding and are not implemented in this coverage change.
The analysis below is the brief a follow-up should start from.

**Required behavior.** `.5` requires workspace pause, explicit run cancellation,
agent disable/removal and workspace removal to prevent new taskless launches and
stop live taskless executions, with partial stop failures remaining visible and
retryable. `.6` requires unfinished attempts to be reconciled against runtime
evidence after a backend restart, a possibly live predecessor to block its
replacement until stopped or proven absent, interrupted work to reach an
explicit outcome, and no taskless attempt to remain claimed indefinitely solely
because it has no task.

**Why it is not in this change.** This change proves the launch path from an
armed trigger to a real session. Stop and recovery need a separate maintainer
review because they cross runtime ownership, operator controls and restart
ordering. Keeping them as outstanding criteria avoids treating the current
gaps as an accepted product decision.

**What is therefore accepted as a known gap today.** Cancelling a run does not
stop a live coordinator: run cancellation is a status write on `runs` with no
run-session stop, and agent disable and agent removal cascade through a
task-session terminator that never consults `office_run_sessions`, so a
run-owned execution outlives all three. Workspace pause and workspace deletion
do reach live run sessions. Separately, a crash between an attempt's terminal
write and its run's terminal write leaves a terminal attempt whose run is still
claimed; because the unfinished-session query selects `preparing` and `running`
only, nothing settles the run from that attempt and the generic stale-claim
sweep eventually requeues it, duplicating an execution of finished work.

**What a follow-up must resolve first.** Two problems are known and unsolved,
and neither is a drafting gap:

1. *"Visible and retryable" has no channel for three of the four controls.*
   Retryability was defined as the next sweep of the workspace re-listing the
   still-live session. That works for workspace pause, which is swept. Run
   cancellation, agent disable and agent removal are one-shot operator events,
   and this design deliberately has no periodic run-session sweep (chosen for
   parity with task sessions), so a failed stop on those three is visible but
   retryable only at the next restart. A follow-up either adds a sweep, gives
   these controls their own retry, or narrows the criterion to say so.
2. *Ordering between a control's own mutation and the stop.* The `runs` status
   write, the agent status write and instance deletion were never ordered
   against the list-intent-stop sequence, and it was never stated whether a
   partial stop failure commits or rolls back the control. Agent removal is the
   sharp case: if the agent row goes first, the guarantee rests on a row whose
   owner no longer exists.

Prior analysis worth reusing rather than re-deriving: settlement must be decided
per run and not per attempt row, from the run's complete attempt set — skip a
run with any live attempt, otherwise let any `finished` attempt complete it
settling from the highest-numbered one, otherwise release the claim by
requeuing. A settlement writes the run's completion event, refreshes the
continuation summary, releases the task checkout and stamps the run finished,
and cannot write the run's output summary because that needs a turn and session
identity a run-owned execution does not carry. Two channels can notice a
settleable run first — startup reconciliation and attempt reservation — and both
must decide by the same predicate. `cancel_requested_at` is what distinguishes a
deliberate stop from a crash.

## System design

[Run-owned sessions](../system-design/taskless-run-sessions.md) covers the
launched behavior here. Criteria `.5` and `.6` remain outstanding and need a
separate lifecycle and recovery design pass.
