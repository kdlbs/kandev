---
created: 2026-11-20
status: done
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-PLATFORM-DELIVERY-RETENTION-005
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md
legacy_specs: []
---

# Implementation Plan: Multi-repository status, start capture, and retention load fixes

## Overview

One PR fixes three backend defects reported from the same desktop install
with multi-repository Worktree tasks:

- [#5398](https://github.com/kdlbs/kandev/issues/5398): every `details=wait`
  status read on a bare multi-repository root logs `git status failed` at ERROR
  with a stack trace (about 1,400 records a day).
- [#5399](https://github.com/kdlbs/kandev/issues/5399): turn-change start capture
  runs checkouts sequentially in one 10-second budget. When the budget runs out,
  endpoint persistence runs on the expired context. The whole change set is lost
  (20-45% of prompts), and the real capture failures are dropped from the log.
- [#5400](https://github.com/kdlbs/kandev/issues/5400): delivery retention halves
  a phase's batch ceiling on every deadline and never raises it again. At a
  ceiling of 1 row, the protected-only sweep runs 5 to 26 back-to-back passes per
  idle cycle (about 5 million writer transactions a day). The orphan count also
  misses its sparse index on SQLite.

The three work orders are independent and touch different packages. They are
ordered by size and run sequentially.

## Evidence and root cause

### #5398: bare root treated as a detail failure

`WorkspaceTracker.GetGitStatusWithDetails`
(`agentctl/server/process/workspace_git_status.go:207-209`) returns
`errGitStatusDetailsUnavailable` for any `DetailState` other than `ready` or
`pending`. A tracker with `gitIndexPath == ""` correctly produces
`StatusState=unavailable`, `ErrorCode=repository_unavailable`,
`DetailState=unavailable` with a nil error, both in `captureBasicGitStatus`
(`workspace_git_status_capture.go:219`) and in the live path
(`workspace_git_status.go:481`). The details-wait branch converts this settled
answer into an error. `handleGitStatus` (`server/api/git.go`) then calls
`logGitStatusFailure`, which logs at ERROR. The backend client calls
`/api/v1/git/status?fresh=true&details=wait` without `repo`, so
`GetWorkspaceTrackerFor("")` resolves to the bare root. Backend callers
(`git_handlers.go`, `review_change_source.go`, orchestrator snapshot and base
commit capture) already tolerate a failed result. The defect is the agentctl
classification and log level, not the callers.

### #5399: expired admission context discards captured checkouts

`Coordinator.Admit` (`task/changes/coordinator.go`) wraps admission in
`turnChangeAdmissionTimeout` (10 s). `captureStart` captures every checkout
sequentially, then calls `AcceptTurnChangeSetStart` with that same context.
When the deadline expires during checkout N, the remaining captures fail at
once and `AcceptTurnChangeSetStart` fails at `BeginTxx` with
`context deadline exceeded`. The rollback discards rows for checkouts that were
captured, and the error path returns only `persist turn change start
endpoints: %w`, without `captureFailures`. The reporter's ref inventory shows
140 of 147 failed sets stopped partway through the loop, so the time is spent
in Git capture, not in the SQLite writer.

This violates AC-TASKS-TURN-CHANGES-003.1 ("Bounded capture failure shall allow
dispatch with affected history marked unavailable") and the current design:
"Persist failed checkout reasons; dispatch proceeds" and "Git capture
concurrency: 2 per executor; 1 per actual checkout". The implementation captures
with a concurrency of 1 and loses unaffected history.

### #5400: one-way batch ceiling decay and an unindexed orphan count

`Worker.recordBatchFailure` (`task/deliveryretention/worker_batches.go`) sets
`batchRows[phase] = max(1, previous/2)` on every `context.DeadlineExceeded`.
Nothing writes a larger value. Only `New` starts with a fresh map. Most
observed deadlines are writer waits (`begin agent delivery retention batch`),
which a smaller page cannot fix. At 1 row, the designed one-pass protected sweep
(about 236 batches) becomes 12-18 thousand single-row writer transactions. Each
pass reports forward discovery, so the worker continues without an idle wait.
The throughput design states the ceiling "never raises", so the design itself
is incomplete (classification 4). It is amended in this package.

`countUnboundWorkflowRowsTx`
(`task/repository/sqlite/agent_delivery_retirement_worker.go:374-386`) binds
`effect_type` as a parameter. SQLite cannot prove the partial index predicate
of `idx_agent_delivery_effects_unbound_workflow` from a bound parameter, so it
scans every `workflow.on_turn_complete` effect and sorts them inside the writer
transaction (23-49 ms against 0.2 ms with the partial index). It is the most
common orphan-phase deadline. Open PR
[#4747](https://github.com/kdlbs/kandev/pull/4747) proposes the same query
change. This package absorbs that change, and #4747 should be closed as
superseded when this PR merges.

## Requirement and design reconciliation

| Issue | Classification | Criteria | Design change |
| --- | --- | --- | --- |
| #5398 | 1: implementation violates an active criterion | AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26 (non-Git parent folders are not failed repositories), .001.21 | [Workspace Git status](../../specs/platform/system-design/workspace-git-status.md): `details=wait` on a repository-less tracker returns the settled `repository_unavailable` result without an error-level log |
| #5399 | 1: implementation violates an active criterion and the current design | AC-TASKS-TURN-CHANGES-003.1, .003.6 | [Turn changed files](../../specs/tasks/system-design/turn-changed-files.md): per-execution concurrent capture within the existing bound, 6-second persistence reserve, joined failure reporting |
| #5400 | 4: design incomplete | AC-PLATFORM-DELIVERY-RETENTION-005.1, .005.5, .005.6 | [Retention throughput](../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md): ceiling recovers on committed batches; literal-predicate orphan count |

No requirement text changes. No new requirement, ADR, schema, runtime flag,
UI, or public documentation change is needed.

## Scope

In scope: the agentctl details-wait classification, coordinator start capture
and persistence budget, retention ceiling recovery, and the SQLite orphan count.

Out of scope, with reasons:

- Backend callers requesting per-repository status (`repo=<name>`) for
  multi-repository tasks (#5398 option b). Callers already handle the
  unavailable result. Rerouting them changes what they compute and is a
  separate behavior change.
- Downgrading `context_canceled` status failures (#5398, 393 a day). These are
  real-repository waits whose caller gave up. They need separate evidence.
- The absent-scope variant of #5399 (`checkout ... is absent from registered
  repository scopes`). It has a different root cause (a repository added to a
  running task is not registered as an agentctl tracker). The reporter suggests
  a separate issue.
- Sweeping `refs/kandev/turn-changes/.../start` refs left by sets whose start
  was never accepted (#5399 option e). After this fix, accepted starts persist
  their refs with cleanup intent. Historical leftover refs need a separate
  compaction design.
- Treating protected-only discovery as unproductive, keeping discovery maxima
  across idle waits, or moving discovery reads off the writer (#5400 options
  b-d). With the ceiling restored, the sweep returns to its designed one-pass
  cost. Those options change the 005.5/005.6 scheduling contract.

## Technical approach

1. **Task 01 (#5398).** In `GetGitStatusWithDetails`, when the observed status
   is `StatusState=unavailable` with `ErrorCode=repository_unavailable`, return
   it with a nil error. `handleGitStatus` and the per-repository path then
   serialize it through `gitStatusResult`, so the response keeps
   `success=false`, `status_state=unavailable`, and
   `error_code=repository_unavailable`, and no failure is logged. The backend
   client keeps returning its existing error for a non-ready result, so callers
   do not change.
2. **Task 02 (#5399).** Split `captureStart` into a concurrent capture step and
   an acceptance step.
   - Derive a capture context whose deadline is the admission deadline minus
     `turnChangeStartPersistenceReserve` (6 s). If the parent has no deadline,
     use the parent unchanged.
   - Capture checkouts with at most `turnChangeStartCaptureConcurrency` (2)
     concurrent `CaptureTurnCheckpoint` calls per execution, shared across
     overlapping admissions, which matches the design bound.
     Write each row into its checkout's index so row order is unchanged. Checkouts
     absent from scopes skip capture as they do today.
   - Call `AcceptTurnChangeSetStart` with the admission context, which still has
     the reserve.
   - On acceptance failure, return the persistence error joined with every
     capture failure.
3. **Task 03 (#5400).**
   - In `worker_batches.go`, add `recordBatchSuccess(phase)`. When a phase has a
     reduced ceiling, it doubles that ceiling, and it deletes the entry when the
     result reaches the unreduced limit. Call it from the successful branch of
     the batch runner that already counts `CommittedBatches`.
   - In `countUnboundWorkflowRowsTx`, use a SQLite-only query with the literal
     `effect_type = 'workflow.on_turn_complete'` predicate and
     `INDEXED BY idx_agent_delivery_effects_unbound_workflow`. Keep the bound
     query on PostgreSQL.

## Tests

| Task | Regression (fails before the fix) | Boundary |
| --- | --- | --- |
| 01 | `TestGitStatusDetailsWaitBareRootReturnsRepositoryUnavailable` (process) and `TestHandleGitStatusDetailsWaitBareRootDoesNotLogFailure` (API, observed zap core, `newMultiRepoStatusServer`) | agentctl tracker and HTTP handler |
| 02 | `TestCoordinatorStartPersistsCapturedCheckoutsWhenBudgetExpires`, `TestCoordinatorStartCapturesCheckoutsConcurrently`, `TestCoordinatorStartPersistFailureReportsCaptureFailures` | coordinator with fake client and real SQLite repository |
| 03 | `TestWorkerBatchCeilingRecoversAfterCommittedBatches`, `TestUnboundWorkflowCountUsesSparseIndex` (`EXPLAIN QUERY PLAN`) | worker with fake repository; real SQLite |

Existing suites stay green, especially `worker_batches_test.go` (halving),
`worker_protected_sweep_test.go`, `worker_continuation_test.go`,
`coordinator_start_failure_test.go`, and the progressive details-wait tests.
These are backend-only fixes with no rendered UI change, so no browser test is
added. The turn-changed-files E2E coverage of unavailable summaries remains
the user-facing evidence.

## Work orders

- [x] [Task 01: Settle details-wait on repository-less trackers](task-01-bare-root-details-wait.md) (done)
- [x] [Task 02: Persist captured start endpoints within the admission budget](task-02-start-capture-budget.md) (done)
- [x] [Task 03: Recover retention batch ceilings and index the orphan count](task-03-retention-ceiling-recovery.md) (done)

Dependency order: none. Run them sequentially in the listed order. The PR
closes #5398, #5399, and #5400.

## Verification

Each work order lists its exact commands. Package-wide, from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/process ./internal/agentctl/server/api ./internal/task/changes ./internal/task/deliveryretention ./internal/task/repository/sqlite)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/agentctl/server/process/workspace_git_status.go \
  apps/backend/internal/agentctl/server/process/workspace_git_status_bare_root_test.go \
  apps/backend/internal/agentctl/server/api/git_status_bare_root_test.go \
  apps/backend/internal/task/changes/coordinator.go \
  apps/backend/internal/task/changes/coordinator_fakes_test.go \
  apps/backend/internal/task/changes/coordinator_start_budget_test.go \
  apps/backend/internal/task/changes/coordinator_start_capture.go \
  apps/backend/internal/task/changes/start_capture_limiter.go \
  apps/backend/internal/task/deliveryretention/worker.go \
  apps/backend/internal/task/deliveryretention/worker_batches.go \
  apps/backend/internal/task/deliveryretention/worker_ceiling_recovery_test.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_retirement_worker.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_unbound_count_plan_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks

- **Concurrent capture load (Task 02).** Two concurrent captures add Git
  pressure on an already loaded host. The shared per-execution limiter bounds
  capture at 2 across overlapping admissions, and the per-git-dir lock still
  serializes checkouts that share a common Git directory.
- **Reduced capture window (Task 02).** Capture now ends at 4 s instead of 10 s
  to leave a 6-second persistence reserve. Two captures can improve throughput
  for multiple checkouts, but a single checkout needing more than 4 s now
  becomes unavailable instead of being captured. Before this fix, writer
  contention could cause a late capture to lose the whole set.
- **Ceiling oscillation (Task 03).** Under sustained writer contention, the
  ceiling can alternate between halving and doubling. Each failure is still
  bounded by the 250 ms batch budget and existing backoff, which is strictly
  better than staying at one row.
- **SQLite `INDEXED BY` (Task 03).** If the index is missing, the statement
  fails instead of falling back. The index is created by the retention schema
  that `requiredstores` enforces before readiness. A schema-replay test covers it.

## Results

All three work orders are complete. The combined package test run passed for
the agentctl process and API packages, task change capture, delivery retention,
and SQLite task repositories. Focused regression tests were rerun after the
final lint fixes. The backend build passed.

Additional passing checks:

- Race tests for the affected agentctl process/API, task changes, and lifecycle
  packages.
- Delivery-retention race tests and persistence store conformance race tests.
- SQLite delivery query and query-plan tests, plus the SQL guard.
- Changed-Go lint, spec validation, documentation validation, and
  `git diff --check`.
- PR-fixup regression checks for the shared per-execution start-capture bound,
  SQLite persistence reserve, and the CI-discovered concurrent fake race; the
  local PR documentation coverage evaluator also passed.

The PostgreSQL-gated repository count test was not run because
`KANDEV_TEST_POSTGRES_DSN` is not configured.
