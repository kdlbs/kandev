---
status: draft
system: system-page
created: 2026-09-27
owners:
  - kandev
---

# Database Statistics Snapshot Requirements

## Overview

The Data & Logs page must remain useful when logical database-size measurement is slow or the database is temporarily busy. System-page owns the operator-facing database statistics contract and its measurement lifecycle. The snapshot is installation-wide and survives browser navigation and page refresh while the backend process remains running.

## Terminology

- **Live metadata:** Database driver, configured path, backup directory, database and WAL sizes, schema version, and latest backup time.
- **Logical totals:** The byte counts for message content, message metadata, external message payloads, and Git snapshots. These are sampled estimates, not physical file sizes.
- **Snapshot:** The last complete set of logical totals with its measurement time.

## Requirements

### REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001: Responsive database status

**Intent:** Opening or refreshing Data & Logs must not repeat a long logical-size scan in the request path.

**User story:** As an operator, I want the database status to remain readable while size analysis runs, so that I can inspect maintenance controls without waiting for a full scan.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.1:** A database-status read shall return live metadata and the current logical-snapshot state without waiting for the logical totals to finish. The read shall have a bounded database wait.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.2:** On a cold read, logical totals shall be explicitly pending or unavailable. An absent total shall never be displayed or exported as a measured zero.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3:** Only one logical scan shall run per installation at a time. Repeated page loads and concurrent readers shall share its progress and last complete snapshot. A mounted page shall revalidate on mount, including when the browser store already has a response, and poll while a scan is pending or refreshing. Polling shall stop on unmount.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.4:** A complete snapshot shall remain available across page refreshes for 15 minutes. After it expires, reads shall keep showing its measured time while a background refresh runs. A mounted page shall revalidate when a ready snapshot expires. A failed refresh shall preserve the last complete snapshot, mark it stale, and allow a later retry.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5:** The page shall distinguish current, refreshing, stale, and unavailable logical measurements. Database maintenance controls and backup-location guidance shall remain usable when logical totals are pending or stale.

### REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002: Bounded measurement and recovery

**Intent:** Background measurement must not become another source of sustained database contention.

#### Acceptance criteria

- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.1:** Logical measurement shall use bounded batches, yield between batches, observe cancellation, and release database resources between batches. A partial scan shall not replace a complete snapshot or update logical-total metrics.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.2:** The snapshot shall be invalidated or refreshed after database maintenance or restore changes its meaning. A late result from an older database generation shall not overwrite a newer result.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3:** A transient database error shall not erase a last good browser or server snapshot. While the page remains mounted, it shall revalidate stale or unavailable status after a bounded delay so a retry can recover without a full page reload. An explicit user retry shall bypass the remaining server retry backoff and request one shared background scan. Recovery shall clear a read error without retrying maintenance actions. A real failed analysis shall remain identifiable as failed.
- **AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.4:** Desktop and phone users shall see the same measurement state and timestamp. The existing page shall keep one scroll owner, readable wrapping, accessible status text, and no horizontal overflow. New copy shall be localized.

## Out of scope

- Persisting logical snapshots across backend restarts or storing them in the database.
- Changing the eligibility or retention policy for message compaction.
- Automatic VACUUM, backup creation, or cleanup.

## Related documents

- [System design](../system-design/database-statistics-snapshot.md)
- [Implementation plan](../../../plans/database-statistics-resilience/plan.md)
- [Tool payload retention](tool-payload-retention.md)
