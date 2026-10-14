---
created: 2026-10-13
status: implemented
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-003
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
legacy_specs: []
---

# Implementation Plan: Remove Archived Task Directories

## Overview

Complete task archive cleanup by removing the managed task directory and loose
files or nested folders after every worktree is safely reclaimed. One sequential
work order covers original archive, delayed reclamation, regression coverage,
retry behavior, and the public archive explanation.

Tasks owns this outcome because archive identity and cleanup jobs authorize
terminal resource removal. Workspace directory primitives implement that
decision; storage maintenance does not decide whether archive is complete.

## Confirmed root cause and evidence

`Manager.tryRemoveEmptyTaskDir` calls `os.Remove` on the parent of a removed
worktree. Nonempty parents are retained and the failure is logged at debug
level. `Service.cleanupDestructiveTaskResources` has no later recursive task
root step, so the archive cleanup job succeeds with leftover files present.

A temporary `TestReproArchiveTaskDirectoryWithLeftovers` used the real task
service, SQLite repository, worktree manager, and `ArchiveTask`. It created a
clean checkout plus `notes.txt` and `artifacts/nested/output.txt` beside it.
The job reached `succeeded` and the checkout disappeared; the assertion that
the parent disappeared failed with `stat` returning nil. Command:

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/service -run '^TestReproArchiveTaskDirectoryWithLeftovers$' -count=1 -v)
```

The throwaway reproduction was removed after diagnosis. Permanent regressions
are created during implementation.

## Scope

### In scope

- Remove an owned managed task root recursively after all worktrees are removed.
- Apply the final step to direct, cascade, scheduled archive and final delayed
  worktree reclamation, using their existing archive identity guards.
- Preserve roots containing retained dirty, shared, failed, or unknown worktrees.
- Retain a retryable root obligation when worktree removal succeeds but root
  deletion fails, including after restart.
- Update the archive section of `docs/public/tasks-and-workflows.md` with the
  files-beside-worktrees deletion behavior when implementation ships.

### Out of scope

- Changing dirty-worktree preservation, delete consent, branch policy, remote
  executor cleanup, session deletion, or unrelated storage maintenance.
- Changing rendered UI, public API shapes, or feature flags.
- Backfilling task roots from historic succeeded jobs with no remaining active
  checkout; this fix applies to archive/reclaim work executed after the update.

## Technical approach

Add a narrow archive-only manager capability in a focused task-directory file.
Consume it through a service-local interface declared with existing cleanup
contracts in `resource_cleanup_taskdir.go`. Keep `tryRemoveEmptyTaskDir` empty-only.

Call the capability from original archive cleanup in `executeTaskResourceCleanupJob` and
from `processArchiveReclaimJob` in `resource_cleanup_jobs.go`, before job success.
Read the complete owning environment inventory, not just the filtered cleanup
batch. Defer while any recorded sibling remains active or unknown. Revalidate
runtime-stop success, borrowers, archive generation, exact managed-root layout,
owner marker, and pinned directory identity before recursive removal.

Retain root target/proof in existing JSON snapshots before destructive I/O.
Extend snapshot payloads with native root identity, the current task owner, the
creation marker, and admitted worktree paths. Use absent-tolerant decoding and
fresh authoritative proof for older jobs. The creation marker remains unchanged after an admitted ownership transfer.
Root errors must retry even after the
worktree record becomes deleted; the reclaim path's current stale-row shortcut
must not discard the pending root obligation. Use existing no-follow directory
handles, lifecycle fencing, and lock order; no new database schema is expected.
Update removed-root recording used by orphan-process cleanup where applicable.

## Tests

- `resource_cleanup_taskdir_test.go`: `TestArchiveTaskCleanupRemovesNonEmptyTaskDir`
  covers a loose file, nested folder, retained environment/branch history, and
  preservation of tasks base and another task (003.1).
- The same suite covers a clean and dirty sibling, a borrower, stop failure,
  lookup failure, and partial batch failure (003.2).
- `resource_cleanup_taskdir_test.go`: reclaiming the last clean sibling
  removes the root; earlier siblings cannot remove it (003.2, 003.4).
- Service fault-injection coverage proves root failure retries after worktree
  deletion and restart, and unarchive fences a delayed root attempt (003.3).
- `manager_archive_taskdir_test.go`: preserve existing individual-removal and
  sibling tests; add root-operation tests for foreign markers, path escapes,
  root/parent symlink replacement, internal symlink targets, and cancellation
  (003.1, 003.3, 003.4).

## End-to-end evidence

Backend service integration drives the real archive and durable worker through
filesystem removal and retained metadata assertions. Exercise direct archive
and a cascade descendant, plus the shared scheduled path. Browser interaction
does not establish filesystem removal and no rendered UI changes are planned.

## Work orders

- [x] [Task 01: Complete archived task directory cleanup](task-01-remove-archived-task-directory.md)

## Verification results

Implemented final managed-root cleanup for archive and delayed reclamation.
The snapshot retains native filesystem identity, current task owner, original
creation marker, and admitted paths. Root failures remain retryable after
worktree deletion and restart; retained siblings, borrowers, live runtimes,
unsafe paths, and replaced roots remain protected. Individual worktree removal
keeps its empty-only parent behavior.

Verification:

- Red: `TestArchiveTaskCleanupRemovesNonEmptyTaskDir` failed with the worktree
  gone and nonempty task root present, then passed after implementation.
- `go test -trimpath -race -tags fts5 ./internal/task/service ./internal/worktree ./internal/system/storage/workspaces -count=1`
  passed: service 100.390s, worktree 105.276s, filesystem 1.121s.
- After the final ownership-transfer correction,
  `go test -trimpath -race -tags fts5 ./internal/task/service ./internal/worktree -run 'TestArchiveTaskDirectory|TestArchiveTaskCleanupRemovesNonEmptyTaskDir|TestArchiveReclaimRemovesNonEmptyTaskDir|TestCascadeArchiveRemovesDescendantTaskDirectory|TestCleanupTaskDirectory|Test.*OrphanReap' -count=1`
  passed: service 6.001s and worktree 3.015s. This reran the new regressions and
  orphan-reaping coverage after the last production edit.
- The documented `scripts/lint-go-changed` command passed with zero issues.
  Lint followed race/build checks sequentially, per current backend guidance.
- The documented Windows filesystem-package cross-compilation passed.
- Catalog validation passed (382 decisions, 1603 specifications), specification
  lint passed, and all 36 specification-linter tests passed.
- Public-document validators passed: 63 tests and 48 published pages.
- Harness checks after the base update passed: 19 tests and 204 files.
- Documentation coverage preflight and `git diff --check` passed.

Restart fixtures now retain the original tasks base and actual environment
root. New directory tests stop the automatic worker and drive the durable job
processor explicitly, avoiding competing claims. The public task/workflow
how-to explains removal of files beside reclaimed worktrees.

PR publication and exact-head CI/review follow-up are tracked in the task plan
and reported separately from this local verification record.

## Risks

- The last worktree may already be deleted on retry; root identity must survive.
- Recursive removal in the individual-worktree helper would destroy siblings.
- Marker removal during empty-parent cleanup must not destroy retry proof.
- Unarchive, borrowers, root replacement, and runtime-stop failures must remain
  destructive cleanup barriers.

## Related completed packages

[Dirty archive preservation](../dirty-worktree-archive/plan.md) and
[archived-worktree reclamation](../archived-worktree-reclamation/plan.md) remain
completed delivery records for requirements 001 and 002. This pending package
extends that contract with 003; it does not reopen or replace their results.

## PR remediation results

- Review findings now preserve individually audited checkout removal for legacy
  unmarked/unsupported roots, changed task bases, multi-root inventory, and
  unrelated workspace paths. The production primary-checkout workspace shape
  admits the parent root. Persisted root removers still fail closed.
- Deferred roots with no active checkout retain durable retry; missing deleted
  repository evidence stays retryable. Worker/restart tests cover both cases.
- Captured worktree paths are deduplicated, retaining the initial path when
  active inventory omits deleted history. Platform identity helpers already
  reject nil and typed-nil handles; no wrapper was added for that suggestion.
- `go test -trimpath -race -tags fts5 ./internal/task/service ./internal/worktree ./internal/system/storage/workspaces -count=1`
  passed (101.018s, 102.441s, 1.149s).
- Scoped Go lint passed with zero issues.
- Desktop archived-session recovery: the documented managed runner passed
  all 3 tests (25.4s). Phone recovery reused that fresh build and passed all
  3 tests (20.8s). These reproduce the failed CI shard cases.
- Specification catalog/lint and diff checks passed. Exact-head remote checks
  and thread disposition remain externally pending until the fixup push settles.
