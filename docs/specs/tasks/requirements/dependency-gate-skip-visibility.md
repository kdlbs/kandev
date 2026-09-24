---
status: active
system: tasks
created: 2026-09-16
owners:
  - kandev
---
# Dependency Gate Skip Visibility Requirements

## Overview

A task that reaches an automated launch path while it still has unresolved
dependencies is deliberately not started. That deliberate skip must be visible
in backend logs at the default level, so an operator can tell a working
dependency gate from a broken launcher without session surgery or a restart.

## Requirements

### REQ-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001: Dependency gate skip visibility

**Intent:** Every automated-launch skip caused by unresolved task dependencies
is observable by operators at the default backend log level.

#### Acceptance criteria

- **AC-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.1:** When an automated launch
  path skips a launch because the task has unresolved dependencies, the backend
  shall log a WARN-level entry whose message identifies the triggering event
  and whose structured fields carry `task_id` and the dependency
  `blocked_reason`.
- **AC-TASKS-DEPENDENCY-GATE-SKIP-VISIBILITY-001.2:** A task that passes the
  dependency gate shall produce no blocked-skip WARN entry, so the WARN level
  stays reserved for genuine dependency skips instead of noise on every
  automated launch.

## Scenarios

- **GIVEN** a task whose dependency gate reports blocked, **WHEN** the task
  enters a workflow step with `on_enter: auto_start_agent`, **THEN** the
  backend emits one WARN entry naming the step-enter event, carrying the task
  and the blocked reason, and starts no session.
- **GIVEN** a task whose dependency gate reports blocked, **WHEN** WIP queue
  promotion or an automated session launch reaches the gate, **THEN** the same
  blocked-skip WARN entry is emitted for that event.
- **GIVEN** a task with no unresolved dependencies, **WHEN** any automated
  launch path runs, **THEN** no blocked-skip WARN entry is emitted.
- **GIVEN** a dependency read that fails, **WHEN** the gate fails closed,
  **THEN** the existing lookup-failure WARN entry (with the read error) is
  emitted instead of the blocked-skip entry.

## Out of scope

- Gate semantics: the blocked verdict, the fail-closed read behavior, and
  launch-token handling remain owned by
  [Task Dependencies and Auto-Start Chains](task-dependencies.md).
- Board or dashboard indicators for dependency-blocked tasks.
- Log-level changes on paths other than the dependency gate.
