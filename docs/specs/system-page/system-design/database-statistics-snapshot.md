---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
created: 2026-09-27
updated: 2026-09-28
owners:
  - kandev
---

# Database Statistics Snapshot System Design

## Purpose and boundaries

`internal/system/database` owns the database-statistics endpoint. It currently computes four `SUM(LENGTH(...))` totals synchronously on every `GET /api/v1/system/database`. The Data & Logs route needs live low-cost metadata and a process-local snapshot for those expensive totals. Existing `internal/system/storage/OverviewCache` is the model for a single background flight and stale-while-refresh behavior, but its host-storage data model is separate.

Tool-payload savings analysis already runs as a background operation and persists `last_analysis` in `toolretention`. It must not be reimplemented as another cache. The [retention design](tool-payload-retention.md) remains authoritative for that job.

## Requirement mapping

| Requirement                                   | Design sections                                 |
| --------------------------------------------- | ----------------------------------------------- |
| `REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001` | API and cache, Presentation                     |
| `REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002` | Scanner, Invalidation and failure, Verification |

## API and cache

Split `database.Service.Stats` into a bounded live-metadata read and a process-local logical-totals cache. `GET /api/v1/system/database` retains the current live fields and adds `logical_stats_state` (`pending`, `ready`, `refreshing`, `stale`, `unavailable`) and `logical_stats_measured_at`. The four existing logical byte fields become nullable when no complete snapshot exists; zero is returned only after a successful measurement. Add a `logical_stats_error` stable code only for a failed scan. Keep raw SQL errors in server logs. Update the TypeScript type and public API documentation together; callers must handle null explicitly.

The metadata read uses the request context with a short bounded database deadline. Cache the last successful metadata for temporary read failures. Add `metadata_stale` and `metadata_measured_at` so a fallback cannot appear current. If no metadata has ever succeeded, return a bounded error. The backup directory is derived from the configured database path and remains available independently of logical totals. Retain existing route permissions and do not put the path in an event or metric label.

The cache starts one background logical scan when empty or older than 15 minutes. Concurrent GETs observe one flight. The first scan returns `pending`; an expired snapshot returns `refreshing` with its previous totals and measurement time. A failure keeps those totals and returns `stale`; with no prior snapshot it returns `unavailable`. Use bounded retry backoff after failure so every page refresh cannot start another scan. `POST /api/v1/system/database/refresh` explicitly clears the remaining backoff and requests one shared background scan, but never runs the scan inside HTTP. A worker context belongs to service lifecycle rather than the originating request; shutdown cancels it.

## Scanner

Replace the four whole-table aggregates in `readLogicalStorageStats` with ordered keyset batches against the existing tables. Each batch obtains one bounded reader query, totals the existing length expressions, closes rows, and releases its database connection before the next batch. A run fixes an upper key per table when it starts, so newly inserted rows wait for the next sample. Concurrent edits can affect a sample; the result is a timestamped estimate. Use one worker, a finite batch size, a per-batch deadline, and a short yield under contention. Abort a run on repeated errors or service shutdown. Publish all four totals atomically only after every table completes. No partial result may become a zero or replace the last complete snapshot.

## Invalidation and failure

Invalidate on successful VACUUM/optimize and on restore/reset generation change through existing service wiring. A generation token prevents an old flight from publishing after invalidation. Do not measure during exclusive maintenance or a known unhealthy persistence interval; defer and retry. Keep the last complete snapshot visible as stale when safe, but never claim a value from a replaced database is current. If restore/reset changes the selected database, clear the old snapshot and metadata before responding for the new generation.

The current retention status hook already keeps its last status through GET failure and clears a recovered read error on the next successful poll. Change the card's status-read error copy to describe status unavailability instead of a failed operation; keep action and persisted-operation errors distinct. Polling remains bounded and never retries mutation automatically. The page's independent cards stay mounted and usable according to their own state.

## Presentation

The Data & Logs page has two consumers of the database-statistics response:
DatabaseStatsCard renders database details, and the Backups section uses
`backup_directory` in its resolved-directory description. `DatabasePanel`
calls `useDatabaseStats` once and passes the same result to both consumers;
DatabaseStatsCard does not create an independent observer. TanStack Query owns
the `DatabaseStats` snapshot and GET request state. Zustand does not mirror
this response or expose a setter for it; other System resources remain in
Zustand. The query is lazy and starts when the Data & Logs database panel
mounts. The boot payload and hydration remain unchanged and contain no database
statistics. The backend process-local snapshot supplies the previous logical
measurement after a full document reload.

