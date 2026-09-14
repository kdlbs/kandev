---
created: 2026-09-07
status: completed
requirements:
  - REQ-TASKS-RUNNER-SWITCH-001
  - REQ-TASKS-RUNNER-SWITCH-002
  - REQ-TASKS-RUNNER-SWITCH-003
  - REQ-TASKS-RUNNER-SWITCH-004
system_design:
  - ../../specs/tasks/system-design/runner-switch-before-materialization.md
---

# Implementation Plan: Runner Switch Before Materialization

## Overview

Allow a task's executor profile to change while the task has no materialized
runtime state. Keep the mutability verdict in the task service, validate target
runner compatibility before persistence, and serialize the switch against every
writer that can materialize the task. The existing task dialog uses the same
server-owned verdict on desktop and mobile.

The implementation keeps the stored runner as task metadata. A successful
switch changes only that value and publishes the existing task update. The next
session preparation resolves the committed profile, while explicit launch
overrides remain explicit.

## Delivery

- Project the ordered runner mutability verdict through every task projection.
- Add the task runner action with compatibility validation and typed outcomes.
- Lock all materialization and repository writers against concurrent switches.
- Wire the dialog save sequence and runner selection state to the action.
- Cover the backend races, PostgreSQL lock order, and partial-save retry flow.

## Work orders

- [Task 01: Runner switch before materialization](task-01-runner-switch-before-materialization.md)
