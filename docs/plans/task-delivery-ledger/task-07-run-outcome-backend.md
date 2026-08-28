---
id: "07-run-outcome-backend"
title: "Office run outcome: column, writers and rollup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 07: Office run outcome: column, writers and rollup

The run-grain form of the same defect the ledger fixes: a terminal status is not
an outcome. `FinishRun` writes `status='finished'` on six paths, four of which
did no work, and `RunCountsByDayForAgent` counts every `finished` row as a
success.

This task is independent of the ledger and shares no files with it.

## Schema

`apps/backend/internal/office/repository/sqlite/base_migrations.go`:

```go
r.migrate.Apply("runs.outcome", `ALTER TABLE runs ADD COLUMN outcome TEXT`)
```

Also list `outcome TEXT` inline in the `runs` `CREATE TABLE`
(`internal/office/repository/sqlite/base.go:301`) so fresh databases get it. Per
`apps/backend/CLAUDE.md`, the migration is the source of truth for evolution and
must stand alone; the inline listing is a convenience for fresh databases only.

Nullable, **no default**. Pre-activation rows read `NULL` and are never
backfilled — the spec is explicit that the pre-activation series keeps its own
bucket so the discontinuity is visible in the data rather than silent.

Write the activation point once via `persistence.WriteKeyIfAbsent` (task 01):
`telemetry.run_outcome.activated_at`. If task 01 has not landed, note the
dependency and land the column first; do not use the upsert `writeKey`, which
would rewrite the instant on every boot.

Vocabulary: `processed | budget_blocked | idle_skipped | agent_inactive |
task_tree_held`, as typed constants in
`internal/office/models`.

## Writers

Add `FinishRunWithOutcome(ctx, id string, outcome models.RunOutcome) error` to
`internal/office/service/scheduler_runs.go` and the sqlite repository; leave
`FinishRun(ctx, id)` delegating with `OutcomeProcessed` so existing callers are
unchanged.

The four scheduler call sites in `internal/office/service/scheduler_integration.go`:

| Line | Path | Outcome |
|---|---|---|
| `:218` | agent not active | `agent_inactive` |
| `:247` | idle skipped | `idle_skipped` |
| `:517` | task-tree hold | `task_tree_held` |
| `:829` | pre-execution budget block | `budget_blocked` |

The two agent-completion callers (`internal/office/service/event_subscribers.go:408`
and `:512`) keep `processed`. Checkout errors use the normal retry or failure
path, and checkout contention requeues the run; neither path finishes a run or
records completed work.

`runs.status` is unchanged. A blocked run still reaches `finished`, so every
existing reader of `status` keeps working — that is what keeps this change
contained.

Note that only three of the six total paths write an `office_activity_log` row
(`run_idle_skipped`, `run_budget_blocked`, `run_processed`). The agent-inactive
and task-tree-hold paths log nothing, which is why the activity log cannot carry
this distinction and a column is needed. Taskless or unlaunchable runs fail
before a finished outcome is written.

## Rollup

`internal/office/repository/sqlite/agent_summary.go:36` —
`RunCountsByDayForAgent` returns five buckets instead of three:

```sql
SUM(CASE WHEN status = 'finished' AND outcome = 'processed'  THEN 1 ELSE 0 END) AS succeeded,
SUM(CASE WHEN status = 'finished' AND outcome IS NOT NULL
                                  AND outcome <> 'processed' THEN 1 ELSE 0 END) AS skipped,
SUM(CASE WHEN status = 'finished' AND outcome IS NULL        THEN 1 ELSE 0 END) AS unclassified,
SUM(CASE WHEN status IN ('failed','timed_out')               THEN 1 ELSE 0 END) AS failed,
SUM(CASE WHEN status NOT IN ('finished','failed','timed_out') THEN 1 ELSE 0 END) AS other
```

`AgentRunDayRow` (`:13`) gains `Skipped` and `Unclassified`. Legacy rows land in
`unclassified`, never `succeeded`.

## DTO

`internal/office/dashboard/agent_summary.go` — `AgentRunActivityDay` (`:44`)
gains `skipped` and `unclassified`; `padAgentRunActivity` (`:306`) and
`buildSuccessRate` (`:366`) both change `Total` to
`Succeeded + Skipped + Unclassified + Failed + Other`. `AgentSuccessRateDay`
keeps its `succeeded`/`total` pair.

The success rate will **fall** for agents whose runs were being blocked. That is
the fix landing, not a regression: those runs were never successes.

- **Acceptance:**
  1. Each of the six `FinishRun` paths writes its documented outcome while
     `runs.status` stays `finished`, including the agent-inactive path that
     writes no activity-log row.
  2. The migration applies to a pre-existing database, `outcome` reads NULL on
     every pre-existing row, and it replays cleanly on a second boot.
  3. A day with one processed, one budget-blocked, one failed and one
     pre-activation NULL run reports
     `succeeded=1, skipped=1, failed=1, unclassified=1`.
  4. `Total` in both DTO builders sums all five buckets.

- **Verification:**
  `cd apps/backend && go test ./internal/office/... && make lint`

