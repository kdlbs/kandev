---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002
---

# Progressive Storage Analysis System Design

## Purpose and boundaries

The system-page system owns storage analysis, its cache, and the operator-visible progress contract.
The platform event bus transports notifications but does not own storage state.

This design keeps cleanup behavior unchanged. It changes only read-only analysis, cache reads,
progress delivery, and the Storage page presentation.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002` | [Analysis state](#analysis-state), [Filesystem scan](#filesystem-scan), [Cache flow](#cache-flow), [Progress delivery](#progress-delivery), [Storage page](#storage-page), [Failure and recovery](#failure-and-recovery) |

## Current behavior

`storageOverview.Summary` already starts workspace, Go-cache, quarantine, Docker, and temporary-artifact analysis concurrently.
The slow path is now inside the filesystem providers.

`workspaces.Provider.Analyze` measures discovered task roots one after another. The Go-cache and
temporary-artifact providers also use serial `filepath.WalkDir` calls.

`OverviewCache.Get` waits for the full scan when the cache is empty or expired. The frontend cannot
receive partial values because `GET /api/v1/system/storage` waits for `Get` to finish.

## Components and responsibilities

### Bounded filesystem scanner

Add `internal/system/storage/filescan` for read-only byte measurement. It will preserve the symlink
and exclusion policy of each provider.

One shared limiter permits at most four active directory partitions for one storage analysis.
The limit includes workspace, Go-cache, and temporary-artifact walks.

The scanner partitions each requested root by its immediate children. Each partition uses a serial
`filepath.WalkDir`, so no two workers count the same entry.

The scanner will provide these functions:

- measure one or more roots with indexed results.
- reject or skip symlinks according to the policy of the caller.
- exclude provider-owned marker files.
- stop between filesystem operations when the context is canceled.
- report completed partitions, measured bytes, and completed roots.

The scanner will aggregate results by input index. Worker completion order will not change warning
order or the final response.

The concurrency limit is an internal constant. This change will not add an operator setting or an
environment variable.

### Progressive overview provider

Extend the overview provider with a progress callback. `storageOverview` will report source start,
source progress, source completion, and source failure.

The source identities are `workspaces`, `go_cache`, `quarantine`, `temporary_artifacts`, and
`docker`. Disabled sources remain terminal and do not keep the scan active.

The workspace source will forward completed-root and byte progress from `filescan`. Other sources
will report the finest stable progress that their provider can supply.

### Overview cache

`OverviewCache` remains the authority for single-flight refresh and process-local snapshots. It will
also own the active scan state and its generation.

The cache will add a nonblocking read method for the HTTP handler. This method returns the current
snapshot and scan state, then starts one background refresh when the snapshot is absent or expired.

`Refresh` will remain the manual Analyze path. It will bypass a fresh cache entry but join an active
scan instead of starting duplicate filesystem work.

`Invalidate` will advance the generation and clear the completed snapshot. A late callback from an
older generation will not change progress or cache state.

### Progress publisher

The cache will accept a narrow publisher callback. Backend wiring will publish
`system.storage.analysis.updated` after meaningful state changes.

The event payload will contain only the scan generation and state. It will not contain paths, byte
counts, warnings, or errors.

The WebSocket gateway will map the event to the same action name. Authorized clients will read the
full state from the admin storage endpoint.

## Analysis state

The storage response will keep its settings and capabilities fields. It will make `summary` and
`analyzed_at` nullable for the first scan and add an `analysis` object.

```text
StorageAnalysisState
  generation: unsigned integer
  state: scanning | ready | failed
  started_at: timestamp or null
  completed_at: timestamp or null
  duration_ms: integer or null
  cache_ttl_seconds: integer
  refresh_due_at: timestamp or null
  stale: boolean
  error: string or null
  progress:
    completed_sources: integer
    total_sources: integer
    sources: map of StorageSourceProgress
  partial_summary: partial Summary or null
