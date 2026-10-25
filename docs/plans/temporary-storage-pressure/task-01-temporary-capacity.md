---
id: TEMP-PRESSURE-01
title: Temporary capacity warnings
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-SYSTEM-PAGE-TEMP-PRESSURE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-001.1
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-001.2
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-001.3
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-001.4
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-001.5
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-001.6
system_design:
  - ../../specs/system-page/system-design/temporary-storage-pressure.md
---

# Task 01: Temporary capacity warnings

## Summary

Expose temporary filesystem capacity independently of directory analysis.
Show pressure on desktop and phone without changing tool configuration.

## In scope

- Additive disk API, canonical root resolution, capacity identity, and independent failure results.
- Visible-tab polling, timestamps, stale state, backward compatibility, and warning UI.
- Desktop/mobile E2E and the capacity explanation in `docs/public/operations.md`.

## Out of scope

- Directory breakdown, cleanup behavior, cache configuration, and new settings.

## Acceptance

- A healthy home and critical temporary filesystem show distinct capacity states before folder analysis completes.
- Polling, visibility, refresh, and stale-response handling meet all linked criteria without recursive polling scans.
- Desktop and phone show localized warnings, accessible navigation to the existing temporary row, and valid unavailable states.

## ASCII UI preview

UI-01 from the [full plan](plan.md#ascii-ui-preview), covering all linked criteria:

```text
Desktop: Home <path> 60% | Temporary /tmp 95% Critical
         <GB> available  | <GB> available, updated <time>
                         [View largest entries]

Phone:   Temporary storage
         /tmp  95%  Critical
         <GB> available, updated <time>
         [View largest entries]
```

Both use inline capacity cards before analysis. The navigation opens existing temporary details until Task 02 adds entry rows.
Preserve one page scroller, 28-pixel desktop actions, 44-pixel phone targets, and long-path wrapping.
Unavailable and stale states follow UI-01 in the plan.

## Verification

For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.
All commands below start from the repository root.

```bash
(cd apps/backend && go test ./internal/system/metrics ./internal/system/storage ./internal/system/storage/tempstore ./internal/backendapp -count=1)
(cd apps/backend && probe_dir="$(mktemp -d)" && trap 'rm -rf "$probe_dir"' EXIT && GOOS=darwin CGO_ENABLED=0 go test -c -o "$probe_dir/tempstore-darwin.test" ./internal/system/storage/tempstore && GOOS=windows CGO_ENABLED=0 go test -c -o "$probe_dir/tempstore-windows.test.exe" ./internal/system/storage/tempstore)
(cd apps/web && pnpm test components/settings/system/storage/storage-disk-capacity-card.test.tsx components/settings/system/storage/storage-maintenance-settings.test.tsx hooks/domains/system/use-storage-maintenance.test.tsx hooks/domains/system/use-storage-maintenance-terminal-refresh.test.tsx lib/api/domains/system-api.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/system/storage-temporary-folders.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-temporary-folders.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Add platform reader tests where adapters change. The compile commands verify Darwin and Windows build compatibility, not runtime behavior.
Their output directory is temporary and removed afterward.
No tests use the host temporary root or fill a real disk.

## Files likely touched

- `apps/backend/internal/system/storage/handler.go` and `handler_test.go`.
- `apps/backend/internal/system/storage/tempstore/` root resolution and platform identity adapters.
- `apps/backend/internal/backendapp/storage_maintenance.go` and its tests.
- `apps/web/lib/types/system.ts` and the existing system store disk projection.
- `apps/web/hooks/domains/system/use-storage-maintenance.ts` and its tests.
- `apps/web/components/settings/system/storage/` capacity and parent components.
- `apps/web/components/settings/system/storage-settings.tsx` for Host visibility.
- `apps/web/e2e/tests/system/storage-temporary-folders.spec.ts` and the mobile counterpart.
- `apps/web/src/locales/` system catalogs and `docs/public/operations.md`.

## Dependencies

None. Existing capacity and temporary measurement contracts remain compatible.

## Risks

- Root failures must not erase successful sibling values.
- Home-capacity compatibility must survive temporary-reader errors.
- Background requests must remain scoped to the current backend, identity, and visible Host tab.

## Parallelism

`sequential`

## Inputs

- Design sections Capacity contract, Refresh and failure behavior, and Presentation and mobile contract.
- Existing `StorageDiskCapacityCard`, `metrics.DiskUsage`, and temporary-folder E2E fixtures.

## Results

Completed on 2026-09-28.

- The disk API retains home-capacity fields and adds isolated temporary-root results, timestamps, aliases, and filesystem relationship.
- The Host view refreshes capacity while visible, retains stale successful values, and renders localized desktop and phone warnings before folder analysis finishes.
- `go test ./internal/system/metrics ./internal/system/storage ./internal/system/storage/tempstore ./internal/backendapp -count=1` passed. Darwin and Windows temp-store test binaries compiled.
- The focused frontend suite passed 129 tests across 11 files; typecheck and changed-file ESLint passed.
- Desktop temporary-storage E2E passed 5 tests and mobile E2E passed 4 tests. Both E2E runs built the backend and Vite production bundle.
- `pnpm run i18n:check`, both new-code ratchets, and `git diff --check` passed.
