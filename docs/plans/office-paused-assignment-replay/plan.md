---
requirements:
  - REQ-OFFICE-PAUSE-REPLAY-001
system_design:
  - ../../specs/office/system-design/workspace-kill-switch-02.md
created: 2026-09-26
status: planned
---

# Implementation Plan: Replay Assignments Deferred by a Workspace Pause

Office Beta ISSUE-8. A task assigned while its workspace is paused has its wake
refused by the pause gate and nothing replays it on resume. This plan records a
durable deferred assignment and replays it once after resume.

## Work order

- [Task 01: Defer and replay paused assignments](task-01-deferred-assignment-replay.md)

## Outcome

An assignment made during a pause launches exactly one run after resume, a task
keeps the assignment actor and its queue policy, a task archived, unassigned
or reassigned during the pause is handled by its latest state, and each
deferral is visible in the task's activity.
