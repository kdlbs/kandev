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
promotion order, and successful-path session handling remain unchanged. Guarded
decisions use a separate entry path and are outside this session-specific log.

The transition result carries the admission decision and the WIP limit used by
the commit. This keeps the lifecycle log aligned with the limit that placed the
task. If a compatibility commit does not return admission state and the task
reread fails, log the failure and stop before destination entry.

## Work order

- [Task 01: Log the on-enter deferral](task-01-log-onenter-deferral.md)
