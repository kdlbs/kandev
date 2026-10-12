---
id: "02-native-convergence"
title: "Native convergence"
status: in_progress
wave: 2
depends_on:
  - 01-shutdown-settlement
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.4
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
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

The 49-pair lifecycle matrix and SQLite authenticated-binding suite pass.
Foreign/missing submission streams and reconstructed unknown outcomes were
reproduced before correction. Automatic loaded-successor and legacy restart checks pass, including queue-origin
and retired-history cases. Prepared checkpoint and stale composer-error SQL
controls pass. CI exposed the distinction between stored-record inspection and
retirement proof; the retained-reader correction passes its focused journal test,
and the affected established restart/gap regressions pass with race detection.
Final CI is pending.

[Parent plan](plan.md). [Requirements](../../specs/platform/requirements/durable-restart-reconciliation.md).
