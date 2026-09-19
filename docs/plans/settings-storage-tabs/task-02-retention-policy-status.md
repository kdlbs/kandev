---
id: "02-retention-policy-status"
title: "Plain-language retention policy and status"
status: done
wave: 2
depends_on: ["01-shared-header-tabs"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-004
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.1
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.2
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.3
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.1
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.2
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.3
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.5
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.6
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 02: Plain-language retention policy and status

## Summary

Deliver the retention policy and status redesign on the Office retention tab. Preserve the existing backend semantics.

## In scope

- Split policy/status presentation into focused components and extract a pure status view-model helper.
- Replace schema labels, correct owner labels, move secondary help into accessible Tooltip/Drawer controls, and restore shared input sizing.
- Group tuning controls in Advanced settings and retain validation visibility.
- Show saved policy state, measured counts, committed deletion totals, and accurate preview/backlog/error states.
- Translate every changed label and preserve stable technical test IDs.

## Out of scope

- New retention algorithms, provider eligibility, persistence, or API payloads.
- Unrelated settings-page migration, release flags, and task delegation.

## Acceptance

- Policy controls retain settings values and save behavior, with correct labels, sizing, help, and advanced-field access.
- Status distinguishes unavailable, zero, stale, preview, mixed deletion/preview, backlog, and error data without invented metrics.
- Desktop and phone support the same editing and diagnostic tasks, including drawer dismissal, focus return, and saved-policy status during unsaved edits.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview).

### UI-02: Header tabs, phone

Entry: the mobile Settings navigation. State: Storage, Office retention selected.

```text
Storage
Manage disk usage and history
retention.

[ Host | Office retention ]
------------------------------
Retention status
Automatic cleanup          On
Last cleanup
Today, 09:00          Completed

Routine history         1,240
Agent run history         860
Run events             12,400

> Cleanup details
------------------------------
Retention policy
Automatically delete old
history                  [ON]
Active work remains protected.

Routine history
Keep for                  (i)
[ 30                    ] days
Minimum per routine       (i)
[ 50                 ] records

> Advanced settings

          [ Save changes ]
```

Tabs move below the description inside the header. Counts and fields form a single column.
The existing floating save control appears only for dirty settings and clears the safe area.
Maps to AC-UI-SETTINGS-HEADER-TABS-001.2/.5 and AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-004.6.

### UI-03: Office retention, desktop and status variants

Entry: Storage > Office retention. State: loaded, saved policy.

```text
Storage                           [ Host | Office retention ]
Manage disk usage and history retention.
------------------------------------------------------------
Retention status                       Automatic cleanup: On
Last cleanup                  Today, 09:00 - Completed
Routine history       Agent run history       Run events
1,240 records         860 records             12,400 records
Updated today, 09:00
Stored counts include protected work.
> Cleanup details
------------------------------------------------------------
Retention policy                         Automatic cleanup [ON]
Old finished history and related details are deleted.
Active work remains protected.

Routine history
Keep for (i) [30] days       Minimum per routine (i) [50]
Agent run history
Keep for (i) [30] days       Minimum per agent profile (i) [50]

v Advanced settings
Cleanup interval (i) [6] hours      Batch limit (i) [5000]
Warn above (i)
Routine records [25000]  Agent run records [25000]
Run events [250000]
```

Changed states:

```text
Loading:    [spinner] Loading retention settings
Load error: Unable to load retention settings. [existing retry path]
No result:  No cleanup recorded since server startup.
Unavailable count: Routine history: Not available
Stale count:       Run events: 12,400 (out of date, timestamp)
Preview:    Routine history: Preview, 120 eligible, 0 deleted
Mixed:      Routine history: Preview | Agent run history: 40 deleted
Error:      Cleanup completed with errors. 40 records deleted.
Backlog:    More eligible history remains for a later cleanup.
Disabled:   Automatic cleanup: Off | Stored counts remain visible
```

The retry path means the existing hook/page reload behavior; it does not require a new API.
Error/backlog/stale notices remain outside collapsed details. Preview totals never count as deletions.
Use saved settings for status while a policy draft is dirty.
Maps to AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.1-.4 and -004.1-.6.

## Verification

Run from the repository root. New test paths below are implementation deliverables.
Use TDD for changed logic. Run focused failing tests before implementation, then the complete block after changes.
The E2E runner rebuilds current sources; do not pass `--no-build` for changed UI.

```bash
(cd apps/web && pnpm exec vitest run components/settings/system/retention-settings-card.test.tsx lib/utils/retention-status.test.ts)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/system/retention-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-retention-settings.spec.ts)
(cd apps/web && pnpm exec eslint components/settings/system/retention-*.tsx lib/utils/retention-status.ts lib/utils/retention-status.test.ts e2e/tests/system/retention-settings.spec.ts e2e/tests/system/mobile-retention-settings.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- apps/web/components/settings/system/retention-settings-card.tsx and test
- Focused retention policy, status, and help components in the same directory (new)
- apps/web/lib/utils/retention-status.ts and retention-status.test.ts (new)
- apps/web/e2e/tests/system/retention-settings.spec.ts and mobile-retention-settings.spec.ts (new)
- apps/web/src/locales/*/system.json

## Dependencies

01-shared-header-tabs

## Risks

Preview is per category. Stale counts and unsaved thresholds must not produce a false healthy status. Hidden invalid fields must remain actionable.

## Parallelism

`sequential`

## Inputs

- [Header-tab requirements](../../specs/ui/requirements/settings-header-tabs.md) and [design](../../specs/ui/system-design/settings-header-tabs.md).
- [Data/storage requirements](../../specs/system-page/requirements/system-data-storage-pages.md) and [design](../../specs/system-page/system-design/system-data-storage-pages.md).
- Existing SystemPageShell, SettingsPageHeader, settings save tests, and SleepInhibitionInfoTooltip touch pattern.

## Results

Implemented the plain-language Office retention policy, saved-policy status
view-model, friendly scope labels, advanced tuning disclosure, desktop help
tooltips, mobile help Drawer, compact desktop controls, and status handling for
disabled, unavailable, zero, stale, preview, deletion, backlog, and error
states. Draft edits continue to use the existing save coordinator.

Validation passed with retention unit tests, the focused 16-file Vitest run,
desktop retention E2E, mobile retention E2E, frontend typecheck and lint, and
i18n checks.
