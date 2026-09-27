---
created: 2026-09-27
status: complete
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
system_design:
  - ../../specs/system-page/system-design/database-statistics-snapshot.md
  - ../../specs/system-page/system-design/tool-payload-retention.md
legacy_specs: []
---

# Implementation Plan: Resilient Database Statistics

## Overview

Make database statistics quick to read and reusable across browser refreshes. Implement the server snapshot and bounded scanner first, then update the Data & Logs presentation and compaction-status feedback. The cache remains in the backend process; a backend restart starts a new cold measurement. Implementation follows the approved requirements and work orders and is tracked in the feature branch.

## Evidence and intent

The live backend diagnostic bundle from September 27 records a compaction Analyze POST accepted at 12:22:39 local time (HTTP 202). Status GETs returned 200 until a persistence probe classified the database as unhealthy at 12:23:50. Two compaction status GETs then returned 503; health and status recovered at 12:24:03. A database stats GET took 70 seconds, followed by requests taking 87 to 134 seconds. `database.Service.Stats` performs four whole-table logical byte aggregates per GET. The logs prove latency and a temporary probe failure; they do not prove that the stats GET alone caused the probe failure. The backend bundle was partially truncated at its archive byte limit, while the relevant current log window was present.

The screenshot shows progress from the existing background compaction analysis. `toolretention` persists the operation and `last_analysis`, so a second analysis cache is unnecessary. Its generic error banner conflates a status-read failure with an operation failure.

## Scope

### In scope

- A process-local, 15-minute logical-statistics snapshot with one background scan.
- Bounded reads and failure recovery for database status.
- Explicit pending, refreshing, stale, and unavailable states in the API and page.
- Distinct compaction status-read feedback while preserving last known analysis.
- API, metric, localization, public documentation, and desktop/phone tests.

### Out of scope

- Persisting a second snapshot in SQLite or across backend restarts.
- Changing compaction eligibility, policy, cleanup, or backup behavior.
- Automatic VACUUM, adding a new feature flag, or redesigning the settings page.

## Technical approach

### Server

In `apps/backend/internal/system/database`, split live metadata from expensive logical totals. Add a cache with a single worker, 15-minute TTL, last-good snapshot, failure backoff, and generation invalidation. Replace per-request whole-table aggregates with keyset batches using the current length expressions. Keep the four logical byte fields nullable until measured; add state and measured-time fields to `GET /api/v1/system/database`. Bound live metadata reads and preserve a last-good response during transient read failure. Wire invalidation after successful maintenance and database replacement. Expose `POST /api/v1/system/database/refresh` to bypass automatic retry backoff without running the scan in HTTP. Measure cancellation and resource release with focused tests.

### Web and operations

Update `DatabaseStats`, the API client, store, and `DatabaseStatsCard` to render live metadata independently from logical totals. Use one compact localized status line and an explicit retry state. Keep the backup directory available from the same response. Update `useToolPayloadRetention` and `RetentionError` so a status GET failure says status is unavailable, while a failed action keeps its existing message. Document the API null/state semantics, explicit retry endpoint, and logical gauge measurement time in `docs/public/cli.md`; document the page behavior in `docs/public/operations.md` if its database section needs it.

## ASCII UI preview

### UI-01: Database status while logical measurement runs

Entry: Settings > System > Data & Logs > Database. The layout shows only the changed region.

```text
Desktop and phone, cold read:
+ Database ------------------------------------+
| Driver             SQLite                    |
| Database size      4.8 GB                    |
| Last backup        Today, 09:15             |
| Measuring logical totals...                  |
| [Vacuum] [Optimize] [Factory reset]          |
+-----------------------------------------------+

Desktop and phone, cached/refreshing:
+ Database ------------------------------------+
| Driver             SQLite                    |
| Database size      4.8 GB                    |
| Last backup        Today, 09:15             |
| Logical totals measured 12:10. Updating...  |
| [Vacuum] [Optimize] [Factory reset]          |
+-----------------------------------------------+

Compaction status request fails while analysis runs:
+ Messages compaction -------------------------+
| Current status unavailable. Last known       |
| analysis is shown. [Refresh status]          |
| Last known: Analysis running at 12:22        |
| Estimated tool details reduction: 2.9 MB    |
+-----------------------------------------------+
```

