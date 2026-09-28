---
id: "02-apply-repair"
title: "Apply and recover journaled inventory repairs"
status: complete
wave: 2
depends_on:
  - "01-preview-repair"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.4
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.5
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.2
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 02: Apply and recover journaled inventory repairs

## Summary

Implement exact offline application with backups, recoverable moves, and guarded
SQL publication. Preserve original cleanup evidence and create an explicitly
linked recovery generation for an incomplete old snapshot.

## In scope

- Reuse backend home/database ownership locks; reject active or unknown consumers.
- Private SQLite backup, durable journal, full preflight, no-follow filesystem
  checks, `git worktree move`, and exact old-value SQL comparisons.
- Startup refusal for an unresolved utility journal before migrations/recovery;
  restart or rollback using the same operation identity under ownership locks.
- Repository/branch corrections, exact slot relocation, and dependent workspace
  references. No new inventory rows or foreign marker rewriting.
- Conditional cancellation of the selected non-running cleanup predecessor and
  a unique successor retaining resource scope, consent, and completed progress.
  Preserve the old snapshot. Add successor linkage and observation time to the
  serialized snapshot without dropping them on normal cleanup progress writes.
- Document the utility and maintenance sequence through the docs-maintainer skill.

## Out of scope

Live installation application, global cleanup resets, branch deletion, automatic
foreign-root adoption, broad source-manifest recapture, and remote repair.

## Acceptance

1. Fault injection before/after the move and SQL commit proves resumable or
   reversible application, startup fencing, unchanged original refs/content/index,
   and no publication after a stale tuple or failed verification.
2. A real SQLite cleanup fixture preserves the predecessor snapshot byte-for-byte,
   fills complete successor identities including missing HEAD evidence, resumes
   idempotently, and retains absent-sibling observations and unrelated progress.
3. Repaired fixtures pass real cleanup and selected-environment launch admission;
   dirty, borrowed, changed-commit, or truly foreign worktrees still refuse
   destructive action. Runtime startup uses a recording provider, never a real
   agent in a test.

## Verification

```bash
(cd apps/backend && go test -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair ./internal/worktree ./internal/task/service ./internal/orchestrator/executor ./internal/backendapp/ownershiplock -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run 'InventoryRepair' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1)
node scripts/validate-public-docs.mjs
git diff --check
```

The proposed startup regression must have `InventoryRepair` in its test name.
The utility is SQLite-only and does not add migrations; existing PostgreSQL
application behavior must remain unchanged.

## Files likely touched

- Proposed `apps/backend/internal/task/inventoryrepair/{apply,journal,recovery,cleanup}.go`
- Proposed corresponding `*_test.go` files, with SQLite and real Git fixtures
- `apps/backend/cmd/worktree-inventory-repair/main.go`
- `apps/backend/internal/backendapp/main.go` and a new focused startup test file
- `apps/backend/internal/task/service/resource_cleanup_jobs.go`
- New focused cleanup/executor integration test files
- `docs/public/git-operations.md` and the appropriate existing maintenance section
  selected through docs-maintainer; do not duplicate public guidance

## Dependencies

Task 01. Read the design's journaled application and cleanup evidence sections,
existing ownershiplock implementation, recovery admission tests, and source
manifest boundary tests. Preserve adjacent plans' delivery status.

## Risks

Filesystem and database operations cannot commit atomically. Unknown backup,
journal, liveness, or rollback state must remain a refusal. A missing legacy
head-map entry is not permission to rewrite the old observation.

## Parallelism

`sequential`

## Results

Implemented private verified SQLite snapshots, durable journals and startup fences, exact transactional updates, relocation, verification, rollback, and linked cleanup successors. Race tests pass for all interruption boundaries, unchanged refs/index/content and unrelated column bytes, real worktree admission, dirty-checkout refusal, real source capture, and actual cleanup-worker replay. Startup-fence tests and SQL/store-conformance checks pass. The full affected-package race run passed (inventoryrepair, CLI, worktree, task/service, orchestrator/executor, ownershiplock); the backendapp startup regression and persistence/storeconformance run also passed. Scoped lint, SQL guard, public docs, specification lint/catalog, documentation-coverage preflight, and whitespace checks passed. The standalone utility built successfully; a real backend binary refused an unresolved repair before database creation.
