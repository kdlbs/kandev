---
created: 2026-09-20
status: implemented
requirements:
  - REQ-TASKS-SESSION-STALL-VISIBILITY-001
system_design:
  - ../../specs/tasks/system-design/session-stall-visibility.md
legacy_specs: []
---

# Implementation Plan: Session Stall Visibility and Orphan Healing

## Overview

Close the silent-stall incident behind issue #3712 (and the sweep extension
of #3711) with one vertical correction in the session reconciliation sweep:
detect unarchived tasks holding active sessions whose backing execution is
gone, report each stall once per episode, and heal the orphans after a grace
window. Detection and healing share one pass; the fix plan approved in the
issue rejected review-specific watchdogs, verdict-sniffing, and
dashboard-only signaling.

## Evidence and root cause

Four review tasks (issue #3712, 2026-09-16) sat in `WAITING_FOR_INPUT` for
hours with finished agent reports in `task_session_messages` but no events
after creation. The same orphaned-turn failure as #3711: the turn-complete
actor died with a backend restart, the session row stayed in an active
state, and the event-driven stall warning ("no events received") never fired
because no events flowed at all. Reviews specifically never advance while
their session looks active, so the workflow also froze.

## Work orders

Sequential; one work order owns the sweep.

- [x] [Task 01: Detect and heal orphaned active sessions in the sweep](task-01-stall-detection-and-healing.md)

## Verification strategy

Backend unit regressions colocated with the sweep
(`internal/task/service/active_session_stall_test.go`) and the repository
(`internal/task/repository/sqlite/`); no browser surface changes. See the
work order for exact commands and results.

## Risks and non-goals

- Healing is deliberately conservative: all-or-nothing per task, liveness
  re-checked at the cancel boundary, ID-scoped writes. A false heal needs
  every session of a task execution-less and silent past twice the
  threshold while a live execution registers mid-sweep.
- No front-end consumption of `task.stalled` is included; any UI is a
  separate initiative.
- Agent-runtime stall detection for live executions is unchanged.
