---
id: "01-remove-archived-task-directory"
title: "Complete archived task directory cleanup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-003
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.1
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.2
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.3
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-003.4
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
---

# Task 01: Complete Archived Task Directory Cleanup

## Summary

Implement the final archive cleanup step that removes the managed task root
and its remaining files once all worktrees are reclaimed. Keep the obligation
retryable after worktree deletion and apply it to delayed reclamation.

## In scope

- TDD regression `TestArchiveTaskCleanupRemovesNonEmptyTaskDir`, reconstructed
  from the disposable reproduction documented in the plan. Prove it fails
  because the worktree disappears while the nonempty task parent remains.
- Archive-only root cleanup capability and original/reclaim job integration.
- Complete sibling inventory, borrowed-environment/runtime/generation guards,
  pinned no-follow removal, and durable retry after partial cleanup/restart.
- All regressions named in the plan and public archive explanation.

## Out of scope

Delete consent, remote directories, rendered UI, branch policy, historic-root
backfill, and unrelated refactoring. Preserve individual-worktree empty-parent
cleanup tests; do not replace them with unconditional recursive deletion.

## Acceptance

1. A real archive removes its clean worktrees and nonempty task root while
   preserving base/source/sibling task directories and branch recovery metadata.
2. Retained siblings, borrowers, unknown ownership, and failed stops prevent
   root deletion; final successful reclaim completes it under the same guards.
3. Root failures retry across restart after worktree removal, unarchive fences
   stale deletion, and symlink/path replacement cannot redirect deletion.

## Verification

Run from the repository root. Execute the named regression alone for red, then
the package tests for green and retained behavior coverage. Ensure Node 24 is
on PATH for public-documentation validators; this session has an existing mise
installation but no `node` command on its default PATH.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestArchiveTaskCleanupRemovesNonEmptyTaskDir$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/service ./internal/worktree ./internal/system/storage/workspaces -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/task/service/resource_cleanup_jobs.go \
  apps/backend/internal/task/service/resource_cleanup_taskdir.go \
  apps/backend/internal/task/service/resource_cleanup_taskdir_test.go \
  apps/backend/internal/task/service/resource_cleanup_taskdir_compat_test.go \
  apps/backend/internal/task/service/resource_cleanup_taskdir_deferred_test.go \
  apps/backend/internal/task/service/resource_cleanup_task_owned_test.go \
  apps/backend/internal/task/service/resource_cleanup_archive_identity_test.go \
  apps/backend/internal/task/service/resource_cleanup_archive_reclaim_test.go \
  apps/backend/internal/task/service/resource_cleanup_missing_worktree_test.go \
  apps/backend/internal/worktree/manager_cleanup_taskdir.go \
  apps/backend/internal/worktree/manager_archive_taskdir_test.go \
  apps/backend/internal/system/storage/workspaces/directory_handle_info_unix.go \
  apps/backend/internal/system/storage/workspaces/directory_handle_info_windows.go
(cd apps/backend && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -trimpath -c -o /tmp/kandev-taskdir-workspaces-windows.test.exe ./internal/system/storage/workspaces)
scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --project chromium e2e/tests/task/archived-session-recovery.spec.ts
scripts/run-quiet e2e --summary -- corepack pnpm@9.15.9 --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-archived-session-recovery.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/service/resource_cleanup_taskdir.go` (new)
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- `apps/backend/internal/task/service/resource_cleanup_taskdir_test.go` (new)
- `apps/backend/internal/task/service/resource_cleanup_task_owned_test.go`
- `apps/backend/internal/task/service/resource_cleanup_archive_identity_test.go`
- `apps/backend/internal/task/service/resource_cleanup_archive_reclaim_test.go`
- `apps/backend/internal/task/service/resource_cleanup_missing_worktree_test.go`
- `apps/backend/internal/worktree/manager_cleanup_taskdir.go` (new)
- `apps/backend/internal/worktree/manager_archive_taskdir_test.go` (new)
- `apps/backend/internal/system/storage/workspaces/directory_handle_info_unix.go`
- `apps/backend/internal/system/storage/workspaces/directory_handle_info_windows.go`
- `docs/public/tasks-and-workflows.md`
- Owning requirement, design, this work order, and `plan.md` for results/status

## Dependencies

None. Reuse existing directory handles; change storage primitives only if a
specific regression demonstrates a missing capability and record that scope.

## Risks

The reclaim path currently treats deleted rows as stale. Root failure cannot
be silently converted into stale success. Older snapshot payloads must not
grant deletion authority without authoritative root proof. Avoid holding SQL
transactions across filesystem waits, and preserve existing lock order.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/dirty-worktree-archive.md), requirement 003.
- [System design](../../specs/tasks/system-design/dirty-worktree-archive.md), archived task container cleanup.
- [Decision](../../decisions/2026-10-13-archived-task-directory-cleanup.md).
- Existing `manager_cleanup_taskdir_test.go`, `resource_cleanup_task_owned_test.go`,
  and `resource_cleanup_archive_identity_test.go` fixtures.
- Scoped backend guidance and `/tdd` before implementation.

## PR remediation scope

- Preserve legacy checkout cleanup when no recursive root can be admitted.
- Accept the production checkout workspace shape as well as the task root.
- Retain deferred folder obligations and missing deleted-row proof across retry.
- Deduplicate persisted paths while retaining the seed path for deleted rows.
- Reproduce the failed desktop and mobile archived-session recovery specs.

## Results

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