The database query key contains the canonical full API base URL, page boot ID,
auth mode, authenticated state, and user ID. The resource is installation-wide
and has no workspace key. The authenticated application branch keeps the
existing stable QueryClient. Identity-change cleanup targets only obsolete
SystemInfo and database-statistics queries, cancels them, and immediately
removes them with the same filter. It does not wait for cancellation before
removal or clear unrelated Query entries. Leaving the authenticated app branch
unmounts the provider. The database query passes TanStack's observer
AbortSignal through fetchDatabaseStats to the existing fetchJson transport;
the transport continues to send session cookies and use the existing endpoint.

Database statistics are mutable, so SystemInfo's process-lifetime freshness
does not apply. The query revalidates on every mount, even when it has cached
data. It polls every two seconds while logical statistics are pending or
refreshing, every 30 seconds while they are stale or unavailable, and at the
15-minute snapshot expiry calculated from logical_stats_measured_at while
they are ready. Read errors with cached data use the 30-second recovery delay.
The interval stops when the last observer unmounts. The query does not retry
automatically or refetch on browser focus or reconnect. It attempts requests
while the browser reports offline so the current immediate error and bounded
recovery behavior remain visible. Keep the cache entry for the authenticated
app branch lifetime to preserve the last good response across SPA navigation;
identity cleanup removes obsolete entries, and no browser cache is persisted
across a full document reload.

reload explicitly refetches the same Query entry and resolves without throwing
on a read error, preserving the current caller contract. The retry action calls
the existing refresh endpoint first to bypass server backoff, then refetches
the status query. A failed status read keeps the previous successful Query
data while exposing the error in the card. A failed refresh command is also
shown as an error and does not escape the button handler. DatabaseStatsCard
receives the shared hook result as props and keeps its current presentation and
behavior: metadata appears as soon as it arrives, the localized
measurement-status line includes its measured time when available, and stale or
unavailable totals remain retryable. Direct card tests provide that result
through the component props. The four logical values have no row in this card,
so the migration does not add a new table. The Backups description uses
`backup_directory` from the same result, preserving the resolved location
without a Zustand mirror or second Query observer.

The same card and hook serve desktop and phone. The shipped Data & Logs settings
surface is the mobile exemplar: one document scroll owner and compact cards.
The status line wraps, has text beyond color, and its retry control uses the
existing responsive settings action sizing. No new overlay or navigation is
needed.

## Observability and security

Log scan start, completion, duration, and failure class without row content or database path. Record a fixed-state metric for scan outcome and the latest complete measurement time. Keep logical-byte gauges from the last successful complete scan; a failed scan does not replace them with zero. Do not add task, message, path, or user IDs to metric labels. The endpoint retains existing read authorization and maintenance mutations remain admin-only.

## Verification

Backend tests cover cold reads, concurrent readers, expiry, failure with and
without a snapshot, retry backoff, cancellation, batch yielding, maintenance
deferral, and generation invalidation. Use SQLite fixtures and an
environment-gated PostgreSQL behavior test for changed dialect-sensitive
queries.

Frontend tests cover nullable totals and independent metadata/error states,
plus shared query ownership, mount revalidation, state-based freshness,
explicit retry, error recovery, cancellation, and identity cleanup. The
database-card test preserves its prop contract. `system-route-copy.test.ts`
verifies the Backups description uses the shared `backup_directory` and is
omitted when unavailable.

Desktop and mobile Playwright cover a pending scan, page reload with one flight,
stale result, transient 503 recovery, and available backup/maintenance
controls. `backups-page.spec.ts` verifies the resolved directory remains
visible.

## Related documents

- [Requirement](../requirements/database-statistics-snapshot.md)
- [Existing storage snapshot pattern](storage-database-footprint.md)
- [Implementation plan](../../../plans/database-statistics-resilience/plan.md)
- [Query ownership implementation plan](../../../plans/database-statistics-query-migration/plan.md)
- [Database Statistics Query Ownership](../../../decisions/2026-09-27-database-stats-query-cache-ownership.md)
