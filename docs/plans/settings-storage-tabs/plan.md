---
created: 2026-09-15
status: completed
requirements:
  - REQ-UI-SETTINGS-HEADER-TABS-001
  - REQ-UI-SETTINGS-HEADER-TABS-002
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-004
system_design:
  - ../../specs/ui/system-design/settings-header-tabs.md
  - ../../specs/system-page/system-design/system-data-storage-pages.md
legacy_specs: []
---

# Implementation Plan: Settings Header Tabs and Storage UX

## Overview

Create reusable tabs inside the existing page header, then reorganize Data & Logs and Storage.
Improve retention controls, status, temporary-file explanations, and compaction width.
Implement sequentially: shared navigation, retention presentation, remaining cards, then public documentation.
The user explicitly selected right-aligned desktop tabs and phone tabs below the description.
Implementation is tracked by the work orders below without delegation.

## Scope

### In scope

- Shared SettingsTabs and optional tabs in the existing SettingsPageHeader.
- Data & Logs: Database and Logs. Storage: Host and Office retention.
- URL selection, legacy/search/health destinations, and preserved save lifecycles.
- Compact desktop controls, touch help drawers, translated product labels, and accurate retention status.
- Full-row message compaction and clear temporary-file copy.
- Focused desktop/phone tests and public maintenance instructions.

### Out of scope

- New retention policy, database migrations, cleanup eligibility, APIs, or background jobs.
- Persisted sweep history, predicted next cleanup, or a new manual Office sweep action.
- Migration of unrelated settings pages, global tab restyling, or new runtime flags.
- Product screenshot recapture as a separate media project. Existing stale screenshots can be removed or relabeled without claiming a new capture.

## Technical approach

Use the [UI design](../../specs/ui/system-design/settings-header-tabs.md) for shared controls and state lifetime.
Use the [system-page design](../../specs/system-page/system-design/system-data-storage-pages.md) for domain composition and status derivation.
The [ADR](../../decisions/2026-09-15-settings-header-tabs.md) records the user-selected header placement and replaces the old no-tabs constraint.

Existing SettingsPageHeader already has title, description, and actions. Extend it instead of adding another header.
SystemRouteShell/SystemPageShell expose the optional tabs slot. Each consuming page owns one Tabs root.
Use the existing router and query subscriptions. Tab clicks replace the current history entry and retain forms.
Guard bypass applies only to validated same-path tab replacements. Page exit still uses the save coordinator.
Keep one target registration per discovery ID. Activate a hidden target's tab before focus/highlight.
Office health fix links change, but health identifiers and retention wire contracts do not.

## ASCII UI preview

Structure is required; spacing, example values, and exact copy are illustrative.
All text inside UI code must use localization. Examples below use ASCII `(i)` for the information icon.

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

## Tests

Each named new test is a deliverable, not an existing suite. Test names below define the expected cases.

| Criteria | Evidence |
| --- | --- |
| UI -001.1-.6 | `settings-tabs.test.tsx`: shared composition, manual keyboard activation, panel associations, no-tabs compatibility; rendered header tests |
| UI -002.1/.4/.5; system -002.4 | `use-settings-tab.test.tsx`: default/invalid query, target precedence, repeated target, query preservation, history destination; route/discovery tests |
| UI -002.2/.3; system -001.9/-002.5 | `settings-save-provider.test.tsx` plus tab integration: retained dirty contributors, save failure, discard and guarded exit |
| system -002.1-.3 | `settings-routes.test.ts`: correct tab composition and legacy mapping |
| system -003.1-.4 | `retention-settings-card.test.tsx`: labels, full saved payload, advanced fields and help |
| system -004.1-.6 | `retention-status.test.ts`: absent sweep, saved-enabled state, zero/unavailable/stale, differing timestamps, threshold equality/zero, mixed preview/deletion, partial errors, backlog, skip and attribution |
| system -003.5/.6 | Existing storage-policy and tool-payload card suites: retained actions and copy semantics |
| system -002.4 | Go retention/health tests: new Office retention FixURL |

## E2E tests

