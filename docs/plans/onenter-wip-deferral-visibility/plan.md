---
created: 2026-10-20
status: implemented
requirements:
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
system_design:
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
legacy_specs: []
---

# Visible On-Enter Deferral for Full WIP Steps

When a session-lifecycle engine transition commits to a full WIP-limited
destination, its entry waits for queue promotion. Record the reason so operators
can distinguish a queued arrival from an entry action that never ran. Admission,
promotion order, and session state remain unchanged. Guarded decisions use a
separate entry path and are outside this session-specific log.

## Work order

- [Task 01: Log the on-enter deferral](task-01-log-onenter-deferral.md)
