# ADR-2026-09-05-bounded-progressive-storage-analysis: Use bounded progressive storage analysis

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend, frontend, protocol

## Context

The storage overview already runs its top-level providers concurrently. Large workspace and cache
trees still take a long time because each provider walks directories serially.

The overview endpoint also waits for every provider. A cold or expired cache therefore leaves the
analysis card empty until the slowest scan finishes.

Operators need useful progress, but a partial scan must not look like a complete snapshot. Storage
paths and byte counts also must not leak through broadcast WebSocket events.

## Decision

Storage analysis will use a shared limit of four active read-only directory partitions. The scanner
will preserve the symlink, marker, containment, and ownership rules of each provider.

The overview cache will use stale-while-revalidate reads. A read returns the last successful
snapshot immediately and starts one background refresh when that snapshot is absent or expired.

The last successful snapshot remains atomic. During the first cold scan, partial results remain in
a separate current-scan field and every aggregate is identified as incomplete.

The cache will publish `system.storage.analysis.updated` after bounded progress changes. The event
contains only generation and state. An authorized client refetches the admin endpoint for details.

The WebSocket event is the primary update signal. A short poll operates only while analysis is
active and recovers a missed event.

The snapshot cache remains process-local. The page requests a refresh at the cache deadline while
it stays open, and manual Analyze requests an immediate refresh.

## Consequences

- Independent directory partitions can overlap without unbounded filesystem pressure.
- A cold scan can show completed sources before the full snapshot is ready.
- An expired snapshot stays useful while Kandev measures its replacement.
- The API must distinguish complete snapshots from current-scan partial results.
- A backend restart still has a cold cache.
- The frontend needs event-triggered reads, a bounded poll fallback, and refresh-time scheduling.
- Scan metadata can show duration, cache lifetime, and the next page-owned refresh time.

## Alternatives Considered

- **Keep the blocking endpoint.** Rejected because top-level parallelism cannot improve first-result
  latency when one filesystem provider dominates.
- **Send full progress in WebSocket events.** Rejected because broadcast events can expose paths,
  sizes, warnings, or errors outside the admin request boundary.
- **Persist completed snapshots across backend restarts.** Rejected because it adds a migration,
  schema-version rules, and stale host-state recovery for a cache.
- **Replace snapshot values source by source during refresh.** Rejected because one displayed total
  can then mix measurements from different times.
- **Start a permanent backend refresh timer.** Rejected because Kandev scans idle installations
  that have no active Storage viewer.
- **Use unbounded or file-level parallelism.** Rejected because it can increase disk contention,
  allocate excessive goroutines, and complicate symlink safety.
