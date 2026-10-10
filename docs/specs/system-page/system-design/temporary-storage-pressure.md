---
status: draft
system: system-page
created: 2026-09-28
owners:
  - kandev
requirements:
  - REQ-SYSTEM-PAGE-TEMP-PRESSURE-001
  - REQ-SYSTEM-PAGE-TEMP-PRESSURE-002
---

# Temporary storage pressure design

## Boundaries and current behavior

System-page owns filesystem capacity and temporary-footprint presentation.
The [existing temporary design](storage-temporary-folders.md) owns safe root resolution, traversal, and registered cleanup.
This draft adds pressure and explanation. It does not extend deletion authority.

`storage.HandlerConfig.DiskPath` currently receives `cfg.ResolvedHomeDir()` in `internal/backendapp/storage_maintenance.go`.
`GET /api/v1/system/storage/disk` therefore measures only the Kandev home filesystem.
`tempstore.Provider` separately measures temporary folder sizes through the cached storage analysis.
The two views can report a healthy home filesystem and a full, separate temporary filesystem without exposing that difference.

Host-local tools retain inherited `TMPDIR`, `TMP`, and `TEMP` behavior.
The working directory controls relative paths. It does not constrain absolute paths or a tool's temporary-directory API.
Go build scratch, browser profiles, transform caches, archives, and test environments can therefore occupy shared temporary storage.
Paths alone do not identify the process or task that created historical data.

## Requirement mapping

| Requirement                         | Sections                                                      |
| ----------------------------------- | ------------------------------------------------------------- |
| `REQ-SYSTEM-PAGE-TEMP-PRESSURE-001` | Capacity contract, Refresh and failure behavior, Presentation |
| `REQ-SYSTEM-PAGE-TEMP-PRESSURE-002` | Breakdown contract, Cleanup boundary, Presentation            |

## Capacity contract

Extend the existing disk response additively. Preserve its home-capacity fields and their meanings.
Add an optional `temporary_roots` array and a home `observed_at` timestamp.
Each temporary result contains the existing capacity fields plus requested path, aliases, observation time, and sharing information.
`shared_with_home` is nullable: true, false, or unknown when filesystem identity cannot be proved.
Known aliases share one result. Never sum filesystem capacities across displayed paths.

Backend composition supplies server-selected candidates through the same resolver used by `tempstore`.
Preserve the disposable `KANDEV_E2E_SYSTEM_TEMP_ROOT` override in fixtures.
Resolve roots without recursively enumerating entries. Clients cannot submit a path.
Use `system/metrics.DiskUsage` and its bounded platform calls.
Add native filesystem identity adapters only where the existing mount readers cannot establish shared capacity.
Mount traversal identity and capacity sharing identity are different: bind mounts can share capacity.
If sharing identity is unavailable, keep the measurement and show the relationship as unknown.

`available_bytes` uses the existing platform-available value, including reserved-space restrictions on Unix.
The existing `used_bytes = total_bytes - available_bytes` semantics remain compatible.
UI copy describes capacity in use or unavailable, rather than equating it with file-byte sums.
Reuse warning boundaries of 80% and 90% from `storage-disk-capacity-card.tsx`.
Do not change the existing host metrics or topbar contracts.

Each candidate has its own timeout and result. A home error does not suppress temporary results.
Retain the process-wide bound on potentially blocked platform calls.
If a blocked call exhausts another candidate's budget, that candidate becomes unavailable without unbounded retry.
At most three canonical candidates are inspected: home, effective temp, and Unix `/tmp`.
Windows supplies only its effective temp candidate.

## Refresh and failure behavior

Extend `useStorageMaintenance`, `StorageDiskCapacityResponse`, and the existing system store disk field.
Keep one disk request per refresh and preserve the existing section generation guard.
Fetch on entry, every 30 seconds while Host is visible, on visibility return, on Analyze, and after terminal cleanup.
Coalesce overlapping reads. Stop the timer on unmount, hidden documents, or the Office retention tab.
Do not add periodic recursive scans or another state store.

Capacity timestamps are independent from the cached analysis timestamp.
Keep the last successful per-root capacity during a transient failure, visibly marked stale with the failure.
A root removed by a successful authoritative response is removed from the displayed set.
Scope retained values to backend and auth identity. Never carry a prior server's paths into a new connection.
Missing extension fields from an older backend produce an unavailable temporary-capacity state.
Invalid totals and non-finite frontend numbers also produce unavailable, never a green zero bar.

## Breakdown contract

Extend `tempstore.RootMeasurement` with an optional `breakdown` object that contains `entries`.
Each entry has a direct-child name, kind, observed `size_bytes`, completeness, and ownership classification.
Return at most twenty entries, sorted by descending observed bytes and then name for stable ties.
Also return `other_observed_bytes`, `other_observed_count`, and breakdown completeness.
Do not report unvisited bytes as part of the known remainder.
Missing breakdown fields in an older cached response mean unavailable until the next Analyze.

