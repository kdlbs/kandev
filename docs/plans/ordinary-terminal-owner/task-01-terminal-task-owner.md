---
id: "01-terminal-task-owner"
title: "Resolve the ordinary terminal task owner"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-TERMINALS-001
  - REQ-TASKS-TERMINAL-TARGET-001
acceptance_criteria:
  - AC-TASKS-TASK-TERMINALS-001.3
  - AC-TASKS-TERMINAL-TARGET-001.3
  - AC-TASKS-TERMINAL-TARGET-001.5
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
  - ../../specs/tasks/system-design/terminal-target-lifecycle.md
---

# Task 01: Resolve the terminal task owner

Read the persisted managed-terminal owner before choosing a workspace
execution. Check task and environment access, select the current task session,
and reject a changed binding. Do not consume the initial command while reading
the owner, change unmanaged terminal routing, or recreate missing resources.

## Files likely touched

- `apps/backend/internal/gateway/websocket/terminal_handler.go`
- `apps/backend/internal/gateway/websocket/terminal_owner.go`
- `apps/backend/internal/terminal/service/service.go`
- `apps/backend/internal/agent/runtime/lifecycle/task_environment_execution.go`

## Verification

Run the existing terminal owner, environment authorization, one-shot command,
and workspace admission checks. The implementation depends on #4713's retained
environment retirement guard; review this task's own delta separately.
