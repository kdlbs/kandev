# ADR-2026-09-27-managed-clone-relocation-boundary: Relocate worktrees across managed clone changes

**Status:** accepted
**Date:** 2026-09-27
**Area:** backend, frontend

## Context

The July 2026 move to workspace-scoped managed clones updated a repository's
`local_path` without moving older task worktrees. A task worktree can therefore
be healthy and retain user files while its Git common directory differs from
the repository row. The September 2026 launch guard correctly rejects that
mismatch, but generic recovery actions cannot repair it.

## Decision

Keep source-clone isolation and the read-only Git identity guard. Treat a
verified clone relocation as a separate task-environment recovery operation
before launch admission. Automatically relocate only a fully clean worktree
with a proven source, destination, branch, and commit. Require an explicit
action when files or staging state make the move non-transparent. Retain the
original checkout and recovery snapshot. Never substitute a base branch or
start an agent from an unverified clone.

The task environment owns physical relocation, with the same durable claim and
owner-generation fencing used for worktree metadata recovery. A mutable
repository source path does not rewrite an existing worktree's physical owner.

## Consequences

- Clean legacy tasks can resume after an authenticated relocation.
- Dirty tasks receive a specific repair action and a warning about staging
  choices. Their original files remain available.
- Task worktree inventory needs durable source-clone identity and a migration.
- Retained originals and snapshots consume disk until separate cleanup policy
  handles them.
- Local and remote executor contracts remain unchanged.

## Alternatives considered

- Ignore the common-directory mismatch when remote URLs match. This would
  weaken workspace clone isolation and accept an arbitrary lookalike checkout.
- Rewrite `.git` pointers to the new clone. Git administrative state, index,
  and object ownership make this unsafe for a live or dirty worktree.
- Reject every old worktree and ask users to start a new task. This strands
  valid branches, files, and provider conversations.
- Move every dirty worktree automatically. A content copy cannot promise to
  preserve staging choices, so user intent is required.

## Related specifications

- [Requirements](../specs/tasks/requirements/managed-clone-relocation.md)
- [System design](../specs/tasks/system-design/managed-clone-relocation.md)
