---
status: active
system: ui
created: 2026-08-02
owners:
  - kandev
---
# Repair merge commit details Requirements

## Overview

Preserve the observable behavior documented for Repair merge commit details.

## Requirements

### REQ-UI-MERGE-COMMIT-DETAILS-001: Repair merge commit details

**Intent:** Preserve the observable behavior documented for Repair merge commit details.

#### Acceptance criteria

- **AC-UI-MERGE-COMMIT-DETAILS-001.1:** Opening a merge commit shows the files changed between that commit and its first parent, matching GitHub's commit-detail semantics and the branch history represented by Kandev's commit list.
- **AC-UI-MERGE-COMMIT-DETAILS-001.2:** Each changed file retains its patch, status, additions, and deletions through the existing `CommitDiffResult.files` response.
- **AC-UI-MERGE-COMMIT-DETAILS-001.3:** A merge commit does not duplicate files by returning a separate diff for each parent or switch to a combined-diff view.
- **AC-UI-MERGE-COMMIT-DETAILS-001.4:** Ordinary single-parent commits and root commits keep their current diff behavior.
- **AC-UI-MERGE-COMMIT-DETAILS-001.5:** A genuinely empty commit continues to return an empty file collection and may display **No files in this commit**.
- **AC-UI-MERGE-COMMIT-DETAILS-001.6:** Single-commit details remain uncapped; cumulative-diff budget behavior is unchanged.
- **AC-UI-MERGE-COMMIT-DETAILS-001.7:** **GIVEN** a feature branch whose first-parent tip does not contain a file from the branch being merged, **WHEN** the user opens the resulting clean merge commit, **THEN** the incoming file and its patch are shown in commit details.
- **AC-UI-MERGE-COMMIT-DETAILS-001.8:** **GIVEN** a file already exists unchanged in the merge commit's first parent, **WHEN** the user opens the merge commit, **THEN** that first-parent-only file is not incorrectly presented as introduced by the merge.

## Migrated source detail

## Problem

Opening a clean merge commit from the Changes panel can show **No files in this
commit** even when the merge introduced files and GitHub shows a non-empty
commit diff. The commit row can still report the correct aggregate additions
and deletions, leaving the row and detail view visibly inconsistent.

## Desired behavior

- Opening a merge commit shows the files changed between that commit and its
  first parent, matching GitHub's commit-detail semantics and the branch history
  represented by Kandev's commit list.
- Each changed file retains its patch, status, additions, and deletions through
  the existing `CommitDiffResult.files` response.
- A merge commit does not duplicate files by returning a separate diff for each
  parent or switch to a combined-diff view.
- Ordinary single-parent commits and root commits keep their current diff
  behavior.
- A genuinely empty commit continues to return an empty file collection and may
  display **No files in this commit**.
- Single-commit details remain uncapped; cumulative-diff budget behavior is
  unchanged.

## Regression scenarios

- **GIVEN** a feature branch whose first-parent tip does not contain a file from
  the branch being merged, **WHEN** the user opens the resulting clean merge
  commit, **THEN** the incoming file and its patch are shown in commit details.
- **GIVEN** a file already exists unchanged in the merge commit's first parent,
  **WHEN** the user opens the merge commit, **THEN** that first-parent-only file
  is not incorrectly presented as introduced by the merge.
- **GIVEN** a normal single-parent or root commit with file changes, **WHEN** the
  user opens it, **THEN** its changed files continue to be shown.
- **GIVEN** a commit with no tree change from its parent, **WHEN** the user opens
  it, **THEN** the commit details may show the empty-file message.

## Constraints

- Preserve the existing `session.commit_diff` WebSocket action and
  `CommitDiffResult` response shape.
- Use the merge commit's first parent as the sole comparison base.
- Keep commit metadata lookup and multi-repository routing unchanged.

## Out of scope

- Adding parent-selection controls or combined-diff rendering.
- Changing commit-list aggregation, cumulative PR changes, or diff budgets.
- Changing the commit-details UI, empty-state copy, or mobile composition.
