---
id: "03-settings-surface"
title: "Broaden the existing retention card copy"
status: done
wave: 2
depends_on: ["01-message-metadata-target", "02-cleanup-snapshot-target"]
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-005
  - REQ-SYSTEM-PAGE-DB-HISTORY-RETENTION-006
acceptance_criteria:
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.1
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-005.2
  - AC-SYSTEM-PAGE-DB-HISTORY-RETENTION-006.2
system_design:
  - ../../specs/system-page/system-design/db-history-retention.md
---

# Task 03: Broaden the existing retention card copy

## Summary

The Data & Logs page already mounts `ArchivedDataRetentionCard` for the
`archived_data_retention` policy. This work order broadens its copy so the
message window reads as covering finished or archived tasks at a 14-day initial
value, and updates the localized strings. It adds no second card, record, route,
or navigation.

## Scope

- Update `ArchivedDataRetentionCard` and its fields/results components
  (`archived-data-retention-card.tsx`, `-fields.tsx`, `-results.tsx`) so the
  message window names finished or archived tasks and the copy states the
  stored-byte reduction, not a file-size reduction, with a pointer to the
  separate compaction action.
- Confirm the card still ships disabled by default, still withholds deletion
  until an explicit backup or skip choice is prepared, and still shows per-target
  analysis counts and bytes.
- Localized strings in every catalog (en, pt-pt, zh-cn, zh-hk, zh-tw through the
  hant conversion script, ja, ko), using `t()` for all copy.

## Exclusions

- No new settings record, HTTP route, revision stream, or settings card.
- No change to the tool payload card.
- No new route, navigation, or page.
- No batch limits, SQL paths, or internal file names exposed in the UI.
- No session-metadata, catalog-revision, or plan-revision controls; those designs
  are not implemented in this package.

## Acceptance conditions

1. The card describes the message window as covering finished or archived tasks
   and keeps the disabled-by-default, disable-until-prepared behavior.
2. Analyze reports per-target counts and bytes while the policy stays disabled,
   and a partial result never appears complete.
3. All new or changed copy is localized in every catalog.

## Likely files

- `apps/web/components/settings/system/archived-data-retention-{card,fields,results}.tsx`
- `apps/web/src/locales/*/system.json` (and the hant conversion pair)
- New `*.test.ts` / `*.test.tsx` next to the changed modules

## Verification

```bash
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
cd apps/web && pnpm run vitest run components/settings/system
```

## Results

Implemented as copy only, reusing `ArchivedDataRetentionCard`. Updated
`archivedData.title/description/archivedAge/archivedAgeHelp/targetMessages/window/unsupported`
and `archivedData.skipReason.protected_tasks` in `en`, `pt-pt`, `zh-cn`, `ja`,
`ko`, and regenerated `zh-hk`/`zh-tw` with `pnpm run i18n:zh-hant`. No component
or API change. `pnpm run i18n:check`, `typecheck`, `lint`, and
`vitest run components/settings/system` pass.
