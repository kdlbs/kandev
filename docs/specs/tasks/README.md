---
status: active
system: tasks
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Task and workflow system

## Purpose

The task and workflow system owns durable work items, task relationships,
execution lifecycle, workflow steps, queued launches, and the contracts that
move a task through its configured process.

## Ownership

This system owns task identity and metadata, task documents and attachments,
parent and dependency relationships, task creation and launch behavior, task
runtime state publication, workflow definitions and transitions, completion
signals, and task-scoped scheduling contracts.

## Exclusions

- Agent identity, permissions, and runtime profiles belong to the
  [agent system](../agents).
- Repository and worktree ownership belongs to the
  [workspace system](../workspaces).
- Office-specific autonomous agent identities and dashboards belong to the
  [Office system](../office).
- Presentation-only behavior belongs to the [UI specifications](../ui).

## Migration record

Migration remains in progress. Use the catalog command to find the current
requirement and system-design documents for this system. Other migrated files
still need the same extraction before this system can return to a complete
migration state.

## Related systems

- [Agents](../agents): supplies agent identity and execution profiles.
- [Office](../office): builds autonomous workflows on task primitives.
- [UI](../ui): owns presentation-specific task surfaces.
- [Workspaces](../workspaces): owns repositories and task worktrees.
