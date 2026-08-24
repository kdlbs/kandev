---
status: active
system: agents
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Agent system

## Purpose

The agent system owns agent identity, runtime commands, execution profiles,
permissions, provider capabilities, and managed runtime behavior.

## Ownership

This system owns built-in and dynamic agent definitions. It also owns runtime
selection, managed package recovery, profile behavior, and agent permissions.

## Exclusions

- Task state and workflow transitions belong to the [task system](../tasks/).
- Workspaces and worktrees belong to the workspace system.
- Presentation-only behavior belongs to the [UI specifications](../ui/).

## Specification map

### Requirements

- [Managed npm runtime recovery](requirements/managed-npm-runtime-recovery.md)

### System design

- [Managed npm runtime recovery](system-design/managed-npm-runtime-recovery.md)

## Migration status

Migration is in progress. The new documents are authoritative for managed npm
runtime recovery on local PC, local Docker, and remote SSH executors.

The legacy [runtime update specification](runtime-updates.md) remains
authoritative for version selection, update status, and activation behavior.
Other legacy files in this directory remain authoritative until this index
names their replacements.

## Related systems

- [Tasks](../tasks/): owns the session and task lifecycle that consumes agent runtime results.
- [UI](../ui/): owns agent settings and recovery presentation.
