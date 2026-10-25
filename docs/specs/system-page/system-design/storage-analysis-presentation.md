---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-003
  - REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-005
created: 2026-09-22
owners:
  - kandev
---

# Storage analysis presentation

## Purpose and boundaries

System-page owns storage measurements and their operator-facing presentation.
This design extends the existing Storage accordion with relative bars and concise deadline feedback.
It preserves the [temporary-storage reader](storage-temporary-folders.md), cleanup ownership, and classified total.

The user accepted descending bars, unchanged collapsible behavior, a phone layout, and concise timeout feedback.
The implementation is recorded in the [plan package](../../../plans/storage-analysis-presentation/plan.md).
No new persistence, endpoint, release flag, scan limit, or cleanup action is required.

## Requirement mapping

| Requirement or criterion | Design section |
| --- | --- |
| AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.10 | Deadline feedback |
| REQ-SYSTEM-PAGE-STORAGE-MAINTENANCE-005 | Resource measurements, Ordering and scale, Accordion presentation |
| AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-003.3, .6, .8 | Compatibility, Deadline feedback, Phone composition |

## Current source evidence

Paths below are relative to the repository root.

- `apps/backend/internal/system/storage/filescan/measure.go` joins partition errors in `Limiter.MeasureWithOptions`.
- `apps/backend/internal/system/storage/tempstore/provider.go` walks joined error leaves in
  `measurementFromResult`, removes deadline leaves, and retains distinct non-deadline diagnostics.
- The warning count is bounded at the projection boundary, including warnings carried by joined errors.
- `storage-overview-resources.ts` normalizes warning strings for the visible row and projects numeric
  measurements for sorting and scale derivation.
- `StorageResource` carries optional numeric size, bar scale, and partial-state fields alongside
  localized display values.
- `StorageOverviewResources` renders `Accordion type="multiple"`, with stable resource IDs as keys and item values.

Frontend filenames in this design are under `apps/web/components/settings/system/storage/`, unless another path is given.

## Resource measurements

Extend the local `StorageResource` view model with optional `sizeBytes` and a partial-status field.
Read numeric bytes from the current summary. Never parse `value`, which contains localized text.
Do not change the public `StorageOverviewResponse` shape.

| Row | Numeric source |
| --- | --- |
| Task workspaces | `workspaces.total_bytes` |
| Database / Database backups | respective `size_bytes` for measured status |
| Quarantine | `quarantine.size_bytes` unless unavailable |
| System temporary folders | `system_temporary.size_bytes` for measured or partial status |
| Managed Go cache | `go_cache.size_bytes` unless unavailable |
| Distinct user Go cache | `go_cache.unmanaged_size_bytes`, only for the existing distinct-path row |
| Registered temporary artifacts | `temporary_artifacts.total_bytes` unless unavailable |
| Kandev containers | `docker.managed_container_bytes` when Docker is available |
| Docker image layers | `docker.image_layer_bytes` when Docker is available |
| Docker build cache | `docker.build_cache_bytes` when Docker is available |
| Unused Docker images | `docker.unused_image_bytes` when Docker is available |

Only finite, nonnegative numbers are eligible. Missing or invalid fields have no numeric bar.
Explicit zero is a measurement. Do not convert absent values to zero for sorting or display.
Status gates take precedence over stale numeric fields in unavailable or not-applicable responses.
Keep existing detail text, source progress, ownership warnings, and action handlers.
Expose the existing system-temporary partial status in its header beside the row title.

## Ordering and scale

Build the existing canonical category list, then sort a copy by descending `sizeBytes`.
Use canonical category position for equal measurements and unmeasured rows.
All numeric measurements, including zero and partial samples, precede unmeasured rows.
Locale changes must not change ties. Keep all existing categories, including subset measurements.

The maximum is the largest eligible measurement in the displayed snapshot.
Each fill is `100 * sizeBytes / maximum`, clamped to 0–100.
For an all-zero snapshot, every measured track is empty. Never divide by zero.
Do not impose a minimum fill that overstates small measurements.

Keep the current `summary ?? analysis.partial_summary` snapshot preference.
First-scan source completion updates order and scale from available measurements.
During refresh, the displayed snapshot continues to own both until atomic replacement.
Keep resource IDs stable so ordering does not reset expansion, focus, or existing action state.
No saved sort state or additional scan requests are required.

