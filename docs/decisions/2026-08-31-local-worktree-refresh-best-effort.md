# ADR-2026-08-31-local-worktree-refresh-best-effort: Local Worktree Refresh Is Best Effort

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, security, operations

## Context

Kandev v0.92.1 made pull-before-worktree a universal admission gate. This rule
blocked host worktrees when the selected branch existed only locally.

A local Git worktree does not need remote publication. Users can create local
branches for parallel agents and keep all work off the remote.

Remote-only executors have a different constraint. They must fetch or clone a
branch when no usable local ref is available.

## Decision

Host worktree refresh is best effort when the selected local base exists.
Kandev attempts the configured fetch, then uses the local ref if refresh cannot
improve it.

Fetch errors, an absent remote branch, divergent refs, and uncertain ancestry
produce credential-safe warnings. These states do not block a host worktree
that has a valid local base.

Caller cancellation still stops preparation. Kandev does not create a fallback
worktree after a cancellation request.

Remote materialization remains strict when no usable local base exists. This
rule includes explicit remote-only refs and executors that must clone the
repository.

Worktree preparation never pushes a branch. It does not reset, rebase, merge,
or remove local history.

This decision supersedes the local-worktree admission boundary in
[ADR-2026-08-25-required-worktree-refresh-fails-closed](2026-08-25-required-worktree-refresh-fails-closed.md).

## Consequences

- Users can create worktrees from unpublished local branches.
- Origin outages do not block host work when a valid local base exists.
- Warnings show that the worktree can lack remote changes.
- Remote-only launch still stops when Kandev cannot materialize the branch.
- Local history remains authoritative when local and remote refs diverge.
- The `pull_before_worktree` setting controls a refresh attempt, not a universal
  freshness guarantee.

## Alternatives Considered

### Keep universal fail-closed refresh

Rejected. This rule makes remote publication a condition for local Git work.

### Require users to disable pull-before-worktree

Rejected. An intentional local branch is normal development, not an offline
repository configuration.

### Push the local branch before worktree creation

Rejected. Worktree preparation must not publish user history.

### Use the local ref without a warning

Rejected. The warning tells the user that remote changes can be absent.
