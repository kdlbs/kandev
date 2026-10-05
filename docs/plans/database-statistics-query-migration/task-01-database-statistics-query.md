---
id: "01-database-statistics-query"
title: "Move database statistics to the scoped Query cache"
status: in_progress
wave: 1
depends_on: []
updated: 2026-10-05
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.3
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.4
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.4
system_design:
  - ../../specs/system-page/system-design/database-statistics-snapshot.md
---

# Task 01: Move database statistics to the scoped Query cache

## Summary

Give the Data & Logs database panel one identity-scoped owner for the database
statistics snapshot and GET lifecycle. Its one `useDatabaseStats` result feeds
both DatabaseStatsCard and the Backups description that displays
`backup_directory`. Preserve the existing mutable freshness, retry, error, and
presentation behavior while removing the duplicate Zustand snapshot and local
request effects.

## In scope

- Replace the database-statistics store read/write and local load/poll state with
  one TanStack Query entry.
- Include the canonical API base URL, page boot ID, auth mode, authenticated
  state, and user ID in the query identity. Do not scope the resource by
  workspace.
- Pass TanStack's AbortSignal to the existing status GET.
- Lift the hook to `DatabasePanel` and pass its result to DatabaseStatsCard and
  the Backups description, so the two consumers share one observer and one
  response. Update direct card tests to supply the hook result through props.
- Keep the existing two-second pending/refreshing poll, 30-second stale,
  unavailable, and read-error recovery interval, and ready-snapshot expiry
  based on logical_stats_measured_at.
- Keep the explicit reload and POST-then-GET retry behavior, including the
  current non-throwing UI caller contract and preservation of last-good data.
- Extend provider cleanup to cancel and immediately remove obsolete database
  query entries with the same targeted identity filter used for SystemInfo.
- Remove the database field and setter from Zustand, update their tests, web
  architecture guidance, and migration tracker.

## Out of scope

- Backend endpoint, scanner, snapshot, or maintenance changes.
- Changes to the Data & Logs layout, copy, routes, or boot payload.
- Migrations of backup, disk-usage, retention, jobs, metrics, or other System
  state.
- A generic query ownership rule or WebSocket-to-Query bridge.

## Acceptance

1. TanStack Query is the only browser owner of database-statistics data and GET
   request state. The query key includes backend URL, page boot ID, and auth
   identity; Zustand no longer exposes the database response or setter.
2. Mount revalidation, pending/refreshing polling, stale/unavailable recovery,
   measured-time expiry, last-good data after errors, explicit reload/retry,
   and GET cancellation preserve the system-design contract.
3. Identity changes cancel and immediately remove only obsolete SystemInfo and
   database-statistics queries without remounting unrelated shell state. Both
   the database card and Backups description use the shared result, and the
   resolved backup-directory description remains visible. Endpoint
   permissions, desktop behavior, and phone behavior remain intact.
4. Desktop and phone users retain the same status and timestamp. Phone status
   text wraps, maintenance controls remain touch-sized, and the page has no
   horizontal overflow.

## Verification

```bash
mkdir -p /root/tmp/kandev-query-02
export TMPDIR=/root/tmp/kandev-query-02
(cd apps/web && pnpm exec vitest run \
  hooks/domains/system/use-database-stats.test.ts \
  hooks/domains/system/use-system-info.test.tsx \
  lib/state/slices/system/system-slice.test.ts \
  components/settings/system/database-stats-card.test.tsx \
  components/settings/system/system-route-copy.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/system/database-page.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/system/backups-page.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/system/mobile-database-page.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/list-docs.py decisions --format paths
python3 scripts/lint-spec-files.py --all
python3 scripts/lint-architecture.py --all
python3 scripts/lint-harness-files.test.py
python3 .github/scripts/lint-harness-files.py --all
pre-commit run harness-lint --files apps/web/AGENTS.md
git diff --check
```

The PR's `PR documentation coverage` check validates the changed work order and
its links to requirements, acceptance criteria, and system design on the exact
head because that check requires GitHub pull-request context.

## Files likely touched

- apps/web/hooks/domains/system/use-database-stats.ts
- apps/web/hooks/domains/system/use-database-stats.test.ts
- apps/web/components/settings/system/data-logs-settings.tsx
- apps/web/components/settings/system/database-stats-card.tsx
- apps/web/components/settings/system/database-stats-card.test.tsx
- apps/web/components/settings/system/system-route-copy.test.ts
- apps/web/e2e/tests/system/database-page.spec.ts
- apps/web/e2e/tests/system/backups-page.spec.ts
- apps/web/e2e/tests/system/mobile-database-page.spec.ts
- apps/web/hooks/domains/system/system-info-query.ts
- apps/web/components/system-info-query-provider.tsx
- apps/web/lib/state/slices/system/types.ts
- apps/web/lib/state/slices/system/system-slice.ts
- apps/web/lib/state/slices/system/system-slice.test.ts
- apps/web/AGENTS.md
- docs/specs/platform/system-design/system-info-query-cache.md
- docs/specs/system-page/system-design/database-statistics-snapshot.md
- docs/decisions/2026-09-27-database-stats-query-cache-ownership.md
- docs/architecture-maintenance/README.md
- docs/architecture-maintenance/server-state-migrations.md

## Dependencies

The official TanStack ESLint PR #4012 (task
a9b7ddc9-fce5-4e47-96f9-664ba75da0a6) merged into main at
a1e2edadb9cd40a08d23a0f9b72665146ec3fea5. The branch was refreshed to
3328fe887f0e9ea2ffb11a00c4c5d0a94a7b88ed before final local verification, and
the current Query/provider contracts, database-statistics requirements/design,
and scoped lint guidance were reread. No material contract or ownership drift
was found. The reviewed package's implementation checkpoint is satisfied by
the user's explicit request.

## Risks

- Use logical_stats_measured_at for mutable snapshot expiry. Query fetch time
  alone can make an old backend snapshot look fresh.
- Cancel and immediately remove obsolete identity entries; do not defer cache
  removal to a cancellation promise.
- Preserve query errors and last-good data separately so a failed background
  read does not hide database metadata or maintenance controls.

## Parallelism

sequential

## Inputs

- Database statistics snapshot requirements and system design.
- Database statistics Query ownership ADR.
- Architecture maintenance server-state migration tracker.
- Existing SystemInfo Query implementation, database card, hook, and tests.

## Results

Pending implementation.
