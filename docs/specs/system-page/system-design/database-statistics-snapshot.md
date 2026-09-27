---
status: draft
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
created: 2026-09-27
owners:
  - kandev
---

# Database Statistics Snapshot System Design

## Purpose and boundaries

`internal/system/database` owns the database-statistics endpoint. It currently computes four `SUM(LENGTH(...))` totals synchronously on every `GET /api/v1/system/database`. The Data & Logs route needs live low-cost metadata and a process-local snapshot for those expensive totals. Existing `internal/system/storage/OverviewCache` is the model for a single background flight and stale-while-refresh behavior, but its host-storage data model is separate.

Tool-payload savings analysis already runs as a background operation and persists `last_analysis` in `toolretention`. It must not be reimplemented as another cache. The [retention design](tool-payload-retention.md) remains authoritative for that job.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001` | API and cache, Presentation |
| `REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002` | Scanner, Invalidation and failure, Verification |

## API and cache

Split `database.Service.Stats` into a bounded live-metadata read and a process-local logical-totals cache. `GET /api/v1/system/database` retains the current live fields and adds `logical_stats_state` (`pending`, `ready`, `refreshing`, `stale`, `unavailable`) and `logical_stats_measured_at`. The four existing logical byte fields become nullable when no complete snapshot exists; zero is returned only after a successful measurement. Add a `logical_stats_error` stable code only for a failed scan. Keep raw SQL errors in server logs. Update the TypeScript type and public API documentation together; callers must handle null explicitly.

The metadata read uses the request context with a short bounded database deadline. Cache the last successful metadata for temporary read failures. Add `metadata_stale` and `metadata_measured_at` so a fallback cannot appear current. If no metadata has ever succeeded, return a bounded error. The backup directory is derived from the configured database path and remains available independently of logical totals. Retain existing route permissions and do not put the path in an event or metric label.

The cache starts one background logical scan when empty or older than 15 minutes. Concurrent GETs observe one flight. The first scan returns `pending`; an expired snapshot returns `refreshing` with its previous totals and measurement time. A failure keeps those totals and returns `stale`; with no prior snapshot it returns `unavailable`. Use bounded retry backoff after failure so every page refresh cannot start another scan. An explicit status refresh may request a retry after the backoff, but never runs the scan inside HTTP. A worker context belongs to service lifecycle rather than the originating request; shutdown cancels it.

## Scanner

Replace the four whole-table aggregates in `readLogicalStorageStats` with ordered keyset batches against the existing tables. Each batch obtains one bounded reader query, totals the existing length expressions, closes rows, and releases its database connection before the next batch. A run fixes an upper key per table when it starts, so newly inserted rows wait for the next sample. Concurrent edits can affect a sample; the result is a timestamped estimate. Use one worker, a finite batch size, a per-batch deadline, and a short yield under contention. Abort a run on repeated errors or service shutdown. Publish all four totals atomically only after every table completes. No partial result may become a zero or replace the last complete snapshot.

## Invalidation and failure

Invalidate on successful VACUUM/optimize and on restore/reset generation change through existing service wiring. A generation token prevents an old flight from publishing after invalidation. Do not measure during exclusive maintenance or a known unhealthy persistence interval; defer and retry. Keep the last complete snapshot visible as stale when safe, but never claim a value from a replaced database is current. If restore/reset changes the selected database, clear the old snapshot and metadata before responding for the new generation.

The current retention status hook already keeps its last status through GET failure and clears a recovered read error on the next successful poll. Change the card's status-read error copy to describe status unavailability instead of a failed operation; keep action and persisted-operation errors distinct. Polling remains bounded and never retries mutation automatically. The page's independent cards stay mounted and usable according to their own state.

## Presentation

`useDatabaseStats` receives the live metadata and logical snapshot state. Its store preserves last good values across route changes within the SPA; server cache supplies them after a full page refresh. Revalidate when the hook mounts even if the browser store already has a response. Poll every two seconds while the logical state is pending or refreshing. Poll stale or unavailable states every 30 seconds so a retry can start after backend backoff without a page reload. For a ready response, schedule a revalidation at its 15-minute expiry using `logical_stats_measured_at`; if the old snapshot remains visible after that check, retry every 30 seconds. Clear scheduled polls on unmount. `DatabaseStatsCard` shows metadata as soon as it arrives. Add one compact, localized measurement-status line with measured time where available; an unavailable first scan shows a retryable status. The four logical values currently have no row in this card, so do not introduce a new table. Backups continue to read `backup_directory` from the same response.

The same card and hook serve desktop and phone. The shipped Data & Logs settings surface is the mobile exemplar: one document scroll owner and compact cards. The status line wraps, has text beyond color, and its retry control uses the existing responsive settings action sizing. No new overlay or navigation is needed.

## Observability and security

Log scan start, completion, duration, and failure class without row content or database path. Record a fixed-state metric for scan outcome and the latest complete measurement time. Keep logical-byte gauges from the last successful complete scan; a failed scan does not replace them with zero. Do not add task, message, path, or user IDs to metric labels. The endpoint retains existing read authorization and maintenance mutations remain admin-only.

## Verification

Backend tests cover cold reads, concurrent readers, expiry, failure with and without a snapshot, retry backoff, cancellation, batch yielding, maintenance deferral, and generation invalidation. Use SQLite fixtures and an environment-gated PostgreSQL behavior test for changed dialect-sensitive queries. Frontend tests cover nullable totals and independent metadata/error states. Desktop and mobile Playwright cover a pending scan, page reload with one flight, stale result, transient 503 recovery, and available backup/maintenance controls.

## Related documents

- [Requirement](../requirements/database-statistics-snapshot.md)
- [Existing storage snapshot pattern](storage-database-footprint.md)
- [Implementation plan](../../../plans/database-statistics-resilience/plan.md)
