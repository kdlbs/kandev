---
status: draft
system: integrations
created: 2026-09-23
owners:
  - kandev
---

# GitHub review task cleanup requirements

## Overview

A GitHub Review Watch retains a record for each task that it creates. Routine
cleanup checks the pull request and applies the watch's cleanup policy. An
archived task remains in that record so an unarchive or an explicit watch action
can still find it. Routine cleanup must not spend GitHub requests on archived
tasks or delete them.

Integrations owns this contract because it owns GitHub Review Watches, their
cleanup policies, and their provider requests. The task system owns the archive
state. Platform owns shared provider backoff.

## Requirements

### REQ-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001: Archive-aware routine cleanup

**Intent:** Routine review cleanup shall evaluate eligible tasks without
rechecking archived tasks or changing explicit watch actions.

#### Acceptance criteria

- **AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.1:** When a review task is
  archived, scheduled cleanup shall make no pull-request feedback request for
  that task. It shall retain the task and its review-watch record under an
  enabled or disabled watch.
- **AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.2:** When an archived review task
  is unarchived, the next scheduled cleanup shall evaluate it under the watch's
  current policy. Unarchiving shall not create a second task for the same
  watch and pull request.
- **AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.3:** Scheduled cleanup shall
  continue to evaluate active review tasks, incomplete task reservations, and
  records whose task was hard-deleted. A missing task shall not hide a record
  from orphan recovery.
- **AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.4:** Under Auto cleanup, a task
  retained by a user-authored message or an enabled pull-request lifecycle
  prompt shall not trigger a feedback request on each scheduled cleanup.
  A failed retention check shall leave the task and record intact.
- **AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.5:** Explicit cleanup, watch
  deletion, and watch reset shall keep their existing policy and deletion
  behavior for archived tasks. Routine filtering shall not hide archived tasks
  from these actions.

## Out of scope

- Changing the meaning of Auto, Always delete, or Never.
- Deleting archived task history or review-watch records during migration.
- Changing GitHub issue-watch cleanup or task-owned PR-watch identity.
- New settings controls or rendered UI states.

## Related contracts

- [Provider failure backoff](../../platform/requirements/pr-watch-and-bounded-storage.md#req-platform-provider-backoff-001-generation-aware-provider-failure-backoff)
- [System design](../system-design/github-review-task-cleanup.md)
