---
id: "02-report-attachment-rollback-errors"
title: "Report attachment rollback cleanup errors"
status: done
wave: 2
depends_on:
  - "01-finalize-cache-aliases"
plan: "plan.md"
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-001
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-003
acceptance_criteria:
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.6
  - AC-TASKS-ATTACH-WORKSPACE-SOURCES-003.7
system_design:
  - ../../specs/tasks/system-design/workspace-repository-placement.md
  - ../../specs/tasks/system-design/runtime-cleanup.md
---

# Task 02: Report attachment rollback cleanup errors

## Summary

Include errors from worktree discovery and cleanup in the materialization error
returned when attachment compensation fails. Preserve the original attachment
failure and continue cleanup for newly-created worktrees already supplied to
the rollback path even when discovery fails.

This only makes compensation failures visible. It does not assert that cleanup
succeeded, repair inventory directly, or explain the separately observed
active inventory row in `inventory/live shared`.

## Scope

- `apps/backend/internal/backendapp/workspace_source_materializer.go` only.
- Return and join the `GetAllByTaskID` and `CleanupWorktrees` errors from
  `cleanupNewWorktrees` and its caller.
- Keep explicit additional-worktree cleanup running after discovery failure.
- Preserve known-worktree exclusion, physical-ID deduplication, and the
  worktree manager's existing audit, retention, and cleanup authority.

## Out of scope

- Changing canonical inventory, database rows, retry/reuse, retained-checkout,
  cleanup authority, active-reference checks, or any timeout.
- Weakening exact rollback assertions or claiming the active-row cause is
  solved. That cause remains unconfirmed and requires its own evidence.
- New API, schema, acceptance criteria, gates or verification framework.

## Behavior to preserve

1. The original materialization failure and any rollback list/cleanup failures
   are present in the returned error text; both rollback failures remain
   discoverable with `errors.Is` at the rollback boundary.
2. A worktree-list failure does not skip cleanup of explicitly supplied new
   worktrees. Previously known worktrees stay excluded, and duplicate
   non-empty physical IDs are attempted once.
3. Manager cleanup remains the only physical cleanup authority. No result
   claims that a rejected or failed cleanup removed a worktree or inventory
   row.

## Verification results

Disposable baseline overlay: the two focused error-propagation cases returned
nil and failed as expected (`1.073s`). The candidate overlay passed (`0.485s`).
The public `-race` fixture overlay passed (`1.258s`). The exact disposable overlay fixture and logs are retained outside the
repository automatic test collection.
Three focused cases are retained in the existing host-materialization test file,
using the real worktree manager with injected store faults. No new test file
or runtime test seam is added.

These checks establish error propagation and the existing public fixture flow.
They do not establish the cause of, or a fix for, the active inventory row
observed in `inventory/live shared`; preserve that limitation.

Implementation and focused validation are complete. Normal hooks, CI and
publication are separate results; Task 01 remains historically done.

Committed existing-file regression cases passed directly with fts5 and the
race detector (2.278s), covering both injected failures and preserved known-ID
exclusion/deduplication. The physical-inventory limitation above is unchanged.
