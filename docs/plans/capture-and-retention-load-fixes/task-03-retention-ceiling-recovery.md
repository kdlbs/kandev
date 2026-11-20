---
id: "03-retention-ceiling-recovery"
title: "Recover retention batch ceilings and index the orphan count"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DELIVERY-RETENTION-005
acceptance_criteria:
  - AC-PLATFORM-DELIVERY-RETENTION-005.1
  - AC-PLATFORM-DELIVERY-RETENTION-005.5
  - AC-PLATFORM-DELIVERY-RETENTION-005.6
system_design:
  - ../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md
---

# Task 03: Recover retention batch ceilings and index the orphan count

## Summary

A deadline still halves a phase's candidate ceiling, and each later committed
batch in that phase doubles it back toward the configured limit. On SQLite, the
orphan phase's bounded unbound-workflow count uses its sparse partial index.

## In scope

- `task/deliveryretention/worker_batches.go`: add `recordBatchSuccess(phase)`.
  If `batchRows[phase] > 0`, set it to `reduced * 2`. When that value reaches the
  unreduced limit (`opts.BatchRows`, capped at `BackgroundBatchRows` for
  `retention`), delete the entry.
- `task/deliveryretention/worker.go` `runPassBatch`: call it in the
  `committed` branch.
- `task/repository/sqlite/agent_delivery_retirement_worker.go`
  `countUnboundWorkflowRowsTx`: on the SQLite driver, use the literal
  predicate with `INDEXED BY idx_agent_delivery_effects_unbound_workflow` and
  bind only the limit. Keep the existing bound query for PostgreSQL. This
  matches open PR #4747, which becomes superseded.

## Out of scope

Protected-only discovery accounting, discovery maxima across idle waits,
moving discovery reads off the writer, distinguishing writer-wait deadlines
from page deadlines, and offline maintenance.

## Acceptance

1. After deadlines reduce the retention ceiling from 50 to 1, successive
   committed batches request 2, 4, 8, 16, 32, then 50 candidates. A later
   deadline halves again. Busy (maintenance-gate) and failed batches do not raise
   the ceiling. Other phases keep their own limits.
2. Existing halving, protected-sweep, continuation, and productive-pass tests
   pass unchanged.
3. On SQLite, `EXPLAIN QUERY PLAN` for the count statement uses
   `idx_agent_delivery_effects_unbound_workflow` and no temp B-tree. Counts match
   the bound query for mixed bound/unbound rows. The PostgreSQL behavior test runs
   under the DSN gate.

## Verification

Write the regressions first and record their failures (ceiling stays at 1, and
the plan names `idx_agent_delivery_terminal_lookup` with `USE TEMP B-TREE`).
From the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/task/deliveryretention -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'AgentDelivery|Orphan|UnboundWorkflow' -count=1)
(cd apps/backend && go run -trimpath ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed \
  apps/backend/internal/task/deliveryretention/worker.go \
  apps/backend/internal/task/deliveryretention/worker_batches.go \
  apps/backend/internal/task/deliveryretention/worker_ceiling_recovery_test.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_retirement_worker.go \
  apps/backend/internal/task/repository/sqlite/agent_delivery_unbound_count_plan_test.go
```

If `KANDEV_TEST_POSTGRES_DSN` is configured, run the repository count test
against PostgreSQL. Otherwise record that coverage as unavailable.

## Files likely touched

- `apps/backend/internal/task/deliveryretention/worker_batches.go`
- `apps/backend/internal/task/deliveryretention/worker.go`
- `apps/backend/internal/task/deliveryretention/worker_ceiling_recovery_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/agent_delivery_retirement_worker.go`
- `apps/backend/internal/task/repository/sqlite/agent_delivery_unbound_count_plan_test.go` (new)

## Dependencies

None. After merge, close PR #4747 as superseded (with the user's approval).

## Risks

`INDEXED BY` fails instead of falling back if the index is missing. The
retention schema creates it before readiness. Keep the SQLite branch keyed on
the driver name, as other dialect branches in this package are.

## Parallelism

`sequential`

## Inputs

- [Plan](plan.md), #5400 root cause and log evidence.
- [Retention throughput design](../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md), Scheduling.
- [Protected sweep repair](../delivery-retention-protected-sweep/plan.md).

## Results

Implemented success-driven recovery of reduced retention batch ceilings and the
SQLite sparse-index query for unbound workflow counts. The PostgreSQL count
test was skipped because `KANDEV_TEST_POSTGRES_DSN` is not configured.

Verification from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/task/deliveryretention -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/deliveryretention -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'AgentDelivery|Orphan|UnboundWorkflow' -count=1)
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
```

All listed checks passed. The SQLite query-plan test confirmed use of the sparse
partial index without a temporary sort. Changed-Go lint passed across all PR Go
files; the exact invocation is included above.
