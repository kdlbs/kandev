---
created: 2026-11-15
status: done
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-005
system_design:
  - ../../specs/tasks/system-design/runtime-cleanup-preparation.md
legacy_specs: []
---

# Implementation Plan: Archive with failed workspace inspection

## Overview

Archive tasks even when physical workspace inspection fails. Preserve uncertain
resources and their cleanup evidence. Implement one sequential vertical slice.

## Evidence and root cause

Remote `main` was `585824a5d066d22785814c0394b88dd57f6c5ded` on 2026-11-15.
It matches this checkout. The reported runtime uses a November 9 installation.
Its cleanup identity implementation is unchanged on main.

The live child branch references a zero-byte commit object. A bounded Git HEAD
lookup timed out. With lazy fetching disabled, Git reported the empty object.
Backend logs at 00:23:41 and 00:34:30 UTC report `commit_lookup` with
`deadline_exceeded`. The parent and child remain unarchived.

`captureCleanupHeadOIDsForMode` uses all-or-error capture for archive.
`prepareCascadeResourceCleanupWithCompensation` returns that error before
archive mutation. The entire selected tree is blocked by the child's failure.
The existing delete recovery path demonstrates per-resource failure capture,
but its consent and destructive cleanup semantics must not be applied to archive.

## Scope

In scope: archive preparation, durable blocked resources, independent cleanup,
restart/unarchive fences, and existing archive response/event behavior.
Out of scope: repairing the user's clone, deleting unknown resources, changing
delete consent, new UI, remote provider redesign, and dependency upgrades.

## Technical approach

Follow the archive amendment in the referenced design. Reuse snapshot maps
instead of a new queue or schema. Extend preparation at both entry points.
Add a focused archive worker helper instead of expanding the large jobs file.
Audit every enclosing-path cleanup route. Keep group lifecycle errors distinct
from physical inspection failures. Existing healthy archive behavior remains.

Compatibility: local/worktree Git inspection receives the new policy. Shared
or borrowed resources keep their existing ownership guards. Remote runtime
stop and teardown retain their existing contracts. Unsupported providers are
not granted a new physical deletion capability.

## Tests

The initial direct/cascade and independent-cleanup tests failed on main before
production edits. The shared-resource completion test also failed before its
correction. Other cases cover the resulting contract and unchanged paths.

- `resource_cleanup_archive_inspection_test.go`:
  `TestArchiveInspectionFailureCommitsTree` covers .1 with direct and cascade
  archive through the real service. `handoff_cascade_archive_inspection_test.go`
  adds `TestArchiveInspectionFailureAutoArchive` for scheduled archive.
- `resource_cleanup_archive_inspection_test.go`:
  `TestArchiveInspectionFailurePreservesResources` covers .2 and .3 with
  restart, partial manifests, independent healthy cleanup, and enclosing paths.
  `TestArchiveInspectionFailureReclaimCannotReplaceOriginalIdentity` covers .2.
  `TestArchiveInspectionFailureKeepsAdmissionErrorsFatal` covers .4 with
  internal timeout, parent deadline, database failure, and snapshot failure.
  `TestArchiveInspectionFailureUnarchiveFencesCleanup` covers .4.
- `manager_cleanup_preparation_inspection_test.go`: extend the temporary Git
  fixture with an empty HEAD object. Disable lazy fetching for deterministic,
  offline evidence. Do not reproduce corruption in the user's clone.

## E2E tests

Add a handler-to-service-to-SQLite integration test in
`apps/backend/internal/task/handlers/task_archive_inspection_test.go` named
`TestArchiveInspectionFailureHTTPCommitsTree`. Inject the real cleanup service
with a physical-inspection failure. Assert successful archive response, both
persisted archive timestamps, normal task update publication, and retained
workspace files (.1 through .4). This is the end-to-end API contract test.
No rendered UI changes are planned. Existing desktop/mobile archive navigation
continues to consume the same successful response and events.

## Work orders

- [x] [Task 01: Separate archive admission from physical inspection](task-01-archive-inspection.md)

## Verification results

Implementation checks passed:

- Work-order service tests: passed (1.605 s).
- Expanded service selection `Archive|Cleanup|RetainDeletion|TaskDelete` with
  `-race`: passed (24.425 s).
- Worktree capture/inspection/preparation tests: passed (5.193 s).
- HTTP archive integration test: passed (0.384 s).
- Specification catalog, specification lint, and diff whitespace: passed.
- Public documentation validator tests: 63 passed. All 55 pages validated.
- Changed Go lint: passed, zero issues.

The live instance and damaged clone were not modified. No deployment occurred.

## Implementation notes

The service records per-resource completion and preserves partial manifests.
The archived-worktree reclaimer also checks original inspection failures, so
its separate sweep cannot bypass the cleanup snapshot. Shared resources remain
uncompleted. All new fixtures use temporary databases and files.

## Risks

Unknown original identity can require operator recovery and retain disk usage.
An enclosing-directory cleaner could bypass a per-worktree block. Tests must
prove that no alternate removal path can do this. Snapshot persistence errors
must remain fatal. A partial manifest cannot mark the whole job captured.

## PR review validation

Review remediation preserves removed-path evidence and removes stopped runtime
rows after partial cleanup. New archive snapshots require saved source manifests
before background reclaim. Legacy snapshots retain their previous behavior.
The archive/cleanup race suite and HTTP archive regression passed. See the work
order for exact commands and regression results.
