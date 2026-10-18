---
id: "02-reconcile-shared-inventory"
title: "Apply and recover exact reconciliation"
status: done
wave: 2
depends_on:
  - "01-discover-and-preview"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.5
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.4
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.5
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.6
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 02: Apply and recover exact reconciliation

## Summary

Apply a complete version-2 selection as one metadata transaction. Preserve the
physical worktrees and conversations while restoring one canonical environment
owner, with safe repeat application and dependency-ordered rollback.

## In scope

- Extend offline admission for the selected graph. Preserve eligible idle session
  states only after locks, complete process census, runtime checks, and claim checks.
- Journal exact originals and raw timestamps. Remove selected alias slots, rebind
  sessions and proven Git snapshots, correct explicit workspace paths, then remove
  fully redundant environments. Preserve all other fields and foreign records.
- Validate every FK and non-FK dependency. Reject unsupported references and any
  non-inert cleanup job. Recheck negative observations before publication.
- Support negative row observations, dependency-aware rollback, and failpoints
  around commit and fence release. Keep version-1 recovery behavior working.

## Out of scope

Filesystem moves, Git mutations, task-state changes, cleanup supersession,
new platform support, and operating on a developer installation.

## Acceptance

1. `TestAliasApply` preserves files, staged/index state, refs, markers, task/session
   history, provider resume tokens, and snapshots for both generic incident shapes.
2. `TestAliasRecovery` proves complete rollback, stale observation refusal, negative
   claim checks, and idempotent recovery before/after commit and before fence release.
3. Busy, unknown, partial, mixed-state, or unsupported selections refuse the whole
   mutation. Unresolved journals continue to block normal backend startup.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair ./internal/backendapp/ownershiplock -count=1)
(cd apps/backend && go test -trimpath -race ./internal/backendapp -run InventoryRepair -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/inventoryrepair/apply.go apps/backend/internal/task/inventoryrepair/mutations.go apps/backend/internal/task/inventoryrepair/journal.go apps/backend/internal/task/inventoryrepair/observations.go apps/backend/internal/task/inventoryrepair/read.go apps/backend/internal/task/inventoryrepair/alias_apply.go apps/backend/internal/task/inventoryrepair/alias_dependencies.go apps/backend/internal/task/inventoryrepair/alias_apply_test.go apps/backend/internal/task/inventoryrepair/alias_recovery_test.go
git diff --check
```

Update the lint invocation if final helper paths differ or additional files
change. Record the final exact invocation. No schema migration is planned.

## Files likely touched

- `apps/backend/internal/task/inventoryrepair/{apply,mutations,journal,observations,read}.go`
- New `alias_apply.go`, `alias_dependencies.go`, `alias_apply_test.go`, and
  `alias_recovery_test.go` in the same package.
- Existing CLI mode dispatch and backend startup-fence tests only where necessary.

## Dependencies

Task 01's versioned selection, proof, and complete observation graph.

## Risks

Deleting an environment before rebinding its snapshots loses evidence through
FK cascades. Reverse recovery must restore parent rows before their dependents.
An idle session can still have a live agentctl consumer.

## Parallelism

`sequential`

## Inputs

- Linked design sections on dependent records, admission, and journal recovery.
- Existing `apply_test.go`, `recovery_test.go`, and process-census fixtures.

## Results

Passed on disposable fixtures:

- Full repair, CLI, and ownership-lock race tests passed (320.728s, 1.220s,
  and 4.047s respectively).
- Backend startup-fence tests passed (1.302s).
- SQL guard passed after adding a narrow exemption for SQLite-only offline
  schema inspection.
- Store conformance race tests passed (191.778s).
- Scoped lint passed with zero issues. Final invocation:
  `bash scripts/lint-go-changed apps/backend/internal/task/inventoryrepair/alias_apply.go apps/backend/cmd/worktree-inventory-repair/main.go`.
  The helper checks every Go file in both changed packages.
- Apply, rollback, interruption recovery, stale observations, dependent rows,
  raw timestamps, and original consumer-path process checks passed.

An environment interruption invalidated two earlier test runs. Both full runs
above were restarted and completed with recorded exit status zero. No live
installation data changed.

PR review follow-up (2026-10-18): preview now records existing foreign-key
violations in the affected inventory tables. It refuses an invalid selected row
before creating a fence. Publication rejects new violations; unrelated legacy
violations no longer block application or rollback. SQLite enforcement stays on.
A regression with 1,100 unrelated violations reproduced the fenced recovery
failure before this fix. Coverage includes an unrelated violation in an affected
table, rollback from a journal without the new baseline, and refusal of an
invalid selected row. The existing complete-session selection guard also gained
empty, partial, and complete workspace-correction tests; its refusals passed
before production changes and required no extra guard.

Final focused race validation passed:

```bash
go -C apps/backend test -race ./internal/task/inventoryrepair -run 'TestAlias(Repair|Workspace|Recovery)|TestRecovery' -count=1
go -C apps/backend test -race ./cmd/worktree-inventory-repair -count=1
go -C apps/backend run ./cmd/sqlguard ./internal
bash scripts/lint-go-changed apps/backend/internal/task/inventoryrepair/alias_foreign_keys.go apps/backend/internal/task/repository/sqlite/task_environment_identity.go
```

The repair tests passed in 87.834s and the CLI tests in 1.223s. SQL guard and
scoped lint passed with zero issues. Specification validation and lint passed.
All database corruption and recovery checks used disposable fixtures.

The full `inventoryrepair` race package also passed (522.743s). That invocation
included a misspelled CLI package path and therefore exited 1 overall; the
correct CLI package was run separately and passed as recorded above.