| Flow | File and project | Criteria |
| --- | --- | --- |
| Header placement, long labels, keyboard selection, URL/reload/history, dirty tab round-trip, guarded exit, search reveal | `system/settings-header-tabs.spec.ts`, chromium | UI -001/-002; system -002 |
| Phone header, 767/768 boundary checks, coarse-pointer target checks, tab strip overflow and focus | `system/mobile-settings-header-tabs.spec.ts`, mobile-chrome | UI -001; system -001.7/.8 |
| Retention edit/save/reload, advanced help, saved-policy status and mixed/error fixtures | `system/retention-settings.spec.ts`, chromium | system -003.1-.4/-004 |
| Retention touch help Drawer, close/focus return, single-column summary, controls >=44px | `system/mobile-retention-settings.spec.ts`, mobile-chrome | system -003.2/.3/-004.6 |
| Existing temporary cleanup and compaction workflows, full card width, help | Existing temporary-folders and tool-payload specs, chromium/mobile-chrome | system -003.5/.6 |
| Existing database/backups/logs navigation and actions | Existing system specs in task 01 | system -001/-002 |
| Member read-only host/database and denied Office mutations after tab changes | Existing data-storage member-gating specs, auth | system -002.5 |

Use API fixtures for saved data and controlled response fixtures for rare status combinations.
Assertions operate through rendered UI. Restore modified settings after each test.
Measure desktop input height at 28px within 1px and touch targets at least 44px.
Measure header title/tab alignment and card width against the actual parent, not visibility alone.
Capture focused rendered desktop/phone evidence in each UI work order. Check pseudo-locale and long translated labels.

## Work orders

- [x] [Task 01: Shared header tabs and page destinations](task-01-shared-header-tabs.md)
- [x] [Task 02: Plain-language retention policy and status](task-02-retention-policy-status.md)
- [x] [Task 03: Temporary file explanations and full-row compaction](task-03-temporary-files-compaction.md)
- [x] [Task 04: Document the settings destinations](task-04-operations-documentation.md)

## Verification setup

From the repository root, install dependencies once before implementation checks:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Each work order lists exact targeted commands. Run E2E commands sequentially without worker overrides.
The managed runner builds current sources. Use the configured chromium, mobile-chrome, and auth projects.
After locale edits, run `(cd apps/web && pnpm run i18n:zh-hant)` before the task's i18n checks.

## Results

Implemented the four work orders in sequence. The two system pages now use shared
header tabs with URL-addressable selection, preserved drafts, target activation,
and legacy redirects. Office retention has the revised policy and status
presentation, host maintenance retains its existing operations, and Database
contains message compaction and backups.

Validation passed for focused Vitest coverage (16 files, 167 tests), frontend
typecheck and lint, i18n checks and ratchet, public documentation and
specification validation, gofmt, focused Office retention and health tests, and
the affected desktop, mobile, and authenticated E2E suites. The full backend
matrix still reports unrelated failures in existing process probe timing,
ambient config discovery, launcher setup, and an Office FTS migration test;
the changed retention backend packages pass independently.
New components/hooks must meet scoped lint limits. Run `pnpm exec eslint` from apps/web on every changed TS/TSX source and test path.
No generic review, QA, simplify, or broad verification work order is added.

## Companion packages

The page-split, Office run-retention, tool-payload-retention, and storage-temp-artifacts packages are historical inputs.
Their completed results remain historical. This package owns all new route and presentation verification.
Their manifests link here so future work can find the changed allocation.

## Verification results

Implementation checks: in progress. Task 01 is in progress; Tasks 02-04 remain pending.
Design-package checks passed:

- `python3 scripts/list-docs.py validate`: 273 decisions and 933 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs`: passed.
- Package link, REQ/AC identity, and design-path audit: passed.
- `git status --short -- docs/plans/settings-storage-tabs`: confirmed the new manifest and work-order directory.

These results validate the design package only. No production code or permanent tests changed.

## Risks

- Query updates can unregister dirty forms or bypass page-exit protection if mounted state is not preserved.
- Hidden panels can consume discovery focus before their tab opens.
- A mixed preview/deletion sweep or stale count can produce a misleading summary if flattened into one success badge.
- Raw technical IDs can leak through errors; diagnostics remain available, but normal labels must be translated.
- Retention is admin-scoped even though Host contains member-readable information.
- Old screenshots and links can describe the previous layout until the documentation work order ships.
