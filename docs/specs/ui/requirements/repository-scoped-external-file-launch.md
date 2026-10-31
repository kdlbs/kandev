---
status: active
system: ui
created: 2026-10-27
owners:
  - kandev
---

# Repository-scoped external file launch

## Ownership and intent

A file-specific external-editor action must open the file in the repository represented by the action. UI owns the shared file-action handoff; Workspaces owns repository and worktree paths, and Tasks owns session associations. This contract is separate from [embedded editor file targets](embedded-editor-file-targets.md), which governs a downstream embedded dispatch, and [file-tree path scope](file-tree-path-scope.md), whose node paths have a different shape.

## Requirements

### REQ-UI-EXTERNAL-FILE-TARGET-001: Preserve repository identity

**Intent:** A task can contain two repositories with the same relative filename. Opening a selected file in an external editor must not select the other repository's file.

- **AC-UI-EXTERNAL-FILE-TARGET-001.1:** When a Review file action identifies a session worktree uniquely, desktop and phone external-editor actions shall request the selected file relative to that worktree, including when another worktree has the same relative filename or separate workspace placement metadata is unavailable.
- **AC-UI-EXTERNAL-FILE-TARGET-001.2:** When an existing editor toolbar or its overflow identifies a session worktree uniquely, its external-editor file action shall request that file relative to the identified worktree.
- **AC-UI-EXTERNAL-FILE-TARGET-001.3:** A named repository scope that cannot be resolved uniquely, when no authoritative file-specific repository identity selects a worktree, shall disable external-editor file actions and issue no launch request. When independent evidence resolves a scope uniquely to a different worktree than the authoritative file-specific identity, the action shall also be disabled. Missing session information shall also issue no launch request. Other menu actions retain their existing behavior.
- **AC-UI-EXTERNAL-FILE-TARGET-001.4:** An unscoped legacy or single-repository file shall retain the existing default launch when the known session evidence does not indicate multiple worktrees. An unscoped file with multiple known worktrees shall not silently select the first.
- **AC-UI-EXTERNAL-FILE-TARGET-001.5:** The existing editor menu, responsive entry points, labels, copy and folder actions shall retain their composition and interaction. A disabled external-editor item shall be exposed as disabled to keyboard and assistive technology.

## Exclusions

Folder launches, copy path, file-tree context menus, embedded editor dispatch and code-server transport, native editor interpretation, SSH URL construction, path containment, repository attachment, inventory repair, caching, and layout or copy redesign are outside this contract.

## Implementation plan

[Repository-scoped external file launch](../../../plans/repository-scoped-external-file-launch/plan.md).
