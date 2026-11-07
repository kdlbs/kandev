---
status: current
system: tasks
requirements:
  - REQ-TASKS-MULTI-BRANCH-001
created: 2026-11-03
owners:
  - kandev
---

# Multi-branch live commits

## Scope and requirement mapping

This design specifies live commit identity for
[multi-branch tasks](../requirements/multi-branch.md).
The repository-qualified update rules below implement
`AC-TASKS-MULTI-BRANCH-001.2`.
Tasks owns the repository and branch bindings. Workspaces owns checkout identity.

## Commit updates

The `session.git.event` handler passes each `commit_created` payload, including
`repository_name`, into `addSessionCommit` in the session-runtime slice.
Session-to-environment mapping selects the existing commit collection.

Within that collection, commit identity includes its repository scope and SHA.
`repository_name` identifies the checkout, including the branch suffix used by
additional worktrees. Missing and empty names both identify the legacy root.

A repeated notification updates its matching repository-qualified row.
Amend detection selects the first row in the incoming commit's normalized
repository scope. It replaces that row only when the parent SHA also matches.
This preserves an amendment when a sibling repository has a newer global row.
A shared parent across different scopes does not establish an amendment.

The existing full-history query continues to replace a complete snapshot.
No transport fields, persistence, Git commands, authorization, or refresh policy
change. PR history remains an independent provider projection.

## Responsive behavior and verification

Desktop and phone Changes use the same commit store and history projection.
Layout, navigation, scrolling, touch targets, and copy retain their current
contracts. No new presentation or ASCII UI preview is required.

Store regressions cover shared parents, identical SHAs in separate scopes,
same-scope amendments, legacy root normalization, and environment mapping.
An isolated browser scenario creates distinct commits from a shared base in two
bound repositories. Both open desktop and phone views must receive both live
commits before any reload or PR association.

See the [repair plan](../../../plans/repository-scoped-live-commits/plan.md).
The existing [multi-branch decision](../../../decisions/0013-multi-branch-tasks.md)
retains authority over task bindings and checkout layout.
