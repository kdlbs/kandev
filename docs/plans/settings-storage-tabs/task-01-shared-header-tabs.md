---
id: "01-shared-header-tabs"
title: "Shared header tabs and page destinations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-HEADER-TABS-001
  - REQ-UI-SETTINGS-HEADER-TABS-002
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
acceptance_criteria:
  - AC-UI-SETTINGS-HEADER-TABS-001.1
  - AC-UI-SETTINGS-HEADER-TABS-001.2
  - AC-UI-SETTINGS-HEADER-TABS-001.3
  - AC-UI-SETTINGS-HEADER-TABS-001.4
  - AC-UI-SETTINGS-HEADER-TABS-001.5
  - AC-UI-SETTINGS-HEADER-TABS-001.6
  - AC-UI-SETTINGS-HEADER-TABS-002.1
  - AC-UI-SETTINGS-HEADER-TABS-002.2
  - AC-UI-SETTINGS-HEADER-TABS-002.3
  - AC-UI-SETTINGS-HEADER-TABS-002.4
  - AC-UI-SETTINGS-HEADER-TABS-002.5
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.1
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.2
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.3
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.4
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.5
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 01: Shared header tabs and page destinations

## Summary

Deliver the shared header tabs through both real settings pages. Preserve drafts and route compatibility in the same vertical slice.

## In scope

- Extend the existing header and shells, add shared SettingsTabs and useSettingsTab, and adopt them on both pages.
- Move Office retention, update discovery and legacy redirects, and update the backend retention health fix URL.
- Mount each panel lazily on first activation, retain it after activation, hide inactive panels explicitly, and keep an active diagnostic bundle mounted while Logs is hidden.
- Add all changed locale keys and update route-dependent E2E selectors/helpers without removing existing action scenarios.
- Add the shared-component convention to apps/web/AGENTS.md.

## Out of scope

- New retention algorithms, provider eligibility, persistence, or API payloads.
- Unrelated settings-page migration, release flags, and task delegation.

## Acceptance

- Both pages use one shared header-tab implementation with correct desktop/phone placement and keyboard/touch operation.
- Direct links, defaults, invalid tabs, target fragments, repeated search, and history restore the correct visible panel.
- Dirty forms survive tab switches; save/discard, guarded page exit, save failure, and member permissions retain their behavior.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview).

### UI-01: Header tabs, desktop

Entry: Settings > Data & Logs or Settings > Storage. State: selected tab, loaded content.
Before: the header has no tabs; Data & Logs stacks Database, Office retention, compaction, backups, and Logs.
After:

```text
Data & Logs                              [ Database | Logs ]
Database statistics, backups, and server logs.
------------------------------------------------------------
[ Database status and actions                             ]
[ Message compaction: full content width                  ]
[ Backups                                                ]

Storage                           [ Host | Office retention ]
Manage disk usage and history retention.
------------------------------------------------------------
[ Content of the selected tab                            ]
```

Tabs belong inside the existing title/description header. There is no second content tab row.
Header and content use the existing page scroll owner; neither gains a new fixed position.
Maps to AC-UI-SETTINGS-HEADER-TABS-001.1/.3/.6 and AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.1/.2.

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

## Verification

Run from the repository root. New test paths below are implementation deliverables.
Use TDD for changed logic. Run focused failing tests before implementation, then the complete block after changes.
The E2E runner rebuilds current sources; do not pass `--no-build` for changed UI.

```bash
(cd apps/web && pnpm exec vitest run components/settings/settings-tabs.test.tsx components/settings/settings-typography.test.ts hooks/domains/settings/use-settings-tab.test.ts src/settings-route-helpers.test.ts src/settings-routes.test.ts lib/settings-discovery/catalog.test.ts lib/settings-discovery/navigation.test.ts lib/settings-discovery/target.test.ts)
(cd apps/backend && go test ./internal/office/retention ./internal/health)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/system/settings-header-tabs.spec.ts e2e/tests/system/database-page.spec.ts e2e/tests/system/backups-page.spec.ts e2e/tests/system/logs-page.spec.ts e2e/tests/system/retention-settings.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-settings-header-tabs.spec.ts e2e/tests/system/mobile-database-page.spec.ts e2e/tests/system/mobile-logs-bundle.spec.ts)
(cd apps/web && pnpm e2e:run --project auth e2e/tests/auth/system-data-storage-member-gating.spec.ts e2e/tests/auth/mobile-system-data-storage-member-gating.spec.ts)
(cd apps/web && pnpm exec eslint components/settings/settings-tabs.tsx components/settings/settings-tabs.test.tsx components/settings/settings-typography.tsx components/settings/system/system-page-shell.tsx components/settings/system/system-route-shell.tsx components/settings/system/data-logs-settings.tsx components/settings/system/storage-settings.tsx hooks/domains/settings/use-settings-tab.ts hooks/domains/settings/use-settings-tab.test.tsx src/settings-routes.tsx lib/settings-discovery/catalog/system.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- apps/web/components/settings/settings-typography.tsx and settings-tabs.tsx (new), with tests
- apps/web/components/settings/system/system-page-shell.tsx and system-route-shell.tsx
- apps/web/components/settings/system/data-logs-settings.tsx and storage-settings.tsx (new)
- apps/web/hooks/domains/settings/use-settings-tab.ts (new), with tests
- apps/web/src/settings-routes.tsx and src/settings-routes.test.ts
- apps/web/lib/settings-discovery/catalog/system.ts, navigation.ts, and affected discovery tests
- apps/web/components/settings/settings-target-provider.tsx and settings-target.tsx if activation coordination requires changes
- apps/backend/internal/office/retention/health.go and health_test.go
- apps/web/e2e/tests/system/settings-header-tabs.spec.ts and mobile-settings-header-tabs.spec.ts (new)
- Existing E2E files named in Verification and their shared route helpers
- apps/web/src/locales/*/system.json, settings.json, and apps/web/AGENTS.md

## Dependencies

None

## Risks

An inactive mounted panel can steal target focus or lose its save registration. A generic bypass can disable unrelated navigation guards.

## Parallelism

`sequential`

## Inputs

- [Header-tab requirements](../../specs/ui/requirements/settings-header-tabs.md) and [design](../../specs/ui/system-design/settings-header-tabs.md).
- [Data/storage requirements](../../specs/system-page/requirements/system-data-storage-pages.md) and [design](../../specs/system-page/system-design/system-data-storage-pages.md).
- Existing SystemPageShell, SettingsPageHeader, settings save tests, and SleepInhibitionInfoTooltip touch pattern.

## Results

Implemented shared `SettingsTabs` and `useSettingsTab` support, header placement,
same-path query selection, draft-preserving panel mounts, discovery target
activation, legacy redirects, and the Data & Logs and Storage page composition.
Updated the Office retention health destination and route-dependent E2E helpers.

Validation passed with the focused route/tab tests, focused Office retention and
health Go tests, frontend typecheck and lint, and the affected desktop, mobile,
and authenticated E2E suites.
