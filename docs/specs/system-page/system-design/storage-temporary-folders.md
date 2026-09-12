---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-004
created: 2026-09-11
owners:
  - kandev
---

# Temporary storage visibility and cleanup

## Purpose and status

System-page owns install-wide storage inventory, maintenance settings, and quarantine.
This extension adds a broad read-only footprint and an optional policy for existing owned artifacts.
It does not turn discovered paths into cleanup candidates.

This design is implemented. System temporary folders are measured read-only, and the general
footprint is informational. Registered-artifact cleanup remains disabled by default; an enabled
saved policy permits scheduled and full manual maintenance to quarantine eligible artifacts.
The [policy decision](../../../decisions/2026-09-11-temporary-storage-visibility-policy.md)
records the amendment to the existing ownership decision.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003` | Read-only reader, Overview contract, Presentation |
| `REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-004` | Cleanup policy, Mutation safety, Presentation |

## Read-only reader

Add `internal/system/storage/tempstore`, separate from `tempartifacts`.
Backend composition supplies `os.TempDir()` and the Unix `/tmp` candidate.
Windows supplies only its effective service temporary folder.
No HTTP parameter, environment dump, or user-supplied path enters this reader.

Resolve configured root aliases once. Preserve requested and resolved paths for display.
Deduplicate canonical roots and collapse nested candidates into one scan root.
Reject a filesystem root as an effective temporary folder with `unsafe_root`.
Unreadable roots remain unavailable. A missing optional Unix `/tmp` is not applicable.
A missing effective temporary root is unavailable.

Use file lengths, matching current storage measurements. Do not read contents.
Reuse the process-wide four-partition `filescan.Limiter` from backend composition.
Extend its read-only options for tolerant per-entry errors and mount exclusion.
Existing strict callers retain their behavior. Do not create another worker pool.

Read only regular-file metadata. Skip symlinks, sockets, devices, and named pipes.
Reject root replacement after resolution. Never traverse a nested symlink target.
Skip nested mounts, including bind mounts, using platform mount identity.
Device identity alone cannot identify same-device bind mounts.
When mount identity cannot be established, mark the affected root unavailable.
The Linux reader uses fresh process mount-table snapshots. It refreshes at directory and partition
boundaries, reuses the boundary snapshot for file entries, and revalidates the root mount identity
before each partition. Other supported platforms use their native mount information.
Keep these platform readers behind injected interfaces for deterministic tests.

Apply a 60-second deadline to this source, within the existing overview deadline.
Check cancellation between entries and partitions. Bound diagnostic examples to ten paths per root.
Retain counts for all omitted, unreadable, or disappearing entries.
Permission failures, disappearance, mount exclusions, and deadline exhaustion produce partial measurements.
A partial size means successfully sampled bytes only. It is never a final folder total.
File lengths remain a changing sample, not allocated blocks or a transactional snapshot.

## Overview contract

Add the proposed source identity `system_temporary` to `storageAnalysisSources`.
Extend `Summary`, `summaryFromMeasurements`, `summaryFromSourceValues`, and cache compatibility mapping.
Add this optional field to the frontend `StorageSummary` and partial summary types.
Do not change the existing `temporary_artifacts` identity or its cleanup resource name.

The new result contains:

| Field | Meaning |
| --- | --- |
| `status` | `measured`, `partial`, `unavailable`, or `not_applicable` |
| `roots` | Deduplicated root results with paths, status, and optional `size_bytes` |
| `size_bytes` | Combined sampled bytes, present for measured or partial results |
| `included_in_total` | Always false for this informational footprint |
| `reason` | Stable reason, including `informational_overlap`, `unsafe_root`, or `deadline` |
| `warnings` | Bounded authorized diagnostics and complete omission counts |

The footprint can overlap workspaces, database files, caches, quarantine, and registered artifacts.
Therefore, it remains outside `storageAnalysisTotal` by construction.
Its label and visible explanation distinguish it from the classified total.
This preserves existing measured-file attribution without inventing a host-wide ownership hierarchy.
Failure of this informational source does not change completeness of the classified total.
It still marks its own source and the scan outcome as partial or failed.

Publish progress through existing scan revisions and coarse notifications.
Keep paths and measurements in authorized HTTP responses. Events carry no new filesystem data.
Source failure preserves successful sibling results and the last successful snapshot.
Missing fields from an older response remain unknown, never measured zero.
Existing refresh coalescing, stale-result rejection, and 15-minute cache timing remain authoritative.

## Cleanup policy

Add `TemporaryArtifacts ResourceSettings` with JSON key `temporary_artifacts`
to `StorageMaintenanceSettings`. The nested `enabled` field defaults to false.
Use the existing settings document, normalization, patch, and save boundaries.
No storage table migration, public environment variable, or release flag is required.
Old documents normalize the absent field to disabled. Updates preserve unrelated saved settings.

Inject the existing settings reader into `tempartifacts.ProviderConfig`.
`Provider.Cleanup` reads the saved policy and returns a disabled no-op unless enabled.
Enabled cleanup and `CleanupExplicit` share one eligibility and quarantine path.
Explicit cleanup bypasses only this option. It retains every activity and safety check.
The existing global schedule, idle gate, admission control, cancellation, and runner serialization apply.
Policy changes take effect on subsequent runs without restart.

The runner passes the captured maintenance settings snapshot to the provider. Use that snapshot for
the cleanup option and quarantine retention for the whole run.
Do not retain the provider constructor's default when the operator saved another retention value.
Keep the fixed 24-hour stale interval. Do not add an age editor in this package.

## Mutation safety

Reconcile registry ownership before candidate selection on each cleanup run.
Immediately before mutation, reload the exact artifact row and validate lifecycle timestamps and marker identity.
Reject active artifacts, uncertain liveness, missing rows, replaced paths, and mismatched markers.
Serialize lifecycle transitions with mutation through the existing maintenance/ownership boundaries.
When a producer can still mutate a candidate, protect it rather than infer inactivity from file age.
The startup-only reconciliation is insufficient for newly abandoned owners during a long service run.

Keep same-filesystem rename into `<KANDEV_HOME_DIR>/trash/temporary-artifacts/`. When the source
and quarantine are on different filesystems, stage a copy under the quarantine parent, copy only
real directories and regular files, sync the copied data, revalidate the source identity, and
atomically publish the staging directory. Remove the original only after publication and another
identity check. Any copy, publication, or identity failure leaves the original intact and records a
failed quarantine intent.
Bind the validated filesystem identity to each rename or deletion so a replacement path cannot be
accepted because it has the same marker.
Retain restart reconciliation, failed-intent retry, restore, and permanent deletion behavior.
The new schedule cannot bypass registry validation through `Run anyway`.

Display quarantined bytes as moved bytes. They are not immediately freed disk space.
Preserve the legacy `reclaimed_bytes` result field for compatibility if required by existing consumers.
Add an explicit `quarantined_bytes` field for new UI copy, or derive it from the successful quarantine entries.
Permanent deletion retains its existing reporting and retention deadline.

## Presentation

Add a System temporary folders disclosure beside Kandev temporary artifacts in the analysis section.
Show full folder size and the visible note: `Informational. Can overlap counted categories.`
Expanded content shows resolved roots, partial status, and measurement limitations.
Do not add a cleanup button to the broad-folder row.

Add a Temporary artifacts policy card with an off-by-default switch.
Its label is `Clean stale Kandev temporary artifacts`.
Visible help states the 24-hour interval, registered scope, quarantine delay, and shared-file exclusion.
Retain the explicit cleanup action with clear scope and busy/result states.
Use the existing dirty-state, save, authorization, and disabled-reason patterns.

Desktop and phone share the domain hook, policy state, resource model, and action handlers.
The nearest shipped exemplar is the Storage accordion covered by `mobile-storage-maintenance.spec.ts`.
The mobile entry is Settings > Storage. Root details remain inline because they are short reference content.
Keep one page scroll owner. Paths wrap within the resource detail.
Keep required help visible inline. No tooltip or overlay is needed to understand cleanup scope.
Phone controls have at least 44-pixel hit targets. Fine-pointer buttons retain 28-pixel density.
No fixed toolbar or new navigation route is required.
All new copy uses `system` translations in all five supported language catalogs.

## Verification and delivery

Use isolated roots and injected mount/liveness failures for backend tests.
Browser fixtures must inject disposable temp roots, never inspect or mutate the host `/tmp`.
Desktop and phone flows cover analysis, refresh, partial results, policy persistence, and explicit cleanup.
Real provider tests prove that cleanup preserves unregistered and active files.

The [implementation plan](../../../plans/storage-temporary-folders/plan.md) records the completed
work orders and evidence. Completed companion plans remain historical records and link this
successor package. The public operations guidance describes the shipped scope and limits.
