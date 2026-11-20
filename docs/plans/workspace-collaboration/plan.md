---
created: 2026-10-15
status: in-progress
requirements:
  - REQ-TASKS-WORKSPACE-COLLABORATION-001
system_design:
  - ../../specs/tasks/system-design/workspace-collaboration.md
legacy_specs: []
---

# Workspace collaboration implementation

This plan records the workspace collaboration work in [Task 01](task-01-conversation-grants.md)
and [Task 02](task-02-plugin-caller-context.md). The current PR branch includes
the task-owned grant store, server-derived caller checks, separate read/report/
prompt/control grants, non-prompting report delivery, plugin Host caller context,
and grant-editor entry points on desktop and mobile. The native feature toggle
remains off by default.

Profile inheritance, taskless callers, delegated authority, and aggregate
navigation remain follow-up scope. They are not part of this PR.

At the Task 01 checkpoint, the generic plugin Host callback did not carry a
verified conversation origin, so Task 01 alone did not enforce grants across
the main MCP plugin-tool flow. [Task 02](task-02-plugin-caller-context.md)
closes that transport gap through the existing SDK and gRPC broker. The paired
[system design](../../specs/tasks/system-design/workspace-collaboration.md#transport-coverage-and-original-remaining-work)
records the boundary and remaining follow-up scope.

The branch also exposes status-report delivery through `message_task_kandev`.
Task 01's verified domain boundaries and the native permission resolver remain
the authorization foundation for both transport paths.