Structural requirements: metadata and controls do not wait for logical totals; old measured values keep their timestamp; status-read copy does not claim the operation failed. Copy and spacing are illustrative and must be localized. Phone uses the same single-column card with wrapped text and one page scroll owner. The nearest shipped exemplar is the current Data & Logs card and `mobile-tool-payload-retention.spec.ts`; no new drawer or route is needed. A retry control uses the existing touch sizing. Maps to database AC 001.1-.5, 002.3-.4, and retention AC 003.9.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| Database 001.1-.4 | `apps/backend/internal/system/database/stats_cache_test.go`: cold read, single flight, expiry, stale success/failure, backoff |
| Database 002.1-.2 | Scanner and service tests: bounded batches, cancellation, maintenance deferral, generation invalidation, SQLite and Postgres semantics |
| Database 002.3-.4 | `apps/web/hooks/domains/system/use-database-stats.test.ts` and card tests: last-good state, nullable totals, failure recovery |
| Retention 003.7-.9 | Existing hook/card tests: 503 read failure, 200 recovery, action-error precedence, last analysis visibility |

The full ID prefixes are in work-order frontmatter. Tests must exercise behavior rather than mirror cache internals. Existing PostgreSQL test gating uses `KANDEV_TEST_POSTGRES_DSN`.

## E2E tests

Extend `apps/web/e2e/tests/system/tool-payload-retention.spec.ts` (`chromium`) and `mobile-tool-payload-retention.spec.ts` (`mobile-chrome`) for transient status-read failure and recovery with a retained analysis. Extend `database-page.spec.ts` and `mobile-database-page.spec.ts` to hold the logical scan response through a browser reload, verify metadata and controls remain visible, then show the measured timestamp and stale refresh. Use controlled routes/fixtures and causal waits. The mobile test checks wrapping, touch access, and no horizontal overflow.

## Work orders

- [x] [Task 01: Cache and bound database statistics](task-01-background-statistics.md) (done)
- [x] [Task 02: Show resilient status on desktop and phone](task-02-settings-feedback.md) (done)

Execute sequentially; Task 02 depends on Task 01's response contract. No subagents are authorized by this plan.

## Verification results

Task 01 passed:

- `cd apps/backend && go test ./internal/system/database ./internal/system/maintenance ./internal/system -count=1`
- `cd apps/backend && go run ./cmd/sqlguard ./internal`
- `cd apps/backend && go test -race ./internal/persistence/storeconformance -count=1`

The PostgreSQL DSN-gated integration was not run because no test DSN was configured.

Task 02 passed:

- Focused database and compaction hook/card Vitest suite: 29 tests.
- Web TypeScript typecheck, i18n completeness and changed-line ratchet, changed-file ESLint, and public-doc validation (62 tests, 47 pages).
- Production Vite build and backend build.
- Chromium system-page E2E suite: 9 tests; database-page spec rerun after the backup-path assertion: 5 tests.
- Mobile Chrome system-page E2E suite: 6 tests; mobile database-page spec rerun after the backup-path assertion: 2 tests.

## Risks

- The endpoint changes four logical fields from always numeric to nullable before the first complete scan. TypeScript callers and public API readers must handle that state explicitly.
- A keyset scan across concurrent writes is a sampled estimate rather than a transactional point-in-time value. The measured timestamp and later refresh expose that limit.
- Expensive work can still contend with production traffic. Batch deadlines, yielding, maintenance deferral, and failure backoff bound its impact; the exact source of the observed probe failures remains unproven.
- A process-local cache survives browser refreshes, but not a backend restart. The cold state is explicit and triggers a new scan.

## Review remediation results

The database hook revalidates on mount, polls pending/refreshing states, retries stale/unavailable states after bounded delays, refreshes an expired ready snapshot, and clears timers on unmount. Missing required scanner tables now fail the scan, so a partial run cannot replace a complete snapshot with zero totals.

Verification after review remediation: database/system Go tests, backend build, focused hook/card tests (7), web typecheck, changed-file ESLint, web build, desktop database-page E2E (5), and mobile database-page E2E (2) passed.
