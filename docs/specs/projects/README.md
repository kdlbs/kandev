---
status: active
system: projects
specification_version: 1
migration: complete
owners:
  - kandev
---

# Agent Projects system

## Purpose

Agent Projects own a long-lived coordinator conversation, its worker task tree,
and a shared context store for work on a selected set of repositories.

## Ownership

This system owns project identity and membership, coordinator/worker profile
policy, shared context lifetime, and the Projects navigation experience. Its
project identity is separate from Office projects.

## Exclusions

- [Tasks](../tasks/README.md) own task/session lifecycle and parent links.
- [Workspaces](../workspaces/README.md) own repositories and worktrees.
- [Agents](../agents/README.md) own profile definitions and execution.
- [Office](../office/README.md) owns its existing projects and autonomy model.

## Related systems

- [Tasks](../tasks/README.md)
- [Workspaces](../workspaces/README.md)
- [Agents](../agents/README.md)
- [Office](../office/README.md)
