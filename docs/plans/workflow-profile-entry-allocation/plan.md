---
created: 2026-10-25
status: done
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Fence profile destination allocation by entry

One accepted profile-switch entry must retain its first destination allocation
when preparation or attachment is retried. A distinct accepted entry can apply
the configured session-start policy again. This is limited to allocation; it
does not recover a failed runtime, resume an unknown owner, or replay a prompt.

- [x] [Task 01: Fence destination allocation](task-01-fence-destination-allocation.md)

The backend repository owns the transactional gate. Its existing route record
and accepted transition identity supply the durable comparison; no schema,
public API, provider, or deployment change is needed.
