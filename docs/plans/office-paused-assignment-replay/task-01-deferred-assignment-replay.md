---
id: "01-deferred-assignment-replay"
title: "Defer and replay paused assignments"
status: planned
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-PAUSE-REPLAY-001
acceptance_criteria:
  - AC-OFFICE-PAUSE-REPLAY-001.1
  - AC-OFFICE-PAUSE-REPLAY-001.2
  - AC-OFFICE-PAUSE-REPLAY-001.3
  - AC-OFFICE-PAUSE-REPLAY-001.4
  - AC-OFFICE-PAUSE-REPLAY-001.5
  - AC-OFFICE-PAUSE-REPLAY-001.6
  - AC-OFFICE-PAUSE-REPLAY-001.7
system_design:
  - ../../specs/office/system-design/workspace-kill-switch-02.md
---

# Task 01: Defer and replay paused assignments

## Scope

- Add the `office_deferred_assignments` table and repository methods.
- Record a deferral on `ErrWorkspacePaused` in the reactivity assignee handoff
  and the `task.updated` assignment subscriber.
- Drain pending deferrals from `pause.Service.Resume` and from the Office
  recovery tick for unpaused workspaces, re-validating each against the task and
  queueing with the assignment actor and idempotency key. Agent-initiated
  replays use the scheduler assignment allowance.
- Write task-targeted activity entries for deferral, replay and drop.

## Validation

- Regression matrix `TestBetaPausedAssignmentResume` fails before the fix and
  passes after it.
- `go test ./internal/office/... ./internal/backendapp -run 'Pause|Assign' -count=1`
  from `apps/backend`.
- `make -C apps/backend lint`.
