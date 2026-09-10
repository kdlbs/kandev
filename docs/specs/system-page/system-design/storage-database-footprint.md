---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002
created: 2026-09-10
owners:
  - kandev
---

# Database Storage Analysis System Design

## Purpose and boundaries

The system-page system owns storage analysis and the database maintenance surface.
Extend the existing analysis contract with local SQLite database and backup measurements.
This is a read-only extension of existing providers, with no schema or cleanup changes.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-002` | Measurement, Overview integration, Presentation, Verification |

## Measurement

Add a focused reader in `internal/system/storage/databasestore`.
Inject the effective database driver and resolved database path at backend composition.
Match `internal/system/system.go`: use the configured SQLite path, with the resolved
data-directory default only when no path is configured. Normalize relative paths once.
Derive the backup directory as the database file's sibling `backups` directory.
Never assume the database lives under the Kandev home.

Measure file lengths with filesystem metadata, consistent with existing file scan totals.
Do not invoke database Stats, PRAGMAs, VACUUM, checkpoint, backup creation, or content reads.
The Data page's logical database size can differ from this filesystem measurement.

Database bytes include the primary file and existing `-wal`, `-shm`, and
`-journal` sidecars. Missing optional sidecars contribute zero. A missing primary
file or an unreadable expected file makes this source unavailable.

Backup bytes include regular files recursively under the resolved backup directory,
including automatic snapshots, manual snapshots, and their sidecars. This measures the
directory footprint, so do not reuse a paginated or filename-filtered backup listing.
Use the shared four-partition `filescan.Limiter`; do not add an independent worker budget.
A missing backup directory is measured zero. Permission or traversal errors make the
backup source unavailable, independently of database measurement.

Resolve symlinks in configured parent directories once to establish canonical roots,
while preserving the final database or backup component so final-component symlinks
are rejected. Skip symlink entries inside the backup tree, with a warning; never
recursively follow them.
Deduplicate the primary file and its sidecars from the backup walk if the configured
filename causes overlap. Check overlap with the effective filesystem measurement
roots used by the current scan. The scanner retains the full row footprint and
reports the regular-file bytes under those roots separately. If all non-zero bytes
overlap, retain the informational row but exclude its bytes from the aggregate with
an explicit reason. If only part overlaps, retain the row, expose its independently
counted bytes, and identify the partial attribution. This extension does not repair
pre-existing cross-provider overlap or hard-link accounting across workspace roots.

The measurements are sampled file lengths, not allocated blocks or a transactional
snapshot. Files can change during analysis. Optional sidecars disappearing during
the scan count as absent; a backup walk failing during rotation is unavailable until refresh.
PostgreSQL and other non-file drivers report these local measurements as not applicable.
Do not query remote database sizes or scan a guessed local backup directory.

## Overview integration

Add `database` and `database_backups` to `storage.Summary` and
`storageAnalysisSources`. Each uses a typed measurement:

- `status`: `measured | unavailable | not_applicable`.
- `size_bytes`: present only for a complete measured result, including zero.
- `counted_size_bytes`: present for a measured result and contains the bytes this
  source contributes after effective-root overlap attribution.
- `path`: resolved location only when applicable and resolved successfully.
- `included_in_total`: true when the measured result contributes any bytes, including
  a partial-overlap result; false only for a non-zero result fully covered elsewhere.
- `reason`: stable code for unsupported driver, full overlap, or partial overlap;
  localized in the UI.
- `warning`: optional diagnostic for the authorized overview response.

Run both measurements as independent source operations in `storageOverview.summary`.
Extend `summaryFromMeasurements`, `summaryFromSourceValues`, and legacy progress
mapping in `OverviewCache`. Update fixed source counts and all fixtures.
Not-applicable sources finish successfully; unavailable sources publish failed
source progress but preserve sibling measurements.

Use the existing 15-minute snapshot cache, manual Analyze refresh, and first-scan
partial summary. Do not fetch Data-page statistics separately in the Storage component.
Existing revision handling and atomic successful snapshots remain authoritative.

Missing new fields from older responses are unknown measurements, not measured zero.
An unavailable or unknown applicable source sets the counted total to partial.
Not-applicable sources do not make an otherwise complete local total partial.
An overlapping measured source is visibly identified as already counted elsewhere;
partial overlap explains that only its distinct bytes contribute. During a pending
or scanning first read, a missing database measurement uses its source progress.

Keep authorization on the existing storage overview routes. Broadcast events contain
only generation and state; database paths and sizes remain in authorized HTTP responses.
Existing source-duration and outcome telemetry includes the two new identities.
No new database store, migration, setting, feature flag, or polling timer is needed.

## Presentation

Extend `StorageSummary`, `StorageSummaryPartial`, `storageResources`, and
`storageAnalysisTotal`. Add Database and Database backups after Task workspaces,
before Quarantined resources. Each row shows its size and expands to reveal its path
and measurement scope. The Database detail explains inclusion of SQLite sidecars.
The backups detail explains that it measures files in the backup directory.

Use existing `formatGigabytes` consistently: currently 1024 cubed bytes labeled GB.
Do not change the application-wide unit convention in this work.
Add both included measurements once to Total counted and Counted so far.
Render unavailable, pending, not-applicable, and already-counted states explicitly.
Add visible analysis copy explaining that classified storage does not cover all host usage.
Do not add a residual subtraction row: categories can span filesystems.

Desktop and mobile share the existing resource accordion and view-model logic.
The nearest mobile exemplar is the shipped Storage analysis accordion and
`mobile-storage-maintenance.spec.ts`. The page retains document scrolling.
On phones, users tap each row to inspect details; paths wrap and triggers retain
44-pixel touch targets. Fine-pointer density remains unchanged. No new overlay,
navigation, fixed control, or secondary scroll region is required.

All labels, explanations, and status reasons use i18next in en, pt-pt, zh-cn, zh-hk,
and zh-tw. Generate Traditional Chinese catalogs with the existing script.
Document the scope in `docs/public/operations.md` during implementation.

## Verification

Provider fixtures cover custom and relative paths, main file plus sidecars,
empty/missing backups, nested manual backups, unreadable files, cancellation,
symlinks, overlap, and PostgreSQL with no filesystem probes.
Overview tests prove both sources reach partial and completed responses, cache reuse,
refresh, terminal unavailable states, and absence of sensitive event payload fields.
Unit/component tests prove exactly-once addition and every row state.
Desktop and mobile-chrome E2E tests inspect both rows, long paths, total contribution,
and Analyze refresh after a disposable backup file changes. This is emulated mobile
coverage; physical-device validation is not available in this environment and remains
an explicit follow-up gate for closing mobile validation.

## Related decisions

- [Install-wide storage maintenance](../../../decisions/0045-install-wide-storage-maintenance.md)
- [Bounded progressive storage analysis](../../../decisions/2026-09-05-bounded-progressive-storage-analysis.md)
- [Backup location guidance](backup-location-actions.md)
