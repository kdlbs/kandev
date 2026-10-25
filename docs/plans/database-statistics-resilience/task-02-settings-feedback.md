---
id: "02-settings-feedback"
title: "Show resilient status on desktop and phone"
status: done
wave: 2
depends_on:
  - "01-background-statistics"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001
  - REQ-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003
acceptance_criteria:
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-001.5
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.3
  - AC-SYSTEM-PAGE-DATABASE-STATS-SNAPSHOT-002.4
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.7
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.8
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-003.9
system_design:
  - ../../specs/system-page/system-design/database-statistics-snapshot.md
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 02: Show resilient status on desktop and phone

## Summary

Render live database metadata and the logical-snapshot state independently. Clarify that a failed compaction status read does not mean the running operation failed. Cover full page refresh, desktop, and phone behavior.

## In scope

- Update frontend types, store, hook, and cards for nullable logical totals and freshness states.
- Distinguish compaction status-read error from action and persisted-operation errors.
- Complete six-language copy, public API and operations docs, focused unit tests, and desktop/phone E2E.

## Out of scope

- New compaction cache, policy behavior, or new settings navigation.

## Acceptance

1. A cold or stale scan leaves database metadata, backup path, and permitted controls visible; the status line gives measurement time when available.
2. A transient compaction status GET failure keeps the last analysis and says current status is unavailable. Recovery clears that read error while preserving action failures.
3. Desktop and phone browser flows cover page reload, loading, stale/failure feedback, and touch/overflow behavior.

## ASCII UI preview

UI-01: Database status while logical measurement runs. See the [full plan preview](plan.md#ascii-ui-preview). This excerpt covers database AC 001.5, 002.3-.4 and retention AC 003.9.

```text
+ Database ------------------------------------+
| Driver             SQLite                    |
| Database size      4.8 GB                    |
| Logical totals measured 12:10. Updating...  |
| [Vacuum] [Optimize] [Factory reset]          |
+-----------------------------------------------+
+ Messages compaction -------------------------+
| Current status unavailable. Last known       |
| analysis is shown. [Refresh status]          |
| Last known: Analysis running at 12:22        |
+-----------------------------------------------+
```

Phone keeps the same one-column card and page scroll owner. Text wraps; touch actions meet the existing 44px rule. The nearest shipped exemplar is Data & Logs and its mobile retention spec. Copy and spacing are illustrative and must be localized.

## Verification

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/system/use-database-stats.test.ts hooks/domains/system/use-tool-payload-retention.test.ts components/settings/system/database-stats-card.test.tsx components/settings/system/tool-payload-retention-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/system/tool-payload-retention.spec.ts tests/system/database-page.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-tool-payload-retention.spec.ts tests/system/mobile-database-page.spec.ts)
```

## Files likely touched

- `apps/web/lib/types/system.ts`
- `apps/web/hooks/domains/system/use-database-stats.ts`
- `apps/web/components/settings/system/database-stats-card.tsx`
- `apps/web/hooks/domains/system/use-tool-payload-retention.ts`
- `apps/web/components/settings/system/tool-payload-retention-card.tsx`
- `apps/web/src/locales/*/system.json`
- `apps/web/e2e/tests/system/` (existing and new focused specs)
- `docs/public/cli.md`
- `docs/public/operations.md`

## Dependencies

Task 01's response contract must be available before frontend and E2E integration.

## Risks

- Translation completeness can fail if new status keys are missing in any supported locale.
- A generic error mapping must not hide a failed mutation or persisted operation.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/system-page/requirements/database-statistics-snapshot.md) and [retention requirements](../../specs/system-page/requirements/tool-payload-retention.md).
- [Design](../../specs/system-page/system-design/database-statistics-snapshot.md) and [retention design](../../specs/system-page/system-design/tool-payload-retention.md).
- Task 01's finalized API response.

## Results

Implemented the nullable logical-statistics contract in the frontend types and
rendered pending, ready, refreshing, stale, and unavailable states separately
from live database metadata. Stale metadata includes its measurement time, and
stale or unavailable totals expose a retry action while retaining the backup
path and permitted maintenance controls. Compaction status-read errors now use
separate feedback from failed actions; successful reads clear only the read
error and keep the last analysis visible.

Added focused database hook/card tests, compaction hook/card regression tests,
desktop and phone E2E coverage, translations for all supported locales, and
public API and operations documentation.

Verification passed:

- `pnpm exec vitest run hooks/domains/system/use-database-stats.test.ts hooks/domains/system/use-tool-payload-retention.test.ts components/settings/system/database-stats-card.test.tsx components/settings/system/tool-payload-retention-card.test.tsx` (29 tests).
- `pnpm run typecheck`.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`.
- Chromium E2E: 9 system-page tests, followed by a 5-test database-page rerun after adding the backup-path assertion.
- Mobile Chrome E2E: 6 system-page tests, followed by a 2-test database-page rerun after adding the backup-path assertion.
- `pnpm run build:vite`, `make -C apps/backend build`, changed-file ESLint, and public-doc validation (62 tests, 47 pages).
- Backend database/system package tests, SQL guard, and the store-conformance race suite are recorded in Task 01 results; the backend was also built after the final backend change.

The PostgreSQL DSN-gated integration was not run because no test DSN was
configured.

## Review remediation

The database hook now revalidates on mount, polls pending and refreshing scans every two seconds, retries status errors and stale or unavailable responses every 30 seconds, and revalidates at the 15-minute snapshot expiry. Scheduled polling stops on unmount. The hook test changes a mocked API response from pending to ready while the hook stays mounted, verifies stale recovery after backend backoff, and covers expiry refresh.

Verification after the review fix:

- `cd apps/web && pnpm exec vitest run hooks/domains/system/use-database-stats.test.ts components/settings/system/database-stats-card.test.tsx` (7 tests).
- `cd apps/web && pnpm run typecheck` and changed-file ESLint.
- `cd apps/web && pnpm run build:vite`.
- `cd apps/web && pnpm e2e:run --project chromium tests/system/database-page.spec.ts` (5 tests).
- `cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-database-page.spec.ts` (2 tests).

## Additional PR review remediation

The retry action now POSTs to the database refresh endpoint before reading the
current scan state, so it can bypass automatic server backoff without running
the scan in the request. A failed status read schedules the bounded retry even
when the store retains a recent ready snapshot. The retention design now states
that Refresh status clears only its read error; a new user action clears both
error channels and a failed action records its new failure.

Verification after these fixes:

- `cd apps/web && pnpm exec vitest run hooks/domains/system/use-database-stats.test.ts components/settings/system/database-stats-card.test.tsx lib/api/domains/system-api.test.ts` (41 tests).
- `cd apps/web && pnpm run typecheck` and changed-file ESLint.
- `cd apps && pnpm --filter @kandev/web build:vite`.
- Desktop Chromium database page E2E suite (5 tests).
- Mobile Chrome database page E2E suite (2 tests).
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`.
