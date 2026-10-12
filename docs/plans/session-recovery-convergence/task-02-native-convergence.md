---
status: in_progress
---

# Task 02: Native convergence

## Outcome and scope

Shared evidence assessment, guarded legacy retirement, authenticated missing
stream binding and atomic notice/fence resolution. Own lifecycle/native_resume*,
journal retirement, process native retirement, repository/sqlite/native_resume*
and affected orchestrator seams. Depends on task 01. Preserve old terminal
labels, unknown SQL, native conversation and all event cursors.

## Contract

REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002, criteria .2 through .4.
[Design](../../specs/platform/system-design/session-recovery-convergence.md).

## Acceptance

- All active SQL records match authenticated peer evidence before any retirement.
- Legacy failed/no-terminal versus SQL unknown retires without normalization or replay.
- Empty bound stream with absent/stale resolved metadata converges under authenticated exact proof; rollback and conflicts preserve the fence.

## Check

From apps/backend, with GOMEMLIMIT=768MiB GOMAXPROCS=2, run sequentially:
`go test -p 1 -race -trimpath -tags fts5 ./internal/agentctl/journal ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'NativeResume|Acknowledged|Retirement|Retire' -count=1`
`go test -p 1 -race -trimpath -tags fts5 ./internal/task/repository/sqlite ./internal/orchestrator -run 'NativeResume|GenericNative' -count=1`
PostgreSQL variants run in CI against real PostgreSQL.

## Results

Pending.

[Parent plan](plan.md). [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
