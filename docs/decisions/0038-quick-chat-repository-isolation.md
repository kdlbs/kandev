# 0038: Quick Chat Repository Isolation

**Status:** accepted
**Date:** 2026-07-14
**Area:** backend, frontend

## Context

Quick chat already creates an ephemeral task and can associate one repository, but it resolves
the workspace default executor. When that executor is local, selecting another branch can
switch the branch in the user's source checkout. Adding multiple repositories amplifies that
risk, while cloning every repository into a new quick-chat-specific directory would duplicate
the existing task materialization and cleanup systems.

## Decision

Repo-backed quick chats use the built-in worktree executor and the existing multi-repository
task preparation pipeline. The quick-chat request accepts an ordered repository list and the
backend enforces the worktree executor whenever that list is non-empty. Repo-less chats retain
the current scratch workspace and resolved default executor.

The worktrees remain owned by the ephemeral task and use the existing task environment,
session worktree, rollback, deletion, expiration, and garbage-collection paths. The selected
base branch supplies committed repository state; uncommitted source-checkout changes are not
copied.

## Consequences

Concurrent chats can target the same repository and base branch without mutating the user's
checkout. Materialization stays smaller and faster than a full clone, and no second cleanup
system is introduced. A chat does not see uncommitted local changes, and repository-backed
quick chats always run through the local worktree executor rather than a workspace default
remote executor in this initial version.

## Alternatives Considered

- **Clone every repository under `quick-chat/`.** Rejected because it duplicates clone,
  credential, preparation, persistence, and cleanup behavior already owned by task worktrees.
- **Use the workspace default executor unchanged.** Rejected because a local executor may
  checkout the requested branch in the user's source directory.
- **Copy the current working directory.** Rejected because it silently includes uncommitted or
  ignored files and has unclear performance and privacy behavior.
