---
status: draft
system: ui
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# UI

## Scope

UI owns responsive behavior and reusable presentation preferences, including
task-listing modes, Home destination choices, and reusable client-side task
navigation. Reusable editor presentation and reply ownership also belong here;
filesystem mutation authority remains with Workspaces. Other systems retain task,
runtime, and workspace lifecycle/state ownership.

Files move selection belongs here: the browser constructs independent move
roots from selected rows before requesting workspace mutations. Workspaces
retains entry eligibility, containment, and rename authority.

The reusable settings manual-save interaction belongs here. Its
[requirements](requirements/settings-manual-save.md) and
[system design](system-design/settings-manual-save.md) define route coordination
and contributor draft registration; domain components retain payload and
persistence ownership under ADR 0046.

## Related

- [Platform](../platform/README.md)
