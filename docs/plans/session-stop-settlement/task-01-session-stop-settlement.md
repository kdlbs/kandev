---
id: "01-session-stop-settlement"
title: "Settle stops with nothing left to stop"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-SESSION-STOP-SETTLEMENT-001
acceptance_criteria:
  - AC-TASKS-SESSION-STOP-SETTLEMENT-001.1
  - AC-TASKS-SESSION-STOP-SETTLEMENT-001.2
system_design:
  - ../../specs/tasks/system-design/session-stop-settlement.md
---

# Task 01: Settle stops with nothing left to stop

## Summary

Continue a prompt-readiness resume when the execution's teardown is already
complete, and settle an execution-less stop of a waiting or terminal session by
closing its observed open turn.

## In scope

- `reapPromptUnreadyExecution` accepts a completed exact teardown claim; the
  stop itself moves to `stopPromptUnreadyExecution`.
- `StopSession` calls `settleExecutionLessSessionStop` on
  `lifecycle.ErrNoExecutionForSession`.
- Execution-less settlement excludes lifecycle and prompt admission, rechecks
  runtime absence, and preserves active cancellation/resume ownership.
- The winning reaper completes or releases only its exact cleanup token.
- `session_stop_settle_review_test.go` owns the review regressions.
- `session_stop_settle_lock_order_test.go` owns the inverse-order launch,
  nonblocking contention, guard-reference cleanup, and cancelled-context cases.

## Out of scope

Live execution stops, cleanup arbitration, `StopSessionSynchronously`, and
background work settlement.

## Verification

- From `apps/backend`:
  `go test -trimpath -race ./internal/orchestrator -run '^(TestReapPromptUnreadyExecution|TestStopSession|TestClaimForcedExecutionCleanup|TestRegisterExecutionStopOwner|TestCleanupAgentExecution|TestHandleAgentStopped_OwnedPredecessor)' -count=1`

## Initial review verification results (before contention correction)

- Before the repair, causal regressions failed because successful reaper cleanup
  blocked later recovery, failed cleanup could not be retried, admitted prompts
  lost their reused turn, and a newly registered runtime was overlooked.
- The repaired focused race suite passed on the contributor branch (2.649s).
- The current-base merge result passed the bounded affected race suite with
  `GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -p=1 -trimpath -race` and the test
  selector above, also including `TestStopTaskRuntime`, `TestBuildStopTargets`,
  and `TestRefreshTaskRuntimeStopTargets` in `./internal/orchestrator/executor`
  and `./internal/task/service`: orchestrator 2.048s, executor 1.083s, task
  service 2.017s. A prior integration attempt exited 137 during compilation
  before producing test results.
- Catalog and specification lint passed. Public documentation validation passed
  all 63 validator tests and 54 published pages.
- Evidence uses SQLite turns and the real service admission/cleanup paths with
  a mock runtime manager. The state-read interleavings probe existing admission
  locks and invoke the production prompt claim; no native provider process,
  remote executor, browser, PostgreSQL, or live deployment was exercised.
- Parent review, normal hooks, remote CI/reviews, and actual merge remain delivery
  gates; local validation does not complete them.

## Contention correction

The initial locking candidate waited on the prompt guard while holding
lifecycle. A deterministic regression confirmed the cycle with an existing
inverse-order launcher: Stop could finish only after cancelling its context.
Settlement now tries both locks without waiting, drops every guard reference
on contention, and preserves cancelled-context errors. Same-turn admission
remains excluded through turn closure. Final affected verification is recorded
after this correction; the results above are historical.

## Final verification after contention correction

The same bounded three-package affected race command passed all 53 top-level
cases without skips or failures: orchestrator 2.077s, executor 1.085s, task
service 1.703s. This includes both the inverse-order launch regression and the
cancelled-context contract, with the previous same-turn protection preserved.
The architecture audit passed without baseline growth. The public-guide
validator results above remain applicable because that text did not change.
The runtime/provider limits and pending delivery gates above still apply.
Scoped Go lint over the four affected Go files passed with zero issues after
the final correction. Catalog and specification validation also passed.
