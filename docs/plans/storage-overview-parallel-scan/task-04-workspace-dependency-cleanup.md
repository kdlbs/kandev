---
id: "04-workspace-dependency-cleanup"
title: "Add opt-in dependency cleanup"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/system-page/storage-overview-parallel-scan.md"
---

# Task 04: Add opt-in dependency cleanup

## Acceptance

- Storage settings persist a default-off workspace dependency-cleanup option and reject malformed
  settings without changing the previous saved value.
- Scheduled and full manual maintenance can prune only the fixed allowlist beneath Kandev-owned
  task workspaces whose task is archived or deleted and whose authoritative inventory has no active
  references.
- The resource-specific `workspace_dependencies` run uses the existing maintenance activity gate,
  busy-resource response, Run anyway path, cancellation, retry, and history persistence.
- The provider never follows symlinks, crosses the task workspace root, deletes ambiguous directory
  names, or mutates active/borrowed/restored workspaces. It revalidates before each deletion.
- Results report successfully pruned directories, reclaimed bytes, skips, and warnings, and retries
  are idempotent. The directory count is the number of directories actually removed; skipped paths
  are represented in warnings.
- A workspace can be restored after pruning with source and recovery metadata intact, with a clear
  indication that dependencies may need to be installed again.
- The Workspaces policy card visibly lists every allowlisted directory Kandev checks before the
  option is enabled. An adjacent info control exposes the recursive-search, excluded-folder,
  safety, and reinstall explanation on hover, keyboard focus, and touch activation.

## Verification

```bash
cd apps/backend && go test -race ./internal/system/storage ./internal/system/storage/workspaces ./internal/backendapp ./internal/task/service
cd apps && pnpm --filter @kandev/web test -- --run components/settings/system/storage/storage-policy-card.test.tsx components/settings/system/storage/storage-maintenance-settings.test.tsx
```

Add filesystem fixtures for every allowlist entry and negative fixtures for `vendor`, `target`,
`.cache`, `.git`, symlinked directories, escaped paths, active workspaces, and incomplete inventory.
Use channels/barriers for state-change races rather than sleep-based tests.

## Files likely touched

- `apps/backend/internal/system/storage/types.go`
- `apps/backend/internal/system/storage/settings.go`
- `apps/backend/internal/system/storage/workspaces/types.go`
- `apps/backend/internal/system/storage/workspaces/provider.go`
- `apps/backend/internal/system/storage/workspaces/provider_test.go`
- `apps/backend/internal/system/storage/runner.go`
- `apps/backend/internal/system/storage/operations.go`
- `apps/backend/internal/system/storage/operations_test.go`
- `apps/backend/internal/backendapp/storage_maintenance.go`
- `apps/backend/internal/backendapp/storage_inventory.go`
- `apps/backend/internal/task/service/*` only if task/reactivation or restore coordination requires
  an existing cleanup hook to be extended
- `apps/web/lib/types/system.ts`
- `apps/web/components/settings/system/storage/storage-policy-card.tsx`
- `apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
- `apps/web/components/settings/system/storage/storage-setting-help.tsx`
- `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
- `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
- localized system translation resources

## Dependencies

None for the backend contract. Task 02 integrates this behavior with the full desktop/mobile
regression suite.

## Inputs

- Spec: fixed allowlist, eligibility, retention interaction, safety guarantees, and run selection
- Existing workspace candidate/quarantine logic in `workspaces/provider.go`
- Existing authoritative inventory in `backendapp/storage_inventory.go`
- Existing settings normalization and `selectCleanupProviders` resource-selection behavior
- Existing quarantine restore and task resource cleanup tests

## Output contract

Return a compact handoff capsule with settings/API changes, provider safety results, race-test
results, run-history examples, user-facing warning copy, and any remaining package-manager coverage
gaps. Mark the task done only when no path can be deleted outside the allowlist and owned roots.

## Result

Implemented the persisted default-off setting and `workspace_dependencies` provider. The provider
fails closed on incomplete inventory, only accepts the exact allowlist (`node_modules`,
`bower_components`, `.pnpm-store`, `.yarn/cache`, `.yarn/unplugged`, `.venv`, `venv`, `.tox`,
`.nox`, `__pypackages__`, `Pods`, `.gradle`), skips ambiguous folders, never follows symlinks, and
revalidates ownership, containment, protection, and archived/deleted eligibility before each
deletion. Tests cover archived/deleted selection, active protection, exclusions, symlink payloads,
incomplete inventory, cancellation, byte/directory/workspace counts, default-off provider
selection, and restore after pruning with source and ownership metadata intact. The policy card
displays every allowlisted folder and the existing hover/focus/tap help control explains the
recursive scope and reinstall consequence.
