---
status: draft
system: workspaces
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# Workspace system

## Purpose

The workspace system owns workspace lifecycle, repositories, worktrees,
branches, branch policies, workspace secrets, and workspace-scoped execution context.

## Ownership

This system owns workspace creation and deletion, repository attachment,
repository sets, local repositories, branch templates, secrets, and workspace
Git state.

## Exclusions

- Task-owned worktree lifetime belongs to the [task system](../tasks/README.md).
- Provider credentials belong to the [integration system](../integrations/README.md).
- Workspace settings presentation belongs to the [UI system](../ui/README.md).

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents. Use the catalog command to find them.

## Related systems

- [Tasks](../tasks/README.md): consumes workspace repositories and worktrees.
- [Integrations](../integrations/README.md): supplies remote repository identity.

