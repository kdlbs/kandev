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

## Specification map

### Requirements



- [Branch Policies](requirements/branch-policies.md)
- [Create a Local Repository During Task Creation](requirements/create-local-repository.md)
- [Empty Remote Repositories](requirements/empty-remote-repositories.md)
- [Kanban workspace creation](requirements/creation.md)
- [Workspace Deletion](requirements/deletion.md)
- [Improve Kandev](requirements/improve-kandev.md)
- [Local Workspace Repositories](requirements/local-repositories.md)
- [Organization Units](requirements/org-units.md)
- [Repository and Workspace Secrets](requirements/repository-secrets.md)
- [Repository Sets](requirements/repository-sets.md)
- [Copy and Move Secrets Between Scopes](requirements/secret-scope-transfer.md)
- [Workspace Base-Branch Propagation](requirements/workspace-base-branch-propagation.md)
- [Worktree Base Refresh](requirements/worktree-base-refresh.md)
- [Worktree Branch Templates](requirements/worktree-branch-templates.md)

### System design

- [Empty Remote Repositories](system-design/empty-remote-repositories.md)
- [Improve Kandev](system-design/improve-kandev.md)
- [Copy and Move Secrets Between Scopes](system-design/secret-scope-transfer.md)
- [Branch Policies](system-design/branch-policies.md)
- [Worktree Base Refresh](system-design/worktree-base-refresh.md)
- [Local Workspace Repositories](system-design/local-repositories.md)
- [Organization Units](system-design/org-units.md)

## Migration record

Migration remains in progress while legacy source detail is extracted from the
canonical requirement and system-design documents above.

## Related systems

- [Tasks](../tasks/README.md): consumes workspace repositories and worktrees.
- [Integrations](../integrations/README.md): supplies remote repository identity.
