---
created: 2026-10-27
status: implemented
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-004
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
legacy_specs: []
---

# Implementation Plan: Archive reclaim writer admission

## Overview

Avoid reserving the database writer for an archive-reclaim operation that is
already persisted. The implementation is published in
[PR #4849](https://github.com/kdlbs/kandev/pull/4849), at runtime head
`20ad8f128eae95e593cf7aee2aba2e31b786df0d`.
`implemented` records the source result, not merge, deployment or hosted CI success.

This delivery record follows the existing
[archive requirements](../../specs/tasks/requirements/dirty-worktree-archive.md)
and [archive design](../../specs/tasks/system-design/dirty-worktree-archive.md).
The [original reclamation package](../archived-worktree-reclamation/plan.md) and
[immediate archive package](../immediate-archive-worktree-removal/plan.md) remain
delivery history; this repair does not restore dirty-worktree retention.

## Evidence and cause

Candidate reconciliation repeatedly submits the same task, worktree and archive
generation. The unique-key INSERT does nothing for an existing operation, but
admission previously reserved the writer and evaluated the managed-deletion
barrier first. A slow no-op can therefore queue unrelated writers.

A consistent-copy case contained 40 exact persisted operations. Repeating them
opened 40 writer transactions, and a duplicate could not complete while the
writer was reserved. After the correction, the same operations open zero writer
transactions and all complete while the writer remains reserved.

The native cause of a reported minute-scale SQLite step was not independently
reproduced. The established result is removal of unnecessary duplicate admission,
not a measured guarantee about the operational incident.

## Scope and approach

One sequential work order covers `createTaskResourceCleanupJob` in
`apps/backend/internal/task/repository/sqlite/resource_cleanup.go`.
After incoming managed-envelope validation, an archive-reclaim-only reader
lookup checks the existing unique operation ID. A hit creates nothing and
preserves existing job state. A miss retains the original writer transaction,
recovery claim, canonical task lock, managed-owner barrier, archive-generation
comparison and unique-key conflict handling. Read failures remain errors.

No UI, public API, schema/index, deletion policy, health timeout, feature flag,
operating-data change or permanent test framework is included. Generic cleanup
admission and physical worktree removal remain unchanged.

## Work orders

- [x] [Task 01: Skip persisted reclaim admission](task-01-skip-persisted-reclaim-admission.md)

## Verification and follow-up

The work order records original focused validation and its limits. Its temporary
copy/contention and admission cases, existing unarchive controls and persistence
race checks passed. Unchanged runtime checks are reused for this docs-only work.

The original PR head contained one runtime file without a changed work order.
Run `40480440622`, job `122109092445`, failed with
`No changed work order covers the triggering paths.` This package supplies the
missing delivery record and real requirement/design links. It does not change
the trusted coverage evaluator, its criteria or exemptions.

The documentation-only follow-up passed catalog validation (403 decisions and
1,747 specifications), specification lint and whitespace checks. The existing
trusted coverage evaluator accepted the runtime path plus these two documents:
`covered`, `ok: true`, no errors, and the declared archive requirement/design.
Hosted coverage remains pending until the same PR receives the documents and
reports a fresh result.

### Maintainer follow-up: permanent admission regressions

The contributor PR now also has permanent repository tests in
`apps/backend/internal/task/repository/sqlite/resource_cleanup_archive_reclaim_test.go`.
They use separate file-backed SQLite writer and reader pools. The duplicate
operation test holds a writer transaction and confirms that the persisted job
returns unchanged while the writer remains held. Other tests cover current and
stale archive generations, reader errors, cancellation, incoming managed
envelopes, recovery claims, and managed-deletion barriers. New operation misses
also remain blocked by recovery claims and managed-deletion barriers. Existing
service concurrency and unarchive tests remain in place.

The focused repository test command passed after these additions:

```text
go test -trimpath -tags fts5 -p 2 ./internal/task/repository/sqlite -run '^TestCreateArchiveReclaimTaskResourceCleanupJob' -count=1
```

The changed-file Go lint passed with `0 issues`. This is local focused evidence;
it does not replace a fresh hosted CI result.

## Risks

A reader hit is a no-op, not new deletion authority. A concurrent miss must still
pass every writer-side guard; an uncertain read must not admit an operation.
Do not report a persisted failed/cancelled job as newly activated or confuse
source implementation with physical cleanup or deployment.