```

`StorageSourceProgress` contains a source state, completed items, total items, and bytes scanned.
Item counts are optional when a provider cannot determine a stable total.

`summary` always contains the last complete successful snapshot. The backend never mixes values
from different scans into this field.

`partial_summary` contains only results from the current first scan. The frontend must identify its
aggregate as counted so far.

`refresh_due_at` equals `analyzed_at` plus the 15-minute cache lifetime. It is a demand time, not a
daemon schedule.

## Filesystem scan

`backendapp` will construct one `filescan.Limiter` for each overview provider. It will inject that
limiter into workspace, Go-cache, and temporary-artifact analysis.

Each provider will keep its current ownership and path rules. The shared scanner measures bytes but
does not classify ownership or choose cleanup candidates.

Workspace analysis will discover and protect roots before it starts measurement. Then it will send
the non-overlapping roots to the bounded scanner.

Go-cache analysis will preserve its current marker exclusion. It will reject or skip symlinks with
the same policy as the current provider.

Temporary-artifact analysis will continue to measure only registered roots. It will not scan the
shared operating-system temporary directory.

Cleanup providers will remain unchanged. Destructive work will not use the concurrent read-only
scanner.

## Cache flow

### Fresh snapshot

The nonblocking read returns the snapshot with `state=ready`. It does not start filesystem work.

### First scan

The nonblocking read creates a scan generation and starts a background refresh. It returns
`state=scanning` without waiting for any directory partition.

Progress callbacks update `partial_summary`. When all sources finish, the cache commits one atomic
snapshot and records completion time, duration, and refresh time.

### Expired snapshot

The nonblocking read returns the expired snapshot with `stale=true` and starts one background
refresh. The old summary stays visible until the replacement snapshot is complete.

The frontend will request a read when `refresh_due_at` arrives while the page is mounted. Thus, the
displayed next-refresh time is accurate for an open Storage page.

### Manual Analyze

Manual Analyze starts a forced refresh through the current tracked operation. A concurrent cache
refresh joins the same flight.

The Analyze button will keep its current job state. Scan progress will come from the analysis state,
not from a second job record.

## Progress delivery

The cache will publish these transitions:

1. A scan starts.
2. A source starts or reports bounded progress.
3. A source finishes or fails.
4. The full scan finishes or fails.

Byte progress will be throttled to at most four updates per second. Source terminal transitions will
publish immediately.

The web store will keep an `analysisRevision`. The WebSocket handler will advance that revision for
each notification.

`useStorageMaintenance` will reload only the overview after a revision change. While a scan is
active, it will also poll the endpoint every 1.5 seconds.

The polling fallback stops after a ready or failed response. Existing request generations will
prevent an older response from replacing a newer analysis state.

## Storage page

The analysis card will use the complete snapshot when one exists. During the first scan, it will use
the partial summary and source progress.

Completed sources will show their measured values. Pending sources will show localized progress or
an indeterminate state.

The total label will say that the value is counted so far until the first scan finishes. It will
retain the existing partial-data badge when a completed source is unavailable.

An information icon will appear beside the completed snapshot time. Its disclosure will show:

- the scan duration.
- the 15-minute cache lifetime.
- the next refresh time in absolute and relative form.

The disclosure will explain that Analyze refreshes immediately. It will not claim that the backend
scans while no Storage page or client is active.

The existing `StorageSettingHelp` interaction is the nearest mobile example. The new icon will open
on hover and keyboard focus, and pointer activation will pin it.

On a phone, the same button will use the current 44-pixel hit area. A second tap, outside interaction,
or Escape will close the disclosure.

The page will keep one document scroll owner. This change adds no drawer, fixed control, or horizontal
scrolling region.

## Failure and recovery

A provider error will keep the current source-specific unavailable value. Other source results will
continue to update and complete.

A top-level scan error will set `state=failed`. If a successful snapshot exists, that snapshot will
remain visible and keep its original `analyzed_at` value.

A failed first scan will keep completed partial sources visible. The card will show a retry action
through Analyze and will identify the failed attempt.

If the request context closes, the background first scan will continue with a service-owned context.
Manual Analyze will keep the current tracked-job cancellation behavior.

Invalidation will discard late progress and completion from the old generation. WebSocket delivery
loss will not block recovery because the client polls while a scan is active.

## Persistence

The cache and progress state remain process-local. A backend restart starts a new cold scan.

No database migration or snapshot file is part of this change. Storage runs will keep their current
durable history.

## Security

The storage endpoint remains in the admin route group. The progress event carries no storage data,
so it does not expose paths or byte counts through the broadcast bus.

The scanner will not follow symlinks. It will preserve existing containment checks and ownership
classification before it reads a workspace root.

## Observability

The backend will log one structured completion record for each scan. The record will contain total
duration, source durations, source outcomes, partition count, and maximum active walkers.

Add counters for scans started, joined, completed, and failed. Add histograms for full duration and
source duration.

Logs and metrics will not include task paths. Live performance validation will compare serial and
bounded scans on the same dataset when that environment is available.

## Related decisions

- [Bounded progressive storage analysis](../../../decisions/2026-09-05-bounded-progressive-storage-analysis.md)
- [Install-wide storage maintenance](../../../decisions/0045-install-wide-storage-maintenance.md)