- **Files likely touched:**
  - `apps/backend/internal/office/repository/sqlite/base.go`
  - `apps/backend/internal/office/repository/sqlite/base_migrations.go`
  - `apps/backend/internal/office/repository/sqlite/runs_outcome_migration_test.go` (new file, not an append — the 800-effective-line file limit applies to test files)
  - `apps/backend/internal/office/repository/sqlite/agent_summary.go`
  - `apps/backend/internal/office/repository/sqlite/agent_summary_test.go`
  - `apps/backend/internal/office/models/models.go`
  - `apps/backend/internal/office/service/scheduler_runs.go`
  - `apps/backend/internal/office/service/scheduler_integration.go`
  - `apps/backend/internal/office/service/scheduler_integration_test.go`
  - `apps/backend/internal/office/dashboard/agent_summary.go`
  - `apps/backend/internal/office/dashboard/agent_summary_test.go`

- **Dependencies:** Task 01 for `persistence.WriteKeyIfAbsent` (activation point
  only; the column and writers do not depend on it).

- **Parallelism:** parallel-safe with task 01 — disjoint files
  (`internal/office` vs `internal/db` + `internal/persistence`).

- **Inputs:** Spec **`runs.outcome` (new column)**, **Office run outcome**, and
  the **Office run outcome** scenarios; plan decision D2;
  `apps/backend/CLAUDE.md` "Schema & migrations" and code-quality limits.

- **Output contract:** summary, files changed, tests run with counts, blockers,
  risks, and task/plan status update in the same conversation. Call out the
  expected success-rate drop for affected agents so it is not read as a
  regression.

## Results

**Files changed:** `internal/office/repository/sqlite/base_migrations.go`
(`outcome` column, nullable, no default), `internal/office/repository/sqlite/run_outcome_migration_test.go`,
`internal/office/repository/sqlite/agent_summary.go` + `agent_summary_test.go`,
`internal/office/models/models.go` (`Run.Outcome *string`),
`internal/office/service/run.go` (the 5-value `RunOutcome*` vocabulary),
`internal/office/service/scheduler_runs.go` (`FinishRun`/`FailRun`/
`transitionRunTerminal`), `internal/office/service/scheduler_integration.go`
(the four non-subscriber `FinishRun` call sites), `internal/office/service/event_subscribers.go`
(the two `processed` call sites), `internal/office/dashboard/agent_summary.go`
+ `agent_summary_test.go`.

**Commands run:**
- `cd apps/backend && go test ./internal/office/...` → all 23 test-bearing
  packages `ok`, 0 fail (`internal/office/repository/sqlite`,
  `internal/office/service`, `internal/office/dashboard`,
  `internal/office/scheduler`, and 19 others; 4 sub-packages report
  `[no test files]` and are not part of this task's scope).
- `make lint` — clean.

**Acceptance verification:** #1 (each `FinishRun` path writes its
documented outcome, `runs.status` stays `finished`) — confirmed by reading
every call site directly: `scheduler_integration.go` covers
`agent_inactive`, `idle_skipped`, `task_tree_held` and `budget_blocked`
(4 of the 5 values), `event_subscribers.go` covers `processed` (the 5th, written by the
agent-completed subscriber). Review round 2's `TestFailRun_WritesFailedStatusAndNullOutcome`
(added this Build round, finding #1) additionally pins the failed path
(`status='failed'`, `outcome IS NULL`) at the service level, since
`TestFinishAndFailRun` had never actually called `FailRun`. #2 (migration
applies to a pre-existing DB, `outcome` reads NULL on every pre-existing
row, replays cleanly) — `TestRunOutcomeMigration_AddsNullableColumn`. #3
(one processed + one budget-blocked + one failed + one pre-activation NULL
run → `succeeded=1, skipped=1, failed=1, unclassified=1`) —
`TestRunCountsByDayForAgent_BucketsStatuses` and
`TestGetAgentSummary_RunActivityBuckets_SkippedAndUnclassified`. #4
(`Total` sums all five buckets in both DTO builders) — read
`internal/office/dashboard/agent_summary.go` and
`internal/office/repository/sqlite/agent_summary.go` directly: both compute
`Total = Succeeded + Skipped + Unclassified + Failed + Other`.

**Expected success-rate drop:** confirmed in the task's own framing — agents
whose runs were previously miscounted (budget-blocked, idle-skipped,
agent-inactive, task-tree-held, or never-launched runs silently landing in
`succeeded`) will show a lower `AgentSuccessRateDay`
after this lands. This is the fix taking effect, not a regression: those
runs were never real successes.

**Security/trust and external side-effects:** None — new nullable column
and read-only aggregation queries; no secrets, no network calls.

**Lower-priority item folded in (TEST-002, Review round 2):**
`TestTransitionRunTerminal_LastWriterWinsOnStatusAndOutcome`
(`internal/office/service/scheduler_runs_test.go`) drives both orderings of
two terminal callers (`FinishRun` then `FailRun`, and the reverse) on the
same run row and asserts the row always reflects the later call's status
and outcome together — pinning spec:2130-2132's "set in one statement, can
never disagree" contract, which `transitionRunTerminal`'s single
`UPDATE ... SET status = ?, outcome = ?, finished_at = ?` already
guarantees but nothing previously tested directly.

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries when
applicable, or explicitly state `None`.
