---
id: TEMP-PRESSURE-02
title: Entry breakdown and cleanup guidance
status: done
wave: 2
depends_on:
  - TEMP-PRESSURE-01
plan: plan.md
requirements:
  - REQ-SYSTEM-PAGE-TEMP-PRESSURE-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.1
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.2
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.3
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.4
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.5
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.6
  - AC-SYSTEM-PAGE-TEMP-PRESSURE-002.7
system_design:
  - ../../specs/system-page/system-design/temporary-storage-pressure.md
---

# Task 02: Entry breakdown and cleanup guidance

## Summary

Show the largest observed entries within each temporary root.
Explain ownership limits and connect operators to existing registered-artifact cleanup.

## In scope

- Opt-in child summaries in the existing scanner, bounded results, and exact registry classification.
- Overview projection, cache compatibility, responsive entry details, and focus navigation to existing cleanup.
- Desktop/mobile E2E and the temporary-file explanation in `docs/public/operations.md`.

## Out of scope

- Arbitrary deletion, new cache cleanup providers, task attribution guesses, or changes to temporary variables.
- Maintenance admission changes and workspace symlink remediation.

## Acceptance

- One bounded traversal produces correct top-twenty child sizes, observed remainder, and honest partial results without changing strict scanner callers.
- Ownership classification uses exact registry validation. Review navigation invokes no mutation and preserves all existing cleanup checks.
- Desktop and phone expose names, statuses, scope, and limitations through the same data and actions, with focused browser evidence.

## ASCII UI preview

UI-02 from the [full plan](plan.md#ascii-ui-preview), covering all linked criteria:

```text
Desktop: /tmp  Partial  Largest observed entries
         <entry name>       <GB>       Not tracked by Kandev
         Other observed     <GB>       Unscanned usage unknown
         [Review Kandev cleanup]

Phone:   /tmp  Partial
         <entry name wraps>
         <GB>  Not tracked by Kandev
         [Review Kandev cleanup]
         Only registered files across this installation.
```

Use an inline list with the page as its only scroller. Phone action targets measure at least 44 pixels.
Review expands and focuses the existing registered-artifact section. It does not submit cleanup.
Unknown, empty, stale, and no-candidate states follow the full plan.

## Verification

All commands start from the repository root. Task 01 supplies the dependency installation.

```bash
(cd apps/backend && go test ./internal/system/storage/... ./internal/backendapp -count=1)
(cd apps/web && pnpm test components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-overview-card-bars.test.tsx components/settings/system/storage/storage-overview-resources.test.ts components/settings/system/storage/storage-totals.test.ts components/settings/system/storage/storage-confirmation-dialogs.test.tsx hooks/domains/system/use-storage-maintenance.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/system/storage-temporary-folders.spec.ts tests/system/storage-analysis-bars.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-temporary-folders.spec.ts tests/system/mobile-storage-analysis-bars.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use disposable roots for real scan evidence and controlled responses for pressure and partial states.
Retain existing policy-save and explicit-cleanup coverage. Capture and inspect desktop and phone screenshots.

## Files likely touched

- `apps/backend/internal/system/storage/filescan/measure.go` and `measure_test.go`.
- `apps/backend/internal/system/storage/tempstore/provider.go` and `provider_test.go`.
- `apps/backend/internal/backendapp/storage_maintenance.go` and its tests.
- `apps/backend/internal/system/storage/overview_cache.go` and progress tests if projection changes require them.
- `apps/web/lib/types/system.ts`.
- `apps/web/components/settings/system/storage/` overview, resource mapping, and extracted temporary detail components.
- `apps/web/e2e/helpers/storage-maintenance.ts` and the existing temporary-folder and analysis-bar specs.
- `apps/web/src/locales/` system catalogs and `docs/public/operations.md`.

## Dependencies

Task 01 provides capacity warnings and the navigation entry point.

## Risks

- A partial ranking cannot claim to contain the actual largest entries in unscanned data.
- Shared caches remain untracked even when their names contain Kandev.
- The existing cleanup may not materially reduce pressure when untracked entries dominate.

## Parallelism

`sequential`

## Inputs

- Design sections Breakdown contract, Cleanup boundary, and Presentation and mobile contract.
- Existing scanner partition tests, ownership-marker tests, overview rows, and responsive confirmation flow.

## Results

Completed on 2026-09-28.

- The existing scan returns a bounded top-twenty direct-entry breakdown and observed remainder from the same traversal; ties, partial children, and unknown sizes are covered by tests.
- Ownership labels require an exact registered path and valid ownership marker. The UI labels uncertain or untracked entries and routes operators to the existing cleanup section without invoking cleanup.
- Backend scanner, temp-store, ownership, handler, and backend-app tests passed. The focused frontend suite passed 129 tests across 11 files; typecheck and changed-file ESLint passed.
- Desktop temporary-storage E2E passed 5 tests and mobile E2E passed 4 tests, including review navigation, no-mutation behavior, touch sizing, and horizontal-overflow checks.
- Public-doc tests passed 62 tests and the validator accepted 47 pages. Specification validation and lint passed. `git diff --check` passed.
