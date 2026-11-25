---
id: "01-release-archived-borrower-holds"
title: "Release archived borrower holds"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
  - REQ-TASKS-SHARED-WORKTREE-HOLDS-001
acceptance_criteria:
  - AC-TASKS-RUNTIME-CLEANUP-001.3
  - AC-TASKS-SHARED-WORKTREE-HOLDS-001.1
  - AC-TASKS-SHARED-WORKTREE-HOLDS-001.2
  - AC-TASKS-SHARED-WORKTREE-HOLDS-001.3
system_design:
  - ../../specs/tasks/system-design/shared-worktree-holds.md
---

# Task 01: Release Archived Borrower Holds

## Summary

Narrow `CountActiveWorktreeReferences` so an archived, terminal borrower session
with no runtime row stops holding the owner's worktree. Reclaim then removes
pinned worktrees, including those left by earlier versions, and borrower history
survives.

## In scope

- Change the query and doc comment in `store.go`, and the comments in `manager.go`.
- Add the tests and matrix named in [the plan](plan.md#tests), writing the
  failing regression first.
- Correct the archived borrower statement in `docs/public/git-operations.md`.

## Out of scope

- Everything listed as out of scope in [the plan](plan.md#out-of-scope).

## Acceptance

- `TestCleanupWorktrees_RemovesWorktreeHeldOnlyByArchivedBorrower` fails before
  the query change with `active_reference` retention, and passes after it.
- Any borrower outside the released combination still retains the worktree, and
  one such borrower alongside a released one is enough.
- Reclaim of a pinned worktree succeeds after a restart, and the borrower's
  session and messages remain.

## Verification

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run 'ArchivedBorrower|MixedBorrowers|UnjoinableBorrower' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run 'ArchiveReclaimKeepsBorrowerHistory|ArchiveReclaimDefersForLiveBorrower|ResumeReclaimsWorktreePinned|StartupWakeReclaimsDeferred' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/inventoryrepair -run 'TestLegacyAliasLifecycleCanonicalCascade|TestLegacyAliasLifecycle$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 -timeout 20m ./internal/task/service ./internal/worktree -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/task/inventoryrepair -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/store.go apps/backend/internal/worktree/manager.go apps/backend/internal/worktree/manager_cleanup_archived_borrower_test.go apps/backend/internal/task/service/resource_cleanup_archive_borrower_test.go apps/backend/internal/task/service/service_task_priority_test.go apps/backend/internal/task/inventoryrepair/legacy_alias_cascade_test.go apps/backend/internal/task/inventoryrepair/legacy_alias_lifecycle_test.go
```

## Files likely touched

- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/manager.go`
- `apps/backend/internal/worktree/manager_cleanup_archived_borrower_test.go`
- `apps/backend/internal/task/service/resource_cleanup_archive_borrower_test.go`
- `apps/backend/internal/task/service/service_task_priority_test.go`
- `apps/backend/internal/task/inventoryrepair/legacy_alias_cascade_test.go`
- `apps/backend/internal/task/inventoryrepair/legacy_alias_lifecycle_test.go`
- `docs/public/git-operations.md`

## Dependencies

None.

## Risks

- A test fixture without an `executors_running` or `tasks` table would now fail
  the query. The current worktree fixtures use the full task repository schema.

## Results

- `CountActiveWorktreeReferences` now releases a borrower only when its task is
  archived, its session is `COMPLETED`, `FAILED`, or `CANCELLED`, and it has no
  `executors_running` row. Comments and the public recovery note match the rule.
- Added the matrix, unjoinable-hold, removal, mixed-retention, reclaim,
  deferral, deferred-job wake, and restart tests named in the plan. The removal
  and deferred-job wake regressions fail before the query change with
  `active_reference` retention. The wake test keeps the future recheck in place
  so only startup wake can admit the job.
- Updated the inventory-repair cascade controls: an archived terminal borrower
  no longer keeps the shared checkout, while its session association remains.
- The shared worktree system design declares both requirements the work order
  lists, so the trusted PR documentation coverage evaluator reports `covered`.
- All verification commands pass, including the exact
  `bash scripts/lint-go-changed` command (0 issues) and the inventory-repair
  race run. `go build -trimpath ./...` also succeeds.
- The full race run also surfaced a pre-existing fixture race in
  `TestUpdateTask_PriorityOnlyUsesFieldScopedRepositoryWrite`; that test now
  stops the cleanup worker before swapping `svc.tasks`.
