---
status: complete
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-COMPLETION-001
  - REQ-TASKS-WORKSPACE-COLLABORATION-001
system_design:
  - ../../specs/cross-workspace-task-handoff/system-design/completion-notification.md
  - ../../specs/tasks/system-design/workspace-collaboration.md
legacy_specs: []
---

# Implementation Plan: Handoff completion report

## Goal

Report a cross-workspace delivery task's terminal state to its source task
under the current report and prompt grants. Preserve each delivery action
across pauses, event failures, and run-history retention.

## Work orders

- [Task 01: Durable completion report and source wake](task-01-completion-report.md)
  implements the authorization boundary, report, prompt context, durable
  retry, and run-provenance updates.
