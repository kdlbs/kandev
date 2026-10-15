---
id: "02-native-convergence"
title: "Native convergence"
status: done
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

## Unproven dispatch reconciliation follow-up (PR #4519)

The SQL dispatch reconciliation path redundantly required the journal label to
be `interrupted_unknown`, rejecting authenticated `completed`, `failed`, and
`cancelled` history without terminal proof. Reuse `peerMatchesUnknownHistory`
after normalizing only the in-memory SQL candidate. This implements criterion
AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.2 without changing identity,
generation, payload hash, stream, replay-cursor, or SQL compare-and-set guards.

The follow-up owns these files:

- `apps/backend/internal/agent/runtime/lifecycle/native_resume_dispatch_reconciliation.go`
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_dispatch_reconciliation_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/native_resume_state_matrix_test.go`

Real SQLite/bbolt regression cases cover each unproven terminal label, retained
terminal evidence, conflicting hashes, live dispatches, and independent work.
They assert unchanged journal labels and terminal metadata, preserved native
identity, and SQL uncertainty. The cross-store matrix checks repeated recovery
and keeps retired instructions out of current work.

Validation uses resource-capped race tests for lifecycle `^TestNativeResume`
and the journal package, plus changed-file Go lint. CI owns browser coverage;
local E2E and production state mutation are excluded. This restores the existing
recovery contract without changing public controls, configuration, or recovery
instructions, so no public documentation update is needed.

Follow-up local validation passed: lifecycle `^TestNativeResume` race tests,
the complete journal race suite, changed-file Go lint (zero issues), specification
catalog validation and full specification lint. The documentation evaluator
reports `covered` for the complete PR delta. Current-head CI and review remain
delivery gates; these local results do not claim production recovery.

CI shard 11 exposed a test synchronization issue in
`apps/web/e2e/tests/chat/mobile-agent-goal.spec.ts`: the goal drawer's close
button measured 43.999969px while its opening animation was still active.
Wait for the drawer's finite animations before measuring its touch targets,
using the existing helper already used later in the same test. Preserve the
strict 44px minimum and retries-disabled setting. CI supplies browser validation;
this test-only correction does not change the goal UI or recovery behavior.

Shard 8 also caught the queued-prompt cancellation fixture settling its 300ms
hold before the browser inspected pending controls. Desktop and phone cases now
use the existing queue/cancellation fixture pair: an 8s provider hold and a 12s
join budget. All disabled-control, progress-label, queue-pause, same-execution,
and follow-up assertions remain; both cases run with retries disabled. The
Threads grid case in that shard passed on retry and its failure screenshot
already shows the expected two-row layout; its startup timing remains under
observation in the next CI run without a speculative production change.
