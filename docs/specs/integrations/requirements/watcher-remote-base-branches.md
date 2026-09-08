---
status: draft
system: integrations
created: 2026-09-01
owners:
  - kandev
---

# Watcher Remote Base Branches Requirements

## Overview

Issue and review watchers can bind their generated tasks to one repository and
base branch. The integration system owns this watcher configuration and must
preserve the distinction between a local branch and its remote-tracking branch.
This behavior addresses GitHub issue #3262 and applies to every integration
that uses the shared watcher repository control.

## Terminology

- **Qualified remote ref:** A remote-tracking branch that includes its remote
  name, such as `origin/main`.
- **Local ref:** A branch name without a remote qualifier, such as `main`.

## Requirements

### REQ-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001: Qualified watcher base branches

**Intent:** Let a watcher start generated tasks from an explicitly selected
remote-tracking branch without removing the existing local and default choices.

**User story:** As a user who imports external work automatically, I want to
select `origin/main` as the watcher base branch, so that the task launch follows
the repository's remote-refresh policy instead of silently selecting local
`main`.

#### Acceptance criteria

- **AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.1:** When a repository has
  both local `main` and remote `origin/main`, a watcher base-branch selector
  shall show both values as distinct choices.
- **AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.2:** When a user saves a
  watcher with `origin/main`, the watcher shall retain the qualified ref after
  reload and generated tasks shall record that exact base ref.
- **AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.3:** The repository-default
  choice and local branch choices shall keep their current behavior.
- **AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.4:** The watcher
  base-branch selector shall provide branch search, an explicit refresh action,
  and visible labels that distinguish local branches from the supported
  `origin` remote's branches.
- **AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.5:** Desktop and phone
  watcher dialogs shall provide the same qualified remote-ref choices and
  selector capabilities through controls that remain usable in their existing
  dialog surfaces.

## Out of scope

- Adding a watcher-specific pull or fetch policy.
- Changing the repository-level worktree refresh policy.
- Resetting, rebasing, merging, or deleting a user's local branch.
- Supporting arbitrary remote names in backend validation; watcher task launch
  continues to accept the existing `origin/<branch>` contract.
