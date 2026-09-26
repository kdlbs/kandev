---
status: draft
system: office
created: 2026-09-26
owners:
  - kandev
---

# Office: Paused Assignment Replay Requirements

## Overview

The workspace pause ([kill switch](workspace-kill-switch.md)) refuses every wake
while paused and creates no run row (`AC-OFFICE-KILL-SWITCH-002.5`), but it still
persists the assignment that caused the wake (`AC-OFFICE-KILL-SWITCH-002.6`).
Nothing consumed that assignment after resume: the wake was refused, resume
replayed nothing, and the recovery sweep only sees recent `TODO` tasks with no
finished run. Work assigned during a pause was silently lost (Office Beta
ISSUE-8). This document owns keeping that intent and honouring it once after
resume.

## Terminology

- **Deferred assignment:** the durable record of an assignment whose
  `task_assigned` wake a confirmed workspace pause refused.
- **Replay:** creating the refused wake's run after the workspace resumes.

## Requirements

### REQ-OFFICE-PAUSE-REPLAY-001: Assignments made during a pause survive resume

**Intent:** A pause stops launches; it must not discard the work an operator or
agent assigned while it held.

**User story:** As an operator, I want tasks assigned while my workspace was
paused to start once I resume, so that pausing never loses work.

#### Acceptance criteria

- **AC-OFFICE-PAUSE-REPLAY-001.1:** While a workspace is paused, when an
  assignment of an Office task to an agent is persisted and its wake is refused
  by the pause gate, the system shall durably record a deferred assignment for
  that task, keeping only the task's latest assignment and the actor that caused
  it, and the record shall survive backend restarts.
- **AC-OFFICE-PAUSE-REPLAY-001.2:** While a workspace is paused, the system shall
  create no run for a deferred assignment.
- **AC-OFFICE-PAUSE-REPLAY-001.3:** When a workspace is resumed, the system shall
  create exactly one `task_assigned` run for each deferred assignment whose task
  is not archived and whose current runner and assignment generation still match
  the record, using the recorded actor and its queue policy, whatever the task's
  state and whatever runs the task finished before the pause.
- **AC-OFFICE-PAUSE-REPLAY-001.4:** When a workspace is resumed, the system shall
  create no run for a deferred assignment whose task was archived, unassigned or
  reassigned after the record was written, other than the run the latest
  assignment is itself owed.
- **AC-OFFICE-PAUSE-REPLAY-001.5:** When a wake is redelivered, resume
  processing repeats, or the backend restarts between the release and the
  replay, the system shall still create at most one run per deferred assignment
  and shall replay any record the interrupted resume left pending.
- **AC-OFFICE-PAUSE-REPLAY-001.6:** When an assignment is deferred, replayed or
  dropped, the system shall write an activity log entry targeting the task and
  naming the pause, so that the deferral and its outcome are visible per task
  and not only as a process counter.
- **AC-OFFICE-PAUSE-REPLAY-001.7:** When the pause gate cannot determine pause
  state, the system shall record no deferred assignment and keep the behavior of
  `AC-OFFICE-KILL-SWITCH-002.9`.

## Out of scope

- **Replaying wakes other than `task_assigned`.** Comments, approvals and
  blocker wakes refused during a pause are not recorded here.
- **Runs cancelled by the halt sweep.** `AC-OFFICE-KILL-SWITCH-005.4` still
  forbids restoring them.
- **A UI surface for pending deferrals.** Visibility is the task's activity
  entries.
