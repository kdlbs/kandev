---
status: draft
system: workspaces
created: 2026-08-25
owners:
  - kandev
---

# Worktree Base Refresh Requirements

## Overview

The workspace system owns repository state and the base ref used to create a
task worktree. When a workspace enables pull-before-worktree behavior, users
expect each new or recreated worktree to start from verified remote state.
Kandev must not start an agent on an unverified local copy after a required
refresh fails.

## Terminology

- **Required refresh:** The remote synchronization gate enabled by a
  repository's pull-before-worktree setting.
- **Fetched remote base:** The remote-tracking ref produced by the successful
  fetch for the requested base branch.
- **Comparable refs:** Two refs where either ref contains every commit in the
  other ref.

## Requirements

### REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001: Verified Worktree Base

**Intent:** Prevent an agent from starting on stale repository state when the
workspace requires a remote refresh before worktree creation.

**User story:** As a workspace user, I want a required repository refresh to
stop task launch when it fails, so that an agent cannot edit and push from an
old copy without my knowledge.

#### Acceptance criteria

- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1:** When pull-before-worktree is
  disabled, Kandev shall allow a new or recreated worktree to use available
  local refs without requiring remote access.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2:** When pull-before-worktree is
  enabled, Kandev shall complete the remote refresh through the repository's
  configured transport and credential route before it starts an agent in a new
  or recreated worktree.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3:** When a required fetch fails
  because of authentication, network access, timeout, cancellation, or another
  Git error, Kandev shall stop worktree preparation and shall not use a stale
  local fallback ref.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4:** When the required fetch
  succeeds, Kandev shall choose a start ref that contains the fetched remote
  base. It can preserve local-only commits only when the local base also
  contains the fetched remote base.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5:** When the local base and fetched
  remote base have diverged, or Kandev cannot prove their ancestry, Kandev
  shall stop worktree preparation without resetting, deleting, or hiding either
  ref.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6:** When required refresh fails
  for any repository in a multi-repository task, Kandev shall not start the
  task agent and shall identify the affected repository in a credential-safe
  launch error.
- **AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7:** When required refresh fails
  during initial launch or a resume that creates or recreates a worktree,
  Kandev shall expose the failure through the existing durable task launch-error
  projection.

## Out of scope

- Making setup-script failures fatal.
- Refreshing a worktree that Kandev reuses without creating or recreating it.
- Adding a background repository-refresh service.
- Automatically resetting, rebasing, merging, or deleting local refs.
- Changing agent Git transport after the agent starts.

## System design

- [Worktree Base Refresh System Design](../system-design/worktree-base-refresh.md)

