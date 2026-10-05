---
created: 2026-09-27
updated: 2026-10-05
status: done
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
system_design:
  - ../../specs/system-page/system-design/database-statistics-snapshot.md
legacy_specs: []
---

# Implementation Plan: Database Statistics Query Ownership

## Overview

Move only the browser snapshot and request lifecycle for database statistics
from Zustand and local hook state to TanStack Query. Preserve the existing
Data & Logs card, authenticated API routes, backend measurement lifecycle, and
every other System resource owner.

## Scope

### In scope

- Give the database-statistics response one identity-scoped Query entry shared
  by the database card and Backups description.
- Preserve lazy mount reads, revalidation, polling, explicit retry, error
  visibility, and last-good data.
- Remove the migrated database response and setter from the System slice.
- Extend the stable system query provider to clean obsolete database-statistics
  entries without clearing unrelated Query data.
- Update the architecture-maintenance tracker with the migration PR evidence.

### Out of scope

- Backend API, statistics scanner, process-local snapshot, or database
  maintenance changes.
- Query migrations for backups, disk usage, or other System resources.
- New boot-payload fields, persisted browser Query data, a Query-wide provider,
  or a generic WebSocket-to-Query bridge.
- Changes to the rendered Data & Logs interface or localized copy.

## Technical approach

- Lift `useDatabaseStats` to `DatabasePanel`, which passes one result to
  DatabaseStatsCard and the Backups description. The card must not create a
  separate observer. Keep direct card tests by supplying the result through
  props.
- Key the query by canonical API base URL, page boot ID, auth mode,
  authenticated state, and user ID. Do not add workspace scope.
- Reuse the authenticated app branch's stable QueryClient. Extend its targeted
  identity cleanup to cancel and immediately remove obsolete SystemInfo and
  database-statistics queries only.
- Pass TanStack's observer AbortSignal through the existing
  fetchDatabaseStats API wrapper. Use the existing refresh endpoint for
  explicit retry. Do not change the boot payload or transport.
- Schedule revalidation from logical_stats_state and
  logical_stats_measured_at: two seconds for pending/refreshing, 30 seconds for
  stale/unavailable/read errors, and at the ready snapshot's 15-minute expiry.
  Stop polling when there is no query observer. Disable implicit retry, focus,
  and reconnect refresh; keep offline requests in the same visible failure
  path as direct fetch.
- Keep the Query entry for the authenticated app branch lifetime. Preserve the
  last good response on refetch errors. Remove database from the System slice
  and update the web architecture guidance and migration tracker.

## Tests

- REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001, AC 001.3 and 001.4:
  use-database-stats.test.ts covers mount revalidation with cached data,
  shared reads, state-based polling, expiry, and stop-on-unmount behavior.
- REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002, AC 002.3:
  the hook tests cover boot/auth/backend identity changes, cancellation,
  last-good data after GET failure, bounded recovery polling, and explicit
  retry followed by status refetch.
- REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001, AC 001.5: the card and route
  tests preserve the maintenance controls and resolved Backups description.
- REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002, AC 002.4: phone E2E preserves
  measurement status, touch controls, readable wrapping, and the no-overflow
  layout.
- The System slice test confirms the migrated field and setter are removed
  while the remaining System state is unchanged.
- Existing database-card tests verify the prop contract and card behavior.
- `system-route-copy.test.ts` verifies that the Backups description still uses
  the shared query result's resolved `backup_directory` and remains omitted
  when the result or directory is unavailable.

## E2E tests

- AC 001.3, 001.5, and 002.3: desktop Data & Logs database status, page reload,
  explicit recovery, and available maintenance controls in
  e2e/tests/system/database-page.spec.ts on chromium.
- AC 001.5, 002.3, and 002.4: the same status and retry path on a phone in
  e2e/tests/system/mobile-database-page.spec.ts on mobile-chrome.
- `e2e/tests/system/backups-page.spec.ts` verifies the resolved backup
  directory remains visible after the query owner moves to `DatabasePanel`.

## Work orders

- [x] [Task 01: Move database statistics to the scoped Query cache](task-01-database-statistics-query.md)

## Verification results

Implemented and locally verified after refreshing onto main at
`eb589f279dc8527101b71fdaf99ce523909a5299`.
Focused tests passed (5 files, 56 tests); web typecheck and full lint passed.
Managed E2E passed for desktop database (5), desktop Backups (3), and phone database (2).
Documentation, architecture, specifications, coverage markers, harness, and diff checks passed.

## Risks

- The Query key must use the server measurement time for expiry; browser fetch
  time does not represent logical-statistics freshness.
- The shared provider cleanup must remove only old SystemInfo and database
  query entries, and must do so without remounting shell state.
- The official TanStack ESLint task
  a9b7ddc9-fce5-4e47-96f9-664ba75da0a6 merged before implementation. The branch
  was refreshed after PR review to `f45fe59cf26c49dda309a88a0fbb835ed6c2185c`.
  Before PR handoff, main advanced to `eb589f279dc8527101b71fdaf99ce523909a5299`
  with workspace-secret loading changes. The changed paths are disjoint, the
  SystemInfo Query/provider contract is unchanged, and the branch was rebased
  onto that current base before verification.
