---
id: "01-protect-archived-worktrees"
title: "Protect archived worktrees from storage purge"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002
acceptance_criteria:
  - AC-TASKS-DIRTY-WORKTREE-ARCHIVE-002.5
system_design:
  - ../../specs/tasks/system-design/dirty-worktree-archive.md
---

# Task 01: Protect archived worktrees from storage purge

## Summary

Make an active archived worktree row a live storage reference. Prevent a
workspace that contains one from entering quarantine or being deleted from an
older quarantine entry.

## In scope

- Include archived active worktree paths in the authoritative workspace
  inventory; retain existing protection for borrowers and exclude deleted
  historical branch rows.
- Recheck active worktree ownership before regular and forced permanent
  deletion of a quarantined task root. Protect old entries, fail closed on
  inventory errors, and report protected entries through the existing bulk
  purge result.
- Add regressions for multi-repository roots, manual Run now, scheduled
  cleanup, eligible purge, force clear, and a pre-upgrade quarantine entry.

## Out of scope

- Retrying archived task cleanup or changing the storage scheduler default.
- Automatically restoring a previously quarantined workspace.

## Acceptance

- An archived task's active worktree protects its root from analysis candidate
  classification and quarantine; a deleted historical row does not.
- A pre-existing quarantine entry with an active worktree descendant remains
  restorable through both normal and forced purge.
- An inventory failure stops workspace mutation and exposes the failure.

## Verification

```bash
(cd apps/backend && go test ./internal/backendapp ./internal/system/storage/workspaces)
(cd apps/backend && make lint)
```

## Files likely touched

- `apps/backend/internal/backendapp/storage_inventory.go`
- `apps/backend/internal/backendapp/storage_inventory_test.go`
- `apps/backend/internal/system/storage/workspaces/provider.go`
- `apps/backend/internal/system/storage/workspaces/provider_test.go`
- `apps/backend/internal/backendapp/storage_quarantine_controller.go`
- `apps/backend/internal/backendapp/storage_maintenance_test.go`

## Dependencies

None.

## Risks

- Purge is a separate path from quarantine; fixing inventory selection alone
  does not protect an already-quarantined checkout.
- Preserve the existing bulk-purge distinction between protected and failed
  entries without masking an inventory error.

## Parallelism

`sequential`

## Inputs

- `REQ-TASKS-DIRTY-WORKTREE-ARCHIVE-002`, the paired system design, and the
  reclamation ADR.
- Existing storage inventory, provider, quarantine, and controller tests.

## Results

- `go test ./internal/backendapp ./internal/system/storage/workspaces` passed.
- `make lint` passed with 0 issues.
