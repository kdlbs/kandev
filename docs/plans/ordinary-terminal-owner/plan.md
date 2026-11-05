---
created: 2026-11-06
status: implemented
requirements:
  - REQ-TASKS-TASK-TERMINALS-001
  - REQ-TASKS-TERMINAL-TARGET-001
system_design:
  - ../../specs/tasks/system-design/task-terminal-persistence.md
  - ../../specs/tasks/system-design/terminal-target-lifecycle.md
legacy_specs: []
---

# Ordinary terminal execution owner

Select the execution belonging to the persisted ordinary terminal's task when
multiple tasks share an environment. Keep authorization, one-shot command
handling, missing-target responses, and unmanaged terminal routing intact.

## Work order

- [Task 01: Resolve the terminal task owner](task-01-terminal-task-owner.md)
