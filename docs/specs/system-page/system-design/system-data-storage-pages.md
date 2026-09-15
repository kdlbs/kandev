---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003
  - REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-004
---

# System Data and Storage Pages System Design

## Purpose and boundaries

System-page owns the composition and maintenance presentation of these two routes.
This revision describes the pending [settings storage tabs package](../../../plans/settings-storage-tabs/plan.md).
The existing two-page split is already implemented.
The [UI header-tab design](../../ui/system-design/settings-header-tabs.md) owns the reusable interaction.
Office owns retention policy, deletion eligibility, preview markers, and count semantics.
Those backend contracts remain unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-001 | Route composition and compatibility |
| REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-002 | Route composition and compatibility; State and permissions |
| REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-003 | Retention controls; Temporary files and compaction |
| REQ-SYSTEM-PAGE-DATA-STORAGE-PAGES-004 | Retention status |

## Route composition and compatibility

`src/settings-routes.tsx` retains both canonical paths.
`SystemRouteShell` and `SystemPageShell` pass an optional tabs slot to the existing `SettingsPageHeader`.
Each page owns one shared Tabs root around its header and content.
Use dedicated route compositions when the shared root must contain the shell itself.
Do not put a second page heading or tab strip inside the content.

| Path | Query | Content order |
| --- | --- | --- |
| `/settings/system/data-storage` | `tab=database` (default) | DatabaseStatsCard, ToolPayloadRetentionCard, BackupsTable |
| `/settings/system/data-storage` | `tab=logs` | LogViewer |
| `/settings/system/storage` | `tab=host` (default) | StorageMaintenanceSettings |
| `/settings/system/storage` | `tab=office-retention` | Retention status, retention policy |

`DataLogsSettings` becomes the Data & Logs tab composition.
A proposed `StorageSettings` composition wraps existing host maintenance and Office retention.
Settings menu labels and breadcrumb paths remain unchanged.
Storage description includes host cleanup and Office history retention.

In `lib/settings-discovery/catalog/system.ts`, move the retention entry to the Storage discovery parent.
Keep stable target IDs and add tab queries to all affected entries.
Backups and compaction resolve to Database, log results to Logs, and all existing host targets to Host.
A recognized target fragment selects its owning tab before focus/highlight.
A manually selected tab removes the old target fragment so it cannot override later navigation.

Legacy routes resolve as follows:

- `/settings/system/database` -> Data & Logs, Database.
- `/settings/system/backups` -> Data & Logs, Database, existing backups target.
- `/settings/system/logs` -> Data & Logs, Logs.
- Old Data & Logs retention fragments -> Storage, Office retention, same retention target.

Unrelated query parameters survive redirects. Legacy redirects use replacement navigation.
An unqualified Data & Logs link still opens Database because it cannot identify a historical retention intent.
Update `internal/office/retention/health.go` `fixURL` to
`/settings/system/storage?tab=office-retention` and update its focused tests.
Other backend behavior and health issue identities remain unchanged.

## State and permissions

The existing `SettingsSaveProvider` is keyed by pathname, not query.
Use the UI design's same-path tab replacement and retained stateful panels.
Keep contributors `system:storage-policy`, `system:retention`, and `system:tool-payload-retention` in their owning page.
Do not duplicate them or reset their dirty baselines on tab activation.
Visit panels lazily, then retain forms until route exit. Mount LogViewer only while selected to avoid hidden streaming work.
Existing ongoing maintenance/preparation jobs keep their current lifecycle when a form is hidden.

The floating Save changes control saves all dirty contributors on the page, including inactive panels.
Discard restores their authoritative baselines. Failed saves keep drafts and expose the existing error feedback.
Return to the affected panel without losing state. Cross-page changes retain the existing navigation guard.

Retain existing API role gates and read-only member surfaces.
Office retention remains admin-scoped even beside member-readable host information.
A failed Office request must not hide the Host tab or its permitted content.

## Retention controls

Refactor `retention-settings-card.tsx` into focused policy, status, and help components under the same system directory.
Keep `useRetentionSettings`, the settings wire types, and the save contributor contract.
Use stable technical IDs for test selectors and wire mapping, independently of translated display labels.

| Existing identity | Display label | Explanation |
| --- | --- | --- |
| `office_routine_runs` | Routine history | Routine activity, including skipped and coalesced executions |
| `runs` | Agent run history | Office agent runs |
| `run_events` | Run events | Timeline entries belonging to a run |
| `office_run_route_attempts` | Provider attempts | Provider/model routing attempts for a run |
| `office_run_skills` | Run skill records | Skill references associated with a run |

