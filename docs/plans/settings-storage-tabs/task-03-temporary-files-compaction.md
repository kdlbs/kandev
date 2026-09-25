---
id: "03-temporary-files-compaction"
title: "Temporary file explanations and full-row compaction"
status: done
wave: 3
depends_on: ["02-retention-policy-status"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.2
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.5
  - AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.6
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
  - ../../specs/system-page/system-design/system-data-storage-pages.md
---

# Task 03: Temporary file explanations and full-row compaction

## Summary

Explain temporary Kandev files and give message compaction the full Database content row. Preserve both maintenance workflows.

## In scope

- Replace temporary-artifact terminology with concrete examples and remove repeated descriptions.
- Preserve the scheduled toggle, manual cleanup action, disabled reasons, confirmation, and quarantine behavior.
- Remove the message compaction width cap and keep responsive control wrapping.
- Update desktop/phone tests and translations in the same work order.

## Out of scope

- New retention algorithms, provider eligibility, persistence, or API payloads.
- Unrelated settings-page migration, release flags, and task delegation.

## Acceptance

- Temporary-file copy explains registered ownership, supported examples, inactivity, stale age, quarantine, and exclusions without repetition.
- Compaction fills the content row while analysis, backup preparation, saved activation, and manual action remain functional.
- Phone help uses a real Drawer with touch targets; both cards remain contained and usable on desktop and phone.

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

### UI-04: Temporary files and help

Entry: Storage > Host. State: cleanup available, automatic cleanup off.

```text
Temporary Kandev files
Diagnostic bundles and utility working folders.
Clean inactive temporary files (i)                    [OFF]
Inactive for at least 24 hours. Files move to quarantine.
[ Clean temporary files ]
```

Desktop help opens on hover or focus. Phone tap opens the shared inset Drawer:

```text
+--------------------------------+
| About temporary Kandev files    |
| Registered diagnostic bundles  |
| and utility working folders.   |
| Shared temporary folders and   |
| unrelated caches are excluded. |
|                        [Close] |
+--------------------------------+
```

The Drawer owns its temporary scroll area only when content exceeds available height.
Its close action returns focus to the information icon. Existing cleanup confirmation and disabled reasons remain intact.
Maps to AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-003.2/.5.

## Verification

Run from the repository root. New test paths below are implementation deliverables.
Use TDD for changed logic. Run focused failing tests before implementation, then the complete block after changes.
The E2E runner rebuilds current sources; do not pass `--no-build` for changed UI.

```bash
(cd apps/web && pnpm exec vitest run components/settings/system/storage/storage-policy-card.test.tsx components/settings/system/tool-payload-retention-card.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/system/storage-temporary-folders.spec.ts e2e/tests/system/tool-payload-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-storage-temporary-folders.spec.ts e2e/tests/system/mobile-tool-payload-retention.spec.ts)
(cd apps/web && pnpm exec eslint components/settings/system/storage/storage-policy-card.tsx components/settings/system/tool-payload-retention-card.tsx e2e/tests/system/*temporary-folders.spec.ts e2e/tests/system/*tool-payload-retention.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- apps/web/components/settings/system/storage/storage-policy-card.tsx and test
- apps/web/components/settings/system/tool-payload-retention-card.tsx and test
- Desktop/mobile temporary-folder and tool-payload E2E files named in Verification
- apps/web/src/locales/*/system.json

## Dependencies

02-retention-policy-status

## Risks

Renaming copy must not change provider IDs or broaden cleanup eligibility. Width checks must measure the card against its content parent.

## Parallelism

`sequential`

## Inputs

- [Header-tab requirements](../../specs/ui/requirements/settings-header-tabs.md) and [design](../../specs/ui/system-design/settings-header-tabs.md).
- [Data/storage requirements](../../specs/system-page/requirements/system-data-storage-pages.md) and [design](../../specs/system-page/system-design/system-data-storage-pages.md).
- Existing SystemPageShell, SettingsPageHeader, settings save tests, and SleepInhibitionInfoTooltip touch pattern.

## Results

Updated temporary-file copy to explain registered ownership, supported examples,
inactivity, quarantine, and exclusions. Removed the compaction card width cap
while preserving its analysis, backup, activation, and cleanup behavior. The
mobile help path uses the shared touch Drawer and existing cleanup controls keep
their permissions and confirmation behavior.

Validation passed with storage and compaction unit tests, desktop and mobile
temporary-folder and compaction E2E, the broader storage-maintenance suites,
frontend typecheck and lint, and i18n checks.
