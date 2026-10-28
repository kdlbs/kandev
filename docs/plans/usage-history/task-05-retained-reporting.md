---
id: "05-retained-reporting"
title: "Serve retained statistics with bounded SQL"
status: pending
wave: 5
depends_on: ["04-daily-projection"]
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-001
  - REQ-COSTS-USAGE-HISTORY-002
  - REQ-COSTS-USAGE-HISTORY-003
  - REQ-COSTS-USAGE-HISTORY-005
  - REQ-COSTS-USAGE-HISTORY-006
  - REQ-COSTS-USAGE-HISTORY-007
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-001.1
  - AC-COSTS-USAGE-HISTORY-001.2
  - AC-COSTS-USAGE-HISTORY-001.3
  - AC-COSTS-USAGE-HISTORY-002.3
  - AC-COSTS-USAGE-HISTORY-002.5
  - AC-COSTS-USAGE-HISTORY-003.2
  - AC-COSTS-USAGE-HISTORY-003.7
  - AC-COSTS-USAGE-HISTORY-005.1
  - AC-COSTS-USAGE-HISTORY-005.2
  - AC-COSTS-USAGE-HISTORY-005.3
  - AC-COSTS-USAGE-HISTORY-005.4
  - AC-COSTS-USAGE-HISTORY-005.5
  - AC-COSTS-USAGE-HISTORY-005.6
  - AC-COSTS-USAGE-HISTORY-006.5
  - AC-COSTS-USAGE-HISTORY-006.6
  - AC-COSTS-USAGE-HISTORY-006.7
  - AC-COSTS-USAGE-HISTORY-007.4
  - AC-COSTS-USAGE-HISTORY-007.5
  - AC-COSTS-USAGE-HISTORY-007.6
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 05: Serve retained statistics with bounded SQL

## Summary

Move statistics and saved canonical reads to retained facts and daily summaries.
Show preserved amounts after deletion and verify recent-range performance against years of history.

## In scope

- Apply SQL date predicates before loading facts. Use indexed daily summaries for charts and canonical detail for task/session reads.
- Push provider filtering, grouping, stable sorting, pagination, and full totals into database queries.
- Use scoped existence/freshness metadata and provider dimensions without repeated whole-history scans.
- Preserve calendar-boundary partial coverage, nullable rates, distinct currencies, and separate lifetime totals.
- Use UTC/source-calendar summaries when available. For other calendars, aggregate indexed timed canonical facts against bounded UTC day intervals.
- Add backward-compatible DTOs for deleted links, actor attribution, projection state, and large integer transport.
- Update current Token Usage tables/details, Copy Stats, localized strings, and saved-value preparation/error behavior.
- Add deterministic native/external deletion, scope isolation, rebuild, and desktop/phone browser flows.
- Add five-year fixtures, query-plan assertions, and benchmark evidence for SQLite and real PostgreSQL.
- Include nonmaterialized browser calendars, DST and fractional offsets in the same evidence, without source-observation reconciliation.
- Update relevant public statistics/retention docs. Cut reads over only after Task 02 and Task 04 watermarks prove readiness.

## Out of scope

- New org-wide dashboard, user-management roles, new pricing, and plugin process changes.

## Acceptance

- A recent chart reads date-bounded summaries, with no raw-history pairwise pass. Filters, pagination, totals, and provider options agree.
- Deleted tasks retain totals without broken links. Mixed/unknown attribution, preparing history, and stale values remain explicit on both viewports.
- Record visited rows, query plans, latency, memory, and fixture/hardware details for five years. Do not substitute skipped PostgreSQL evidence.

## ASCII UI preview

UI-H01/UI-H02/UI-H03 are defined in the [full preview](plan.md#ascii-ui-preview).

```text
Desktop tasks table:
  Deleted task          exact-model-id   120,000 tokens   $1.20 estimated
  Archived live task >  exact-model-id    40,000 tokens   $0.40 estimated

Phone tasks list:
  [Deleted task]
  120,000 tokens / $1.20 estimated
  [Details] -> model, provider, user attribution, all categories, calendar

First build: Preparing usage history.
Later rebuild: Keep saved values visible. Updating history...
```

Deleted rows have no navigation link. Phone has one body scroll owner, safe-area spacing, and 44px touch controls.
Use existing focused details and desktop control density. All labels are localized.
Covers 001.1-.3, 002.3, 003.2, 006.5-.6, and 007.4-.6.

## Verification

Run from the Kandev root. Install dependencies first in a fresh worktree.
Managed browser runs are sequential and rebuild the actual frontend/backend.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -race -trimpath ./internal/analytics/... ./internal/plugins/... -run 'TestUsageHistory|TestSessionUsage' -count=1)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/... -run 'TestUsageHistoryPostgres' -count=1)
(cd apps/backend && go test -trimpath ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkUsageHistoryFiveYears' -benchtime=10x -benchmem)
(cd apps/backend && : "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}" && go test -trimpath ./internal/analytics/repository/sqlite -run '^$' -bench '^BenchmarkUsageHistoryPostgresFiveYears' -benchtime=10x -benchmem)
(cd apps/web && pnpm exec vitest run app/stats && pnpm run typecheck && pnpm run i18n:check)
pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/layout/stats-usage-history.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --project mobile-chrome e2e/tests/layout/mobile-stats-usage-history.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --project auth e2e/tests/auth/usage-history-scope.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/{usage.go,usage_history_queries*.go,usage_history_bench_test.go}`
- `apps/backend/internal/analytics/{handlers/stats_handlers.go,models/usage.go,dto}`
- `apps/backend/internal/plugins/host_data.go` and saved canonical readers
- `apps/web/app/stats/token-usage-*`, `lib/api/domains/stats-api.ts`, `lib/types/http.ts`, and locale catalogs
- New `apps/web/e2e/tests/{layout,auth}/*usage-history*` files
- Relevant files under `docs/public`

## Dependencies

Task 04, including completed migration and projection watermarks from preceding tasks.

## Risks

Response pagination does not prove database work is bounded. Capture query plans and visited rows.
Provider/day summaries must not merge currencies, calendars, or unknown actors into misleading complete values.
Existing direct task/session readers must not bypass retained-scope authorization after deletion.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/costs/requirements/usage-history.md), listed ACs.
- [Design](../../specs/costs/system-design/usage-history.md), SQL reads, Ownership, Presentation, and projection-generation contract.
- [Plan](plan.md), test/E2E matrix and UI-H01 through UI-H03.
- Existing `stats_reference_bench_test.go`, Token Usage UI tests, and stats authorization fixtures.

## Results

Pending.
