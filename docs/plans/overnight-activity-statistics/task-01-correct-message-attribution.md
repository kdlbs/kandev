---
id: "01-correct-message-attribution"
title: "Correct message attribution"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-DAILY-ACTIVITY-001
acceptance_criteria:
  - AC-TASKS-DAILY-ACTIVITY-001.1
  - AC-TASKS-DAILY-ACTIVITY-001.2
  - AC-TASKS-DAILY-ACTIVITY-001.3
  - AC-TASKS-DAILY-ACTIVITY-001.4
  - AC-TASKS-DAILY-ACTIVITY-001.5
  - AC-TASKS-DAILY-ACTIVITY-001.6
system_design:
  - ../../specs/tasks/system-design/daily-activity-statistics.md
---

# Task 01: Correct message attribution

## Summary

Make the daily activity query count eligible persisted messages on their own
UTC dates independently of turn start. Prove the public route with an
independent regression and preserve the existing aggregation constraints.

## In scope

- Write an independent real-schema SQLite route test using actual task
  repository `CreateWorkspace`, `CreateTask`, `CreateTaskSession`,
  `CreateTurn`, and `CreateMessage`, and `RegisterStatsRoutes`. Decode the
  actual route JSON response. Do not access the private qualification fixture.
- Verify previous-day and older-than-horizon ongoing turns each produce a
  message-only current bucket; same-day output counts, foreign workspace,
  ephemeral and automation messages are excluded, multiple turns do not
  multiply messages, and adding a same-day turn leaves messages unchanged.
  Include inclusive start and current-day edges, outside-horizon messages,
  ordered zero-filled buckets, exact turn/task counts, and persisted readback
  before/after the GET. Use UTC-relative fixture dates safely across midnight.
- Run the regression before the fix and record its expected failure, then
  correct the query with independent aggregates and update only the two
  obsolete daily parity expectations. Preserve the high-fanout bound.
- Exercise existing PostgreSQL parity with its non-UTC timezone if an existing
  authorized `KANDEV_TEST_POSTGRES_DSN` is available; otherwise record skip.

## Out of scope

UI changes, completion statistics, calendar labels, lifecycle changes,
new database infrastructure, and unrelated cleanup.

## Acceptance

- The registered route's independent numeric oracle fails for the two intended
  missing-message cases before the correction and passes afterward; controls
  retain their expected values.
- The corrected query keeps turn and message aggregation separate, honors the
  existing filters and UTC range, and performs no writes. SQLite parity and
  existing bounded high-fanout tests pass; live PostgreSQL outcome is recorded.
- The shared desktop/phone heatmap consumes the unchanged DTO; no UI changes
  are required. Update both plan and work-order results and promote the paired
  specs only after all required verification passes.

## Verification

Run sequentially from the repository root after the parent grants the heavy
lease. Run the focused regression before the query edit (expected RED) and
again afterward. Set `KANDEV_TEST_POSTGRES_DSN` only if an already authorized
isolated PostgreSQL service is available; record a skip otherwise.

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -p=1 -count=1 ./internal/analytics/handlers -run '^TestDailyActivityOvernightRegisteredRoute$')
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB go test -trimpath -tags fts5 -race -p=1 -count=1 ./internal/analytics/handlers ./internal/analytics/repository/sqlite)
bash scripts/lint-go-changed apps/backend/internal/analytics/repository/sqlite/stats.go apps/backend/internal/analytics/repository/sqlite/stats_aggregate_parity_test.go apps/backend/internal/analytics/handlers/stats_daily_activity_integration_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/overnight-activity-statistics
git status --short -- docs/specs/tasks docs/plans/overnight-activity-statistics
```

The lint wrapper defaults to 3 GiB and two CPUs. Include any additional
changed Go files in its invocation. Record the PostgreSQL subtest status, and
run the same race command with an existing authorized DSN if available.

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/stats.go`
- `apps/backend/internal/analytics/repository/sqlite/stats_aggregate_parity_test.go`
- `apps/backend/internal/analytics/handlers/stats_daily_activity_integration_test.go`
- This work order, its plan, and the paired requirement and design lifecycle.

## Dependencies

None. A separate explicit implementation request and the parent's heavy lease
are required before implementation and verification respectively.

## Risks

A test-only schema or a same-day turn can conceal the defect; use actual
persisted writers, an in-range message without a same-day turn, and exact
numeric controls. A skipped PostgreSQL subtest is not a PostgreSQL pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/daily-activity-statistics.md)
- [System design](../../specs/tasks/system-design/daily-activity-statistics.md)
- Existing `stats_aggregate_parity_test.go` and real-schema task repository
  integration fixtures; ROOT's separate qualification receipt is evidence,
  not a source of tests.

## Results

- Registered-route regression: RED before the query change, with both
  overnight and older-than-horizon message counts zero instead of one; all
  controls passed. GREEN afterward with the same focused command.
- The documented two-package Go race command passed, including the route,
  SQLite aggregate parity, and unchanged high-fanout bound. PostgreSQL parity
  skipped: `KANDEV_TEST_POSTGRES_DSN` was not configured.
- Changed-Go lint passed using the canonical wrapper and default 3 GiB limit.
- The frozen workspace install passed with the pinned pnpm 9.15.9. An initial
  invocation selected Corepack's pnpm 12 and failed its version check before
  installation; placing the pinned binary first on PATH fixed the invocation.
- Specification catalog validation, specification lint and its tests,
  documentation coverage preflight, and diff/status checks passed.

## Required CI test precondition repair

The hosted E2E conflict auto-fix case twice failed after its incidental tooltip
and Escape sequence; the CI cause is unknown. Mobile help-text coverage moved
to the controls case. The conflict case sets its prompt override before opening
the drawer through the existing UI helper, checks the unique visible scoped
controls and uses the normal switch tap. Conflict feedback, round count,
single-message deduplication, and cleanup assertions remain unchanged.

- `pnpm --dir apps/web e2e:run --host --shards 1 --no-build --project
  mobile-chrome` with the 13-case CI worker prefix, `--retries=0 --trace=on`:
  13 passed (1.4m); this single worker run omits the other three concurrent CI
  processes and does not establish the original failure cause.
- `pnpm --dir apps/web exec eslint
  e2e/tests/pr/mobile-ci-automation-options.spec.ts`: passed.
- Existing backend binary, web entry point, and fixture plugin identity hashes
  matched before and after. The subsequent hosted run passed the conflict case.

The later hosted retry failed the separate mobile sidebar page-recovery case:
its retry button measured 43.999969 px against the exact 44 px minimum, and
the Playwright retry passed. The cause is unknown. That case now awaits the
existing finite-animation helper before measurement, as its neighboring case
already does; the size, failed request, retry and recovery assertions remain
unchanged. Both recovery cases passed with one worker, no retries and tracing
enabled against the existing verified build; scoped ESLint passed. This local
result does not establish the hosted failure's cause.
