---
created: 2026-10-27
status: implemented
requirements:
  - REQ-TASKS-DAILY-ACTIVITY-001
system_design:
  - ../../specs/tasks/system-design/daily-activity-statistics.md
legacy_specs: []
---

# Implementation Plan: Overnight Activity Statistics

## Overview

Correct the workspace daily-activity message projection and retain existing
turn/task and presentation semantics. One sequential work order proves the
registered route with independently authored persisted fixtures before changing
the query, then verifies both dialects where the established facility exists.

## Scope

### In scope

- Attribute eligible messages by their own UTC creation day, independent of
  turn starts, using separate message and turn aggregates.
- Preserve existing workspace, ephemeral and automation eligibility, date
  spine, response contract, authorization, and read-only behavior.
- Update only the legacy omission expectations in aggregate parity coverage.

### Out of scope

- Calendar-label and timezone presentation, frontend layout or copy,
  completion metrics, lifecycle, data migration, or unrelated statistics.

## Technical approach

In `GetDailyActivity` in `apps/backend/internal/analytics/repository/sqlite/stats.go`,
replace the turn-derived message-day scope with an independently grouped
message/session/task/date scope. Retain dialect helpers, rebinding, existing
turn scope, and analytics admission and error handling. `StatsHandlers` and
`ActivityHeatmap` already carry and display the corrected data on desktop and
phone without a UI change. No new UI preview applies.

## Tests

- `TestDailyActivityOvernightRegisteredRoute` in a new
  `apps/backend/internal/analytics/handlers/stats_daily_activity_integration_test.go`:
  real full-schema SQLite, real task writers and registered HTTP route. Cover
  overnight and older-than-horizon turns, same-day positive control, a new
  same-session turn that does not change message count, workspace and excluded
  tasks, duplicate turns, range edges, zero-fill and ordering
  (`AC-TASKS-DAILY-ACTIVITY-001.1` through `.4`, `.6`). Establish expected RED
  from the two omitted messages before changing the query.
- `TestStatsAggregateParity` in `stats_aggregate_parity_test.go`: retain all
  aggregates, update the day-before count from 2 to 3 and the no-turn-day count
  from 0 to 1. Its PostgreSQL branch is environment-gated and uses a non-UTC
  connection (`.1` through `.4`).
- `TestStatsRepositoryAndDailyAggregatesAvoidTurnsMessagesProduct`: keep its
  existing 200-turn/10,000-message, 100ms bound and exact counts (`.3`).
- Existing analytics handler authorization and repository tests verify the
  preserved route/access and admission behavior (`.4`, `.6`).

## E2E tests

The registered route integration exercises persistence through the actual HTTP
consumer. Existing `apps/web/e2e/tests/layout/stats-calendar-labels.spec.ts`
(`chromium`) exercises the existing heatmap date and tooltip with stubbed data.
`mobile-stats-calendar-labels.spec.ts` (`mobile-chrome`) covers the adjacent
completed chart, not the activity heatmap. Shared heatmap source establishes
the desktop/phone count-to-intensity and tooltip mapping (`.5`); it is not a
live phone rendering proof. No new browser test or UI change is needed for
this backend-only attribution correction.

## Work orders

- [x] [Task 01: Correct message attribution](task-01-correct-message-attribution.md)

## Verification results

The independently authored registered-route test first failed for both missing
message cases while its controls passed, then passed after the query correction.
The owning handler and SQLite analytics repository packages passed with race,
including SQLite parity and the unchanged 200-turn/10,000-message bounded
check. The PostgreSQL parity subtest skipped because no authorized DSN was
configured. Changed-Go lint passed with its default 3 GiB limit. The work order
records the commands and remaining document checks.

## Risks

- Filtering messages through turns, or raw turn/message fanout, silently drops
  or multiplies counts. Preserve the independent bounded aggregate check.
- PostgreSQL timezone and date semantics require live dialect evidence when an
  existing authorized DSN is available; do not invent a database or credentials.

## Required CI test precondition repair

The first hosted E2E retry failed in the mobile conflict auto-fix case when
the automation drawer was no longer available after an incidental help-tooltip
Escape sequence. A later retry repeated the same test failure. The CI cause
remains unknown: the failure trace was unavailable and unchanged isolated and
worker-prefix runs passed. The test-only follow-up moves the help-text assertion
to the mobile controls test and opens the conflict drawer through its existing
UI helper after prompt setup. Conflict and deduplication assertions remain.

A single 13-case mobile worker prefix passed with one worker, retries disabled,
and tracing enabled against the unchanged verified build. Scoped ESLint passed
for the changed test. The subsequent hosted run passed the conflict case.

A later hosted E2E retry failed a different mobile sidebar recovery case when
the retry button measured 43.999969 px against the unchanged 44 px minimum;
the Playwright retry passed. The exact cause is unknown. The test now waits for
finite drawer animations before measuring, matching its neighboring recovery
case, while retaining the exact size and recovery assertions. Focused local
verification passed both recovery cases with one worker, no retries and tracing
enabled against the existing verified build; scoped ESLint passed. This does
not establish the hosted failure's cause.
