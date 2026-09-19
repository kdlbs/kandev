---
status: draft
system: ui
created: 2026-08-04
owners:
  - kandev
---
# Repair PR-only commit details Requirements

## Overview

Preserve the observable behavior documented for Repair PR-only commit details.

## Requirements

### REQ-UI-PR-ONLY-COMMIT-DETAILS-001: Repair PR-only commit details

**Intent:** Preserve the observable behavior documented for Repair PR-only commit details.

#### Acceptance criteria

- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.1:** A commit row and its detail view use one explicit source of truth.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.2:** A SHA present in the local session commit feed is a **local commit**, even when the same SHA is also present in the linked pull request. It keeps the current local `session.commit_diff` behavior and local commit actions.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.3:** A SHA present only in the selected linked GitHub pull request is a **GitHub commit**. Its source identity includes the workspace, owner, repository, and SHA.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.4:** A GitHub commit whose list response has no statistics does not display a fabricated `+0 -0`. A known empty commit may display zero additions and zero deletions only when those values came from commit-detail data.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.5:** Opening a GitHub commit lazily loads its metadata, aggregate statistics, changed files, and patches from GitHub's individual commit endpoint. It does not depend on the commit object being present in the task worktree.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.6:** Desktop and mobile commit details render the same source-aware data.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.7:** GitHub-only commit details are read-only relative to the task worktree. They do not expose reset, revert, amend, open-current-worktree-file, or local context-expansion controls.
- **AC-UI-PR-ONLY-COMMIT-DETAILS-001.8:** A GitHub request failure produces the existing visible error treatment and never silently falls back to local `git show` data.

## Migrated source detail

## Problem

The Changes panel combines commits discovered in a task session's local
worktree with commits returned by the task's linked GitHub pull request. When a
pull request is force-pushed after the task worktree is created, those sources
can describe different histories.

PR-only rows currently inherit zero-valued statistics from GitHub's pull
request commit-list response and are opened through the local
`session.commit_diff` action. The list response does not contain commit stats,
and the remote commit may not exist in the task worktree. As a result, Kandev
can show false `+0 -0` totals, an empty or failed detail view, metadata from the
wrong source, and local reset/revert/amend controls for a commit that is not in
the local branch.

## Desired behavior

- A commit row and its detail view use one explicit source of truth.
- A SHA present in the local session commit feed is a **local commit**, even
  when the same SHA is also present in the linked pull request. It keeps the
  current local `session.commit_diff` behavior and local commit actions.
- A SHA present only in the selected linked GitHub pull request is a
  **GitHub commit**. Its source identity includes the workspace, owner,
  repository, and SHA.
- A GitHub commit whose list response has no statistics does not display a
  fabricated `+0 -0`. A known empty commit may display zero additions and zero
  deletions only when those values came from commit-detail data.
- Opening a GitHub commit lazily loads its metadata, aggregate statistics,
  changed files, and patches from GitHub's individual commit endpoint. It does
  not depend on the commit object being present in the task worktree.
- Desktop and mobile commit details render the same source-aware data.
- GitHub-only commit details are read-only relative to the task worktree. They
  do not expose reset, revert, amend, open-current-worktree-file, or local
  context-expansion controls.
- A GitHub request failure produces the existing visible error treatment and
  never silently falls back to local `git show` data.
- Viewing a GitHub commit never fetches, checks out, resets, or otherwise
  mutates the task worktree.

## API contract

Add a workspace-scoped WebSocket request for an individual GitHub commit. The
request carries `workspace_id`, `owner`, `repo`, and an exact commit SHA. The
response carries commit metadata, exact additions/deletions/files-changed
statistics, and the existing GitHub-style file records needed by the diff
viewer.

The backend must authorize the requested repository through the same
workspace-scoped credential and repository checks used by the existing GitHub
pull request actions. Pull request commit-list data must distinguish
unavailable statistics from known zero statistics; omitted data must not be
serialized or interpreted as measured zeroes.

The individual-commit response must merge GitHub's paginated file records so a
commit is not presented as complete after only the first page. Provider limits
or unavailable patches remain explicit upstream limitations rather than a
reason to consult the local worktree.

## Regression scenarios

- **GIVEN** a task worktree at an obsolete pull request head and a linked pull
  request that was force-pushed to new SHAs, **WHEN** the user opens a new
  PR-only commit, **THEN** Kandev shows the GitHub commit's metadata, exact
  statistics, files, and patches without invoking the local commit-diff path.
- **GIVEN** a PR-only commit returned by the pull request commit-list endpoint,
  **WHEN** its row is rendered before detail data is loaded, **THEN** the row
  omits statistics instead of displaying `+0 -0`.
- **GIVEN** the same SHA is present in both the local and pull request feeds,
  **WHEN** the user opens it, **THEN** it is deduplicated as a local commit and
  keeps the existing local detail path and local actions.
- **GIVEN** two linked repositories contain PR-only commits, **WHEN** either
  commit is opened, **THEN** the request uses that row's workspace, owner,
  repository, and SHA rather than the active worktree's repository.
- **GIVEN** a GitHub-only commit on mobile, **WHEN** the user taps the row,
  **THEN** the existing full-height commit sheet renders the remote patch and
  closes normally without exposing local commit actions.
- **GIVEN** GitHub rejects or cannot serve the individual commit request,
  **WHEN** the user opens the row, **THEN** Kandev shows an error and does not
  substitute local worktree content.

## Constraints

- Preserve the current local commit-detail flow and action semantics.
- Fetch individual GitHub commit details lazily; do not issue one request per
  row while rendering the commit list.
- Reuse the current desktop panel and mobile full-height sheet rather than
  adding a new detail composition.
- Keep GitHub patches in their existing patch representation, including the
  current binary, unavailable, or truncated-patch treatment.
- Route any changed user-facing copy through i18n. No new explanatory copy is
  required for this repair.

## Out of scope

- Automatically fetching or checking out the pull request head in the task
  worktree.
- Resetting or rebasing a stale task branch.
- Adding a general stale-worktree warning or synchronization workflow.
- Eagerly enriching every pull request commit row with individual commit data.
- Parent selection or combined-diff controls for merge commits.
- Persistence or database schema changes.
- Adding equivalent remote-commit providers for non-GitHub integrations.
