# C8 full-normal benchmark: independent failure review

Reviewed 2026-10-31T02:33:44Z. The detached worktree stayed on `5b79da2f41f9a6e2c9bd9f421313276981e95c00` with tracked and staged source clean. The runner changed only the five authorized path/frame variables; its original script, adapted script, and exact diff are archived in this evidence directory.

## Result

The normal SQLite gate exited **1** after Go reported **3618.666 seconds**. It was not killed or stopped by a Go timeout (`-timeout=0`). The five full-scale cases failed at the warm in-process `fetchUsageBenchReport` call, before isolated child queries, timed samples, query plans, or scenario JSON could be written:

- `BenchmarkUsageHistoryFiveYears/five_years/five_years/America/New_York` — `usage_history_bench_run_test.go:94: analytics busy: context deadline exceeded` (SQLite log line 81)
- `BenchmarkUsageHistoryFiveYears/five_years/five_years/Asia/Kathmandu` — `usage_history_bench_run_test.go:94: analytics busy: context deadline exceeded` (SQLite log line 87)
- `BenchmarkUsageHistoryFiveYears/five_years/five_years/Europe/Lisbon` — `usage_history_bench_run_test.go:94: analytics busy: context deadline exceeded` (SQLite log line 93)
- `BenchmarkUsageHistoryFiveYears/five_years/native` — `usage_history_bench_run_test.go:94: analytics busy: context deadline exceeded` (SQLite log line 99)
- `BenchmarkUsageHistoryFiveYears/five_years/deleted_task` — `usage_history_bench_run_test.go:94: analytics busy: context deadline exceeded` (SQLite log line 109)

The one-year phase has all **16** ten-sample scenario receipts. The five-year phase has **11 of 16**. Those **27** reports remain in `sqlite/`; the four-zone recent-before/after comparison also remains. No missing scenario JSON was fabricated. The runner's final evidence validator did not run because the SQLite command returned nonzero.

## Independent receipt checks

- All 27 available reports parse, contain ten latency samples, match the expected phase fixture counts, and use the same publication for plain and instrumented reads.
- Recomputed order-statistic p50/p95/p99 values match every available report.
- All four recent comparisons have the same selection digest before and after older history is added, different publication IDs, and ten samples in both phases.
- Full-scale receipts report 1,000,005 native observations, 100,013 external observations, 1,100,017 canonical facts, and 24,147,398,656 fixture bytes including SQLite, WAL, and shared-memory files.
- Available five-year p95s range from 2905943 to 4869356779 ns; instrumented SQLite visits range from 23 to 5501968 per report. These measurements exclude the five failed cases.
- The fixture's SQLite plans, scan loops, latency, process RSS, native SQLite memory, fixture shape, and hardware remain in each successful receipt. The failed cases have no plan evidence.

The harness calls `validateUsageBenchResult` for the warm response and each timed response before writing a scenario receipt (`apps/backend/internal/analytics/repository/sqlite/usage_history_bench_run_test.go:93-114`). Thus the existing successful receipts correspond to embedded oracle assertions passing. Actual result rows are not serialized, so this independent review checks receipt integrity and those assertions' execution path; it does not independently recompute report values.

## SQLite plan review

The saved five-year recent UTC plan reports 3,797 instrumented visits and p95 12.281 ms across 13 read statements. Its plans use the report-calendar index, scan the date-bounded 314-row `usage_report_range`, and look up canonical facts through the composite scope/generation/build/fact index. The captured plan does not show a full canonical-facts scan for this recent case.

The five-year full UTC plan reports 438,690 visits, p95 560.918 ms, and 26 read statements. It covers 18,276 daily summary rows in the full calendar range. The mixed-actor case reports 5,501,968 visits and p95 4.869 s; its saved plans use `idx_usage_contribution_points` and the projection-contribution and canonical-fact indexes, with visits of about 2.20M contribution-point rows, 2.20M projection-contribution rows, and 1.10M canonical-fact rows. These are measured completed cases; they do not supply query plans for the five timeouts or imply a latency acceptance threshold.

## Missing evidence and limits

PostgreSQL was not started after the SQLite failure, as directed. There is no PostgreSQL 18.6 version observation, `EXPLAIN (ANALYZE, BUFFERS)`, or performance receipt. Task 05 requires SQLite and PostgreSQL evidence and explicitly says skipped PostgreSQL is not a pass (`docs/plans/usage-history/task-05-retained-reporting.md:64-68`; `docs/specs/costs/system-design/usage-history.md:342-347,413-414`). Overall acceptance is **not met**.

The fixture was created below `testing.T.TempDir()` (`apps/backend/internal/analytics/repository/sqlite/usage_history_bench_fixture_test.go:91`) and was removed by normal Go cleanup when the command exited. No database snapshot was retained, and no backup was attempted after exit. The preserved full-scale JSON, progress checkpoints, resource chronology, and logs are the available diagnosis evidence.

No further benchmarks or source changes were made. The runner, Go test, SQLite test binary, and observer were quiescent at 2026-10-31T02:33:44Z.
