---
id: "01-retire-orphaned-activity"
title: "Retire orphaned activity on execution-less stop"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-STOP-SETTLEMENT-001
acceptance_criteria:
  - AC-TASKS-SESSION-STOP-SETTLEMENT-001.2
  - AC-TASKS-SESSION-STOP-SETTLEMENT-001.3
system_design:
  - ../../specs/tasks/system-design/session-stop-settlement.md
---

# Task 01: Retire orphaned activity on execution-less stop

## Summary

When a stop finds no live execution and settles, it now also retires the
in-memory background activity of the dead executions and settles their open
durable workloads. This clears a stuck `active_subagent_count` without
weakening any successor or liveness fence.

## In scope

- `executionIDsWithActivity(sessionID) []string` in `turn_activity.go`: a
  locked read of distinct non-empty IDs from `background`, `tools`, and
  `executionClaims`.
- In `settleExecutionLessSessionStop`, after a successful
  `settleExecutionLessSessionTurn` and while the guards are still held, create
  a bounded cleanup context with `backgroundWorkCleanupContext(ctx)`. Use it
  for each `retireExecutionActivityAndPublish` call and for durable settlement.
- Pass the session's task ID to the publication. For a missing session row,
  retire with an empty task ID and skip publication.
- Add a new test file, `session_stop_settle_background_test.go`.

## Out of scope

Claude `asyncTasks` capability, live-execution stops,
`StopSessionSynchronously`, registrations without an execution ID, and
age-based clearing.

## Acceptance

- Write a regression test that registers a subagent for `dead-exec` and calls
  `StopSession` on a `WAITING_FOR_INPUT` session with no execution. It must
  fail before the fix (count stays 1) and pass after it (count 0).
- With the fake settler, open rows are settled only when the probe reports no
  live process. A live or failing probe leaves them open.
- If the request context is cancelled after turn closure is confirmed, the
  activity publication remains deadline-bounded and the durable background row
  still reaches `ended`.
- Refused settlement (an `IDLE` state, contention, or a newly registered
  execution) leaves registrations and rows unchanged. The existing
  execution-less stop suites stay green.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestStopSession|TestReapPromptUnreadyExecution|TestOwnedTeardown|TestIdleReclaimSettlesOpenBackgroundWork|TestActiveSubagentCount|TestRetireExecutionActivity|TestExecutionIDsWithActivity|TestExecutionLessSettlement)' -count=1)
(bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_stop_settle.go apps/backend/internal/orchestrator/turn_activity.go apps/backend/internal/orchestrator/session_stop_settle_background_test.go)
(python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all)
```

## Files likely touched

- `apps/backend/internal/orchestrator/session_stop_settle.go`
- `apps/backend/internal/orchestrator/turn_activity.go`
- `apps/backend/internal/orchestrator/session_stop_settle_background_test.go` (new)

## Dependencies

None.

## Risks

- Lock order with prompt admission. Run the lock-order suite under `-race`.
- `settleExecutionLessSessionTurn` may need to return the session to stay
  within function-length limits.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/session-stop-settlement.md` (AC .2, .3)
- `docs/specs/tasks/system-design/session-stop-settlement.md`, "Orphaned
  background activity"
- `docs/specs/agents/system-design/background-work.md`, process-end settlement
- Fixtures: `newExecutionLessStopService` in
  `session_stop_settle_review_test.go`; the settler fakes in
  `background_work_settlement_paths_test.go`

## Results

- RED confirmed: the activity regression failed before the production change
  because `ActiveSubagentCount` remained 1.
- RED confirmed for request cancellation: canceling after turn closure left
  the open durable row `running`; the regression now proves it reaches `ended`.
- RED confirmed for publication bounds: post-cancellation activity publication
  had no deadline; the regression now proves the read has a cleanup deadline.
- The same bounded cleanup context now covers activity publication and durable
  settlement, so a slow session read cannot hold stop guards past its deadline.
- `go test -trimpath ./internal/orchestrator -run '^TestStopSession_ExecutionLessSettlementSettlesRowsAfterRequestCancellation$' -count=1` passed. It verifies a deadline on the post-cancellation publication read and an `ended` durable row.
- `go test -trimpath ./internal/orchestrator -run '^(TestStopSession_ExecutionLessSettlementRetiresDeadExecutionActivity|TestStopSession_ExecutionLessSettlementSettlesOpenBackgroundRows|TestStopSession_ExecutionLessRefusalKeepsActivity|TestExecutionIDsWithActivityIncludesDistinctOwnerSources|TestExecutionLessSettlementRetiresActivityWithoutSessionRow)$' -count=1` passed.
- After the bounded-context fix, `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestStopSession|TestReapPromptUnreadyExecution|TestOwnedTeardown|TestIdleReclaimSettlesOpenBackgroundWork|TestActiveSubagentCount|TestRetireExecutionActivity|TestExecutionIDsWithActivity|TestExecutionLessSettlement)' -count=1)` passed.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run '^(TestExecutionIDsWithActivityIncludesDistinctOwnerSources|TestExecutionLessSettlementRetiresActivityWithoutSessionRow)$' -count=1)` passed.
- After the bounded-context fix, `go build -trimpath ./internal/orchestrator` and `make -C apps/backend build-kandev` passed.
- `GOMEMLIMIT=6GiB GOMAXPROCS=4 GOFLAGS=-p=4 bash scripts/lint-go-changed apps/backend/internal/orchestrator/session_stop_settle.go apps/backend/internal/orchestrator/turn_activity.go apps/backend/internal/orchestrator/session_stop_settle_background_test.go` passed with 0 issues.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed.