Reuse the existing `filescan.Limiter` traversal and direct-child partition boundaries.
Add an opt-in child-summary result to `filescan.MeasureOptions` and `filescan.Result`.
Keep default callers unchanged. Reduce partition measurements into the top twenty and the observed remainder.
Do not scan once for totals and again for the list.
Use the current 60-second temporary-source deadline, cancellation, warning bound, and four shared scan slots.
Preserve symlink, socket, special-file, root-replacement, and nested-mount exclusions.
Partial children remain visible with partial labels. Unknown children do not acquire zero values.

Sizes remain apparent regular-file bytes, consistent with the existing footprint contract.
They are a changing sample, not allocated-block accounting or guaranteed recoverable bytes.
Hard links, sparse files, filesystem metadata, reserved blocks, and deleted open files can prevent reconciliation with capacity.
Do not subtract the apparent-file sum from physical filesystem usage to invent an Other disk usage number.

At backend composition, join exact child paths against the current installation's artifact registry.
Use existing marker validation before assigning `registered_kandev`.
Otherwise show `untracked`, or `unknown` when registry access or validation fails.
Registered ownership is not a cleanup eligibility decision. Do not expose marker tokens.
Do not infer a tool, task, or active process from a directory prefix.
The list itself answers which paths are large. Generic explanatory copy names possible types of temporary data.

## Cleanup boundary

Review cleanup is navigation to the existing Kandev temporary-artifact row.
Expand that row, move focus to its heading, and retain its existing confirmation and action handler.
Do not add a `system_temporary` mutation resource or pass a selected entry path to cleanup.
Explain that existing cleanup is installation-wide, and not limited to the selected temporary root.
Show the current stale count and candidate bytes when available, without promising that these bytes will become free space.
Busy, unavailable, unauthorized, and no-candidate states retain visible reasons.

Quarantine results remain separate from measured capacity changes.
Same-filesystem quarantine does not free filesystem space. Cross-filesystem quarantine needs destination capacity and retains a recoverable copy.
Refresh capacity after cleanup instead of predicting reclaimed space from folder sizes.
Standard tool-cache cleanup remains separate future work. It must discover effective standard locations and use tool-specific cleanup semantics.
This package changes no existing cache configuration or cleanup settings.

## Presentation and mobile contract

Entry point: Settings > System > Storage > Host.
Place temporary capacity next to the existing home capacity, before recursive analysis.
Warnings remain visible without opening the footprint accordion.
Use a View largest entries action that expands the existing System temporary folders row and focuses its heading.
Expansion displays cached data and never starts a new scan. Analyze remains the explicit rescan action.

Desktop uses aligned entry-name, size, and ownership columns inside the expanded root.
Phone uses a vertical list with name first, then size, completeness, and ownership.
The closest live exemplars are `storage-disk-capacity-card.tsx` and `storage-overview-card.tsx`.
Their inline cards and accordion fit this inspection flow. A separate drawer or route adds no value here.
Reuse the mobile UI language's single focused flow and explicit touch actions.
The page remains the only scroll owner. Long names wrap, and no footer or overlay changes safe-area behavior.
Keep normal desktop actions at 28 pixels and phone/coarse-pointer targets at least 44 pixels.
Share sorting, measurements, focus targets, and cleanup handlers across both compositions.

Extract focused components under `components/settings/system/storage/` rather than growing the existing overview component.
All new copy uses the `system` namespace in six locales. Generate Traditional Chinese with `i18n:zh-hant`.
Use text labels and timestamps alongside color. Preserve keyboard expansion and focus across sorting and refresh.
Announce manual refresh completion through the existing status region. Background polling must not repeatedly announce unchanged warnings.

## Persistence, permissions, and verification

No schema, settings, environment, or runtime-flag change is required.
Capacity is ephemeral. Breakdown belongs to the existing versioned overview cache and partial-summary projection.
Reuse authenticated reads and admin-only Analyze and mutation routes.
Do not expose new path-bearing WebSocket events or path-valued metric labels.

Backend tests inject capacity readers, mount identities, registry records, and disposable trees.
Browser tests use isolated backend roots and mocked capacity values to reproduce a healthy home and full temp filesystem.
No test fills a real disk or deletes shared temporary data.
The [work orders](../../../plans/temporary-storage-pressure/plan.md) contain commands and desktop/phone scenarios.

## Related decisions

- [Temporary visibility and ownership](../../../decisions/2026-09-11-temporary-storage-visibility-policy.md).
- [Owned temporary artifacts](../../../decisions/2026-08-08-owned-temp-artifact-cleanup.md).

These accepted decisions remain unchanged. This proposal adds visibility and navigation within their ownership boundary.
