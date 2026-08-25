---
status: draft
system: ui
specification_version: 1
migration: in_progress
owners:
  - Kandev
---

# User interface system

## Purpose

The user interface system owns Kandev's user-facing composition, navigation,
responsive presentation, accessibility, and interaction behavior across desktop,
tablet, and phone surfaces.

## Ownership

This system owns how existing product capabilities are exposed, grouped, and
adapted to viewport and input mode. It also owns discoverability, touch and
keyboard reachability, scroll ownership, safe-area behavior, and visual hierarchy.

## Exclusions

- Task identity, workflow transitions, and task lifecycle belong to the
  [task and workflow system](../tasks/).
- Repository, worktree, and Git execution contracts belong to their owning task
  and workspace systems.
- Agent identity and runtime behavior belong to the agent system.

## Specification map

### Requirements

- [Mobile task chrome](requirements/mobile-task-chrome.md)

### System design

- [Mobile task chrome](system-design/mobile-task-chrome.md)

## Migration status

Migration is in progress. The requirement and design above are authoritative
for phone task-top-bar action placement. Legacy UI specifications listed in
[`docs/specs/INDEX.md`](../INDEX.md) remain authoritative for capabilities not
yet represented under `requirements/` and `system-design/`.

## Related systems

- [Tasks](../tasks/README.md): supplies task actions, workflow movement, and
  session state rendered by task surfaces.