The maximum includes informational and subset rows because their full footprints are being compared.
It is not Total counted or disk capacity. Existing overlap rules in `storage-totals.ts` remain unchanged.

## Accordion presentation

Keep the existing Accordion and full-row trigger, including its chevron and keyboard behavior.
Use a flexible desktop header with label, rounded track, size, then chevron.
Use existing theme tokens for the muted track and one consistent fill.
Use amber status text or a badge beside the row title for partial measurements, without encoding size thresholds.
Keep existing action buttons inside expanded content.

Render bars as decorative spans with `aria-hidden="true"`.
The trigger already exposes the label, formatted size, partial status, and expansion.
A relative footprint is not scan progress. Do not give it progress-bar semantics.
Bars introduce no tooltip, keyboard stop, animation requirement, or nested interactive control.

Add visible localized explanatory text near the rows:
"Bars compare category sizes. Categories can overlap."
Keep the existing classified-total scope explanation.
Long labels wrap. Numeric values and chevrons remain readable.

## Phone composition

Entry: Settings > System > Storage. Primary row action: tap to expand or collapse.
The nearest shipped exemplar is the Storage accordion in `mobile-storage-maintenance.spec.ts`.
Its inline detail and whole-row activation fit short reference content.
The curated mobile guide contributes one focal flow, visible actions, and a single scroll owner.

Below the existing 768-pixel phone boundary, place the bar beneath the label and value.
A long label can occupy its own line. Keep the chevron at the trailing edge.
Use one shared resource model and Accordion state across viewport changes.
The existing page owns scrolling and safe-area behavior. Do not add a fixed footer or nested scroller.
Keep phone/coarse-pointer trigger hit targets at least 44 pixels.
Preserve desktop density and existing action sizing.

Verify the configured Pixel 5 viewport, 767 pixels, and 768 pixels.
Check actual bar position, target bounds, wrapped paths, and page containment.
The plan owns the exact [ASCII previews](../../../plans/storage-analysis-presentation/plan.md#ascii-ui-preview).

## Deadline feedback

Retain the current 60-second source deadline, sampled bytes, partial status, and `reason: deadline`.
A deadline explains an incomplete measurement. It does not imply failed cleanup or corrupt storage.

Normalize warning examples at the `tempstore` projection boundary.
Walk joined error leaves before converting them into strings.
Discard leaves classified as `context.DeadlineExceeded`, while retaining distinct non-deadline diagnostics.
Do not discard a mixed joined error merely because `errors.Is` finds a deadline somewhere inside it.
Merge remaining examples with `result.Warnings`, deduplicate them, and apply the existing ten-example limit.
Apply the same deduplication when `summarize` combines root warnings.
Retain skipped-entry counts exactly. Cancelled partitions do not invent skipped-entry counts.
Keep parent cancellation behavior and shared strict scanner callers unchanged.

In `systemTemporaryResource`, derive one localized explanation from the summary or root `reason: deadline`:
"Scan timed out. Showing partial usage."
Show it once in the expanded category, alongside existing root sizes, partial status, and skipped counts.
Other bounded diagnostics remain visible.

Handle cached or older payloads at this same frontend boundary.
Split legacy warning strings on line breaks, discard exact trimmed deadline-only lines, and deduplicate remaining examples.
Use structured reasons first. Legacy exact deadline-only lines can also identify this known timeout.
Do not match arbitrary path substrings or hide unrelated warnings.
Cap the rendered examples at ten, independently of the timeout explanation.
No cache flush, migration, automatic retry, or new diagnostic overlay is required.

## Compatibility and verification

The [temporary-storage ownership decision](../../../decisions/2026-09-11-temporary-storage-visibility-policy.md)
remains authoritative. Analysis grants no cleanup authority.
Storage access restrictions, safe traversal, events, and API authorization remain unchanged.
Total counted excludes system temporary folders and avoids double-counting existing subsets.

All new copy uses `t()` in the `system` namespace.
Update English, pt-pt, zh-cn, zh-hk, zh-tw, and ja.
Generate the Traditional Chinese pair with `pnpm run i18n:zh-hant`.

Use deterministic joined errors and injected scanner results for timeout tests.
Do not wait 60 seconds or scan host temporary folders.
Use frontend unit tests for source mapping, sorting, invalid values, ratios, and old payloads.
Use component and browser tests for expansion, refresh, focus, localization, and mobile geometry.
The plan records exact commands and implementation results.
Public operations documentation now explains relative bars and temporary-folder timeout behavior.
