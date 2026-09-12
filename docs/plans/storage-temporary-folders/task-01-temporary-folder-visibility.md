---
id: "01-temporary-folder-visibility"
title: "Temporary-folder visibility"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.8
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.9
system_design:
  - ../../specs/system-page/system-design/storage-temporary-folders.md
---

# Task 01: Temporary-folder visibility

## Summary

Add the read-only temporary-folder footprint from provider through desktop and phone analysis.
Keep it visibly separate from the classified total.

## In scope

- Root resolution, duplicate/nested roots, mount boundaries, partial errors, deadline, and bounded scanning.
- Overview source identity, cache/progress projection, API types, resource rows, and translations.
- Unit and integration tests for scope, read-only behavior, rendering, and aggregate exclusion.

## Out of scope

Cleanup policy, new artifact producers, public docs publication, and unrelated source attribution.

## Acceptance

- The source measures only server-selected roots and never mutates them or traverses unsafe boundaries.
- Cached and progressive responses preserve partial/unavailable distinctions.
- Desktop and phone show root details without adding this footprint to Total counted.

## ASCII UI preview

UI-01 excerpt, [full desktop/phone preview](plan.md#ascii-ui-preview):

```text
v System temporary folders
  <GB>  Read-only
  Informational. Can overlap counted categories.
  /tmp
  <GB>  Complete | Partial | Unavailable
```

Map to AC-003.3, .5, .6, .8.
Phones wrap paths and retain one page scroll owner and 44-pixel row targets.

## Verification

Run from repository root. In a fresh worktree, install dependencies once from `apps/`.

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
(cd apps/backend && rtk go test ./internal/system/storage/...)
(cd apps/backend && rtk go test ./internal/backendapp -run 'Test(Storage|OverviewCache)')
(cd apps/web && rtk pnpm exec vitest run components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-totals.test.ts)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
```

Task 03 owns rendered browser evidence after integration.
The dependency install is unnecessary when this worktree already has a valid install.

## Files likely touched

- New `apps/backend/internal/system/storage/tempstore/` provider and platform mount readers.
- `apps/backend/internal/system/storage/filescan/measure.go` and tests.
- `apps/backend/internal/backendapp/storage_maintenance.go` and tests.
- `apps/backend/internal/system/storage/handler.go`, `analysis_state.go`, `overview_cache.go`, and tests.
- `apps/web/lib/types/system.ts`.
- `apps/web/components/settings/system/storage/storage-overview-resources.ts`, `storage-overview-card.tsx`, `storage-totals.ts`, and tests.
- `apps/web/src/locales/` and affected source-count fixtures.

## Dependencies

None. Read the existing database provider and progressive cache as implementation patterns.

## Risks

Do not weaken strict scanner callers while adding tolerant temp traversal.
A Windows or macOS mount reader needs focused platform tests even when Linux is the development host.

## Parallelism

sequential

## Inputs

[Requirements](../../specs/system-page/requirements/storage-maintenance.md), requirement 003.
[Design](../../specs/system-page/system-design/storage-temporary-folders.md), Read-only reader through Overview contract and Presentation.

## Results

Implemented. Added the isolated system temporary-folder provider, bounded tolerant scanning,
overview source/progress/cache wiring, localized resource rendering, and desktop/mobile contracts.
Focused backend and frontend tests, typecheck, and i18n validation passed. Review remediation added
real-scanner coverage for empty and nested regular directories, preserved sampled bytes from an
interrupted tolerant partition, and verified refreshed injected mount tables for new nested mounts
and same-path replacement. The changed backend packages pass 1,110 tests across 9 packages.