Use Minimum per routine for `routine_runs.floor_per_owner`.
Use Minimum per agent profile for `runs.floor_per_owner`.
Keep the enable switch, retention days, and minimums visible.
Place interval, batch limit, and all three warning thresholds in Advanced settings, initially collapsed.
Keep advanced fields in the draft while collapsed. Show validation errors outside the collapse and reveal the affected field.
The run-events threshold counts events only. Dependent records have no independent retention window or floor.

Remove the explicit `settingsControlClassName("h-11")` override from `NumberField`.
Use the shared sizing helper without a desktop height override.
Each numeric label has a focusable information button. Help covers meaning, limits, and zero-value behavior where supported.
Reuse the `SleepInhibitionInfoTooltip` interaction in `sleep-inhibition-settings.tsx`: Tooltip on fine pointers, Drawer via `useTouchDrawer` on touch.
The drawer has a label, close action, safe-area clearance, and focus return to its opener.
Keep a short visible statement: cleanup removes old finished history and related run details, while active work remains protected.
Helper icons contain secondary explanations; field labels and deletion consequences remain understandable without opening help.

## Retention status

Extract a pure status view-model helper and test it with the existing `RetentionStatus` shape.
The summary uses saved `status.settings`, never the unsaved draft.
The enabled badge means automatic deletion is configured; it does not imply a healthy last cleanup.
Do not show an invented next-run time or a persisted history. Last sweep data resets after server restart.

Render these sections in order:

1. Automatic cleanup enabled/disabled and the last recorded cleanup result/time.
2. Three stored-count summaries with translated labels, counts, and threshold states.
3. Expandable Cleanup details with per-category deletion/preview counts, provider attempts, run skill records, skips, and attribution.

Derive outcome from all reported table errors and both swept-table preview/backlog flags.
Any error produces a visible error outcome, while retaining committed counts from successful categories.
A preview in one category must not mask deletion in another. Show a mixed outcome when both occur.
Show backlog separately from errors and preview. Preview counts never count as deleted records.
A deletion total sums the five reported committed deletion counts and uses the unit records, never runs.

Preserve `not_computed`, fresh, and stale states independently per count.
A missing count shows unavailable, not zero. A stale count retains its last value and timestamp with a visible stale label.
Compare counts against saved thresholds using the backend's strict greater-than rule. Zero disables that threshold.
Stale values can say the last count exceeded a threshold but cannot claim current health.
Use one Updated timestamp only when all displayed measured counts have the same timestamp; otherwise keep per-count timestamps.
State that stored records include protected work and are not a deletion estimate.

Warnings, unknown-status notices, backlog, and errors remain visible when details collapse.
Details preserve unknown status values/counts, top routine ID/share, skip count/time, and per-category errors.
Raw diagnostic values can remain in details; normal titles and labels use product language.
No added network call, count scan, health metric, or history table is required.

## Temporary files and compaction

Rename the temporary-artifacts section to Temporary Kandev files through locale keys.
In `storage-policy-card.tsx`, explain registered diagnostic bundles and utility working folders.
Remove the repeated age sentence. Keep a concise visible stale-age/quarantine consequence next to the cleanup action.
Help explains that shared temporary folders and unrelated package caches are excluded from this provider.
Preserve the scheduled opt-in switch, manual action, disabled reasons, confirmations, and quarantine behavior.

Remove `max-w-3xl` from `ToolPayloadRetentionCard` and retain `min-w-0`.
The card fills the Database content row, while controls keep compact widths and wrap on phones.
Keep existing analysis, backup preparation, automatic compaction, manual compaction, and space-reuse explanations.

## Mobile design contract

The Settings index and full-height Settings surface remain the entry point and page scroll owner.
Header tabs move below the description. They remain inline because they choose the primary page content.
Phone retention status becomes a single-column list. Policy fields stack with labels, units, and 44px touch targets.
The primary settings action remains the existing floating Save changes control with safe-area clearance.
Help uses the shipped sleep-inhibition Drawer pattern; desktop uses hover/focus Tooltip.
No second mobile state model, hidden duplicate form, nested vertical scroll area, or new sidebar is introduced.

## Localization and documentation

All changed labels use `t()` or `Trans` with complete English, Portuguese, and Chinese catalogs.
Use `pnpm run i18n:zh-hant` for Traditional Chinese and verify the pseudo-locale.
Use locale-aware number/date formatting. Preserve internal IDs and wire keys.
Update `docs/public/operations.md` and applicable authentication navigation references when implementation ships.
Public docs remain current during this design-only change.

## Related decisions and contracts

- [Header tabs](../../../decisions/2026-09-15-settings-header-tabs.md)
- [Separate data/storage pages](../../../decisions/2026-09-03-separate-system-data-storage-pages.md)
- [Owned temporary files](../../../decisions/2026-08-08-owned-temp-artifact-cleanup.md)
- [Office retention operations](../../office/system-design/run-history-retention-operations.md)
- [Tool payload retention](tool-payload-retention.md)
