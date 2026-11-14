---
id: "12-close-pr-gates"
title: "Close the PostgreSQL and flaky-test gates"
status: cancelled
wave: 6
depends_on:
  - "11-pr-scope-evidence"
plan: "plan.md"
requirements:
  - REQ-COSTS-USAGE-HISTORY-005
acceptance_criteria:
  - AC-COSTS-USAGE-HISTORY-005.2
  - AC-COSTS-USAGE-HISTORY-005.5
system_design:
  - ../../specs/costs/system-design/usage-history.md
---

# Task 12: Close the PostgreSQL and flaky-test gates

## Summary

Complete the gates #3660 already records as open: the full five-year PostgreSQL benchmark on the final head, and zero `FLAKY` verdicts in current-head CI. The new adoption step and toggle must be part of what is measured.

## In scope

- Find why the C16 run ended with exit 143, and run the gate where its fixture build can finish.
- Fix query causes of the New York, Kathmandu, and Lisbon five-year views and deleted-task reporting if they still fail.
- Reproduce and fix the mobile task colors and mobile port forwarding flakes.
- Rerun the SQLite gate on the final head if report SQL changed.

## Out of scope

- Changing fixtures, oracle, sample counts, or query deadlines to make the gate pass.

## Acceptance

- The full PostgreSQL gate passes all scenarios on the final head with unchanged fixtures, oracle, samples, and deadlines. Record measured values in the `docs/plans/usage-history` plan.
- Current-head CI finishes with zero `FLAKY` verdicts.

## Verification

Use the benchmark procedure recorded in `docs/plans/usage-history/plan.md` for the C12 and C16 runs, without changing its inputs. The CI audit uses:

```bash
scripts/pr-state --summary 3660
scripts/pr-await 3660
```

## Files likely touched

- `apps/backend/internal/analytics/repository/sqlite/usage_history_report_source.go`
- `apps/backend/internal/analytics/repository/sqlite/usage_history_report_fastpaths.go`
- `apps/web/e2e/tests/` files for the two flaky cases
- `docs/plans/usage-history/plan.md`

## Dependencies

Task 11.

## Risks

- This work order stays `in_progress` until every benchmark bound passes. An earlier or partial run does not count.
- The gate needs a host with enough memory and time. Exit 143 suggests something terminated the earlier run, which may be environmental.

## Parallelism

`sequential`

## Inputs

- [Usage History plan](../usage-history/plan.md), the C12 and C16 results.
- `.agents/skills/pr-fixup/SKILL.md` for CI handling.

## Results

Pending.

Superseded on 2026-11-01. Moved to [Tasks 04 and 05](../usage-history-merge-readiness/plan.md).
