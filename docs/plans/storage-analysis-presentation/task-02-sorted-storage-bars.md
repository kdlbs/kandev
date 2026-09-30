---
id: "02-sorted-storage-bars"
title: "Sorted storage bars"
status: done
wave: 2
depends_on:
  - "01-timeout-feedback"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-005
acceptance_criteria:
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.1
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.2
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.3
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.4
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.5
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.6
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.7
  - AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005.8
system_design:
  - ../../specs/system-page/system-design/storage-analysis-presentation.md
---

# Task 02: Sorted storage bars

## Summary

Add relative usage bars to all measured storage headers and sort by descending numeric size.
Preserve the existing Accordion, details, actions, refresh behavior, and phone access.

## In scope

- Numeric size projection for every existing category, including optional and subset rows.
- Stable sorting, common scale, zero and unknown states, and visible partial status.
- Desktop and phone header composition, localization, and accessibility.
- Unit, component, and browser coverage for order, scale, expansion, refresh, focus, and geometry.
- Public operations guidance for relative bars and unchanged counted totals.

## Out of scope

New categories, saved preferences, sort controls, host-capacity thresholds, chart libraries,
new scan behavior, nested resource sorting, cleanup actions, and backend schema changes.

## Acceptance

1. Eligible rows sort by descending bytes with stable ties and proportional fills. Missing measurements remain distinct from zero.
2. Existing expansion, focus, details, actions, and stale-snapshot behavior survive sorting and refresh.
3. Desktop and phone headers match the preview and pass localization, accessibility, geometry, and existing total checks.

## ASCII UI preview

UI-01: Desktop measured rows. UI-02: Phone measured rows.
See the [combined preview and state matrix](plan.md#ascii-ui-preview).

```text
Desktop:
Bars compare category sizes. Categories can overlap.
Task workspaces          [####################] 68.71 GB v
System temporary folders [###############.....] 51.08 GB ^
                                                Partial
  /tmp: 51.08 GB | Partial | 1,015 entries skipped
  Scan timed out. Showing partial usage.
Quarantined resources    [##############......] 47.26 GB v

Phone:
Task workspaces          68.71 GB  v
[##############################]
System temporary folders          ^
51.08 GB | Partial
[######################........]
  /tmp: 51.08 GB
  Partial | 1,015 entries skipped
  Scan timed out.
  Showing partial usage.
```

UI-03: Unknown and zero states use the same row structure.

```text
Database                 [....................]  0.00 GB v
Task workspaces                                 Scanning v
```

Maps to all AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-005 criteria.
Required structure: shared scale, full-row activation, inline details, wrapped phone labels, and one page scroll owner.
Unknown rows have no measured bar. Partial remains visible in the header.
Keep 44-pixel phone/coarse-pointer targets and existing desktop density.
Example numbers, fill counts, and spacing are illustrative.

## Verification

Run from the repository root after Task 01. Workspace dependencies are already installed.
Write the focused unit/component and browser regressions before production changes.

```bash
(cd apps/web && pnpm test components/settings/system/storage/storage-overview-resources.test.ts components/settings/system/storage/storage-overview-card.test.tsx components/settings/system/storage/storage-totals.test.ts components/settings/system/storage/storage-disk-capacity-card.test.tsx)
(cd apps/web && pnpm exec eslint components/settings/system/storage)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/system/storage-analysis-bars.spec.ts tests/system/storage-maintenance.spec.ts tests/system/storage-temporary-folders.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-storage-analysis-bars.spec.ts tests/system/mobile-storage-maintenance.spec.ts tests/system/mobile-storage-temporary-folders.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Create `storage-overview-resources.test.ts` and the two bars E2E files named above.
Before the final checks, add all translations and generate the Traditional Chinese pair.
Use controlled snapshots with shuffled byte magnitudes, stable ties, zero, missing fields, and partial values.
Add a locale case where equal formatted sizes have different raw bytes.

Open two rows, focus one trigger, then replace the snapshot with changed magnitudes.
Assert both expanded identities survive and the focused resource remains focused.
Test first-scan source completion separately from stale-snapshot refresh.
Verify that sorting, expansion, and resizing issue no cleanup requests.

Check proportional fill geometry, text visibility, no new keyboard stops, and no horizontal overflow.
Verify the configured phone viewport and 767/768-pixel composition boundaries.
Capture and inspect desktop and phone screenshots through `prCapture`.

## Files likely touched

- `apps/web/components/settings/system/storage/storage-overview-resources.ts`
- `apps/web/components/settings/system/storage/storage-overview-resources.test.ts` (new)
- `apps/web/components/settings/system/storage/storage-overview-card.tsx`
- `apps/web/components/settings/system/storage/storage-overview-card.test.tsx`
- Optional local helper/component under the same directory, with its logic covered by the listed tests.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/system.json`
- `apps/web/e2e/helpers/storage-maintenance.ts`
- `apps/web/e2e/tests/system/storage-analysis-bars.spec.ts` (new)
- `apps/web/e2e/tests/system/mobile-storage-analysis-bars.spec.ts` (new)
- `docs/public/operations.md`
- This plan, work orders, and the paired draft design for final status/results.

## Dependencies

Task 01, because both tasks change resource projection, localization, component tests, and browser fixtures.
This dependency orders integration and preserves the completed timeout behavior.

## Risks

- Parsing formatted values gives incorrect order across locales and rounded sizes.
- Array-index keys can transfer expansion to the wrong resource after sorting.
- Some existing rows represent subsets or overlaps. Never recalculate Total counted from bar rows.
- Desktop sizing assumptions can clip long translated labels on phones.

## Parallelism

`sequential`.

## Inputs

- [Requirements](../../specs/system-page/requirements/storage-maintenance.md), section 005.
- [Design](../../specs/system-page/system-design/storage-analysis-presentation.md), numeric mapping and presentation sections.
- Existing `StorageOverviewResources`, `ResourceRow`, `storageResources`, and capacity-card styling.
- Existing Storage maintenance browser fixtures and completed Task 01.
- Scoped web `AGENTS.md`, plus TDD, E2E, mobile-parity, and docs-maintainer skills.

## Results

- `storage-overview-resources.test.ts`: passed numeric mapping, stable sorting, common scale, zero,
  missing, invalid, optional, and subset measurement cases.
- `storage-overview-card.test.tsx`: passed decorative bar, partial status, expansion, and focus
  assertions; the full focused Storage component suite passed.
- Desktop and mobile bars browser suites: passed descending order, proportional fills, unknown and
  zero states, refresh reordering, stable expansion and focus, touch target geometry, breakpoints,
  and horizontal containment.
- Public operations guidance now explains relative bars and unchanged counted totals.
