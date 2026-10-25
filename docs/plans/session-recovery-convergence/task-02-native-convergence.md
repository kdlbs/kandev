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

## Projection barrier follow-up (PR #4621)

The native Resume browser regression exposed a setup `session_mode` event that
was received before its canonical projection. Recovery proof could also capture
a cursor behind the peer's journal high-water mark. Retain the admission fence
and wait for complete projection before returning proof. Revalidate advancing
stream evidence under the same owner; reject changed ownership and honor caller
cancellation. The shared wait is bounded to 10 seconds, polls every 200 ms after
verified lag or monotonic same-owner stream progress, and issues evidence reads
only, without repeating Resume or sending an old prompt.

The new lifecycle tests reproduced both incomplete snapshots before the change.
Focused proof race tests and the wider native recovery regressions passed,
including cancellation, owner changes, and advancing acknowledgments. All six
desktop and six phone browser scenarios passed without retries. Changed Go
package lint passed with zero issues. Public controls and recovery instructions
are unchanged.

```bash
cd apps/backend
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p 2 ./internal/agent/runtime/lifecycle -run 'Test(NativeResume|AcknowledgedUnknown|LoadedNative)' -count=1
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -p 1 -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestNativeResumeProof' -count=1
```

Claude requested explicit byte-accounting coverage in the proof-status table.
Both byte-only increase and decrease cases failed before the correction. Retry
now requires a monotonic sequence-counter advance. Byte usage can decrease during
valid pruning, so byte monotonicity is not required. The existing pruning case
now covers that decrease. Focused proof race tests, changed-package Go lint
(zero issues), catalog validation, and specification lint passed.

Main introduced the shared projection-wait owner in PR #4623. Conflict resolution
uses that owner, deadline, and polling cadence. It retains the journal high-water
check and requires sequence progress rather than byte changes for a retry.
The acknowledgement regression covers progress after verified projection lag.
The scrolling regression uses main's observer collection
and keeps the reader-position and disabled-follow shrink cases.

The ported proof-status test rejected unchanged status and byte-only changes
after the sequence guard was restored. Native proof/projection race tests,
196 focused frontend tests, TypeScript, changed-package Go lint (zero issues),
changed-file ESLint, catalog validation, and full specification lint passed.
Managed Playwright runs passed four desktop and four phone cases with retries
disabled, covering projection success/timeout, delayed scrolling, and retained
failure details.

```bash
GOMAXPROCS=2 GOMEMLIMIT=2GiB go test -C apps/backend -trimpath -p 1 -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestNativeResume(Proof|Projection)' -count=1
```

The outside-diff review finding also covers progress on the first proof attempt.
That result now enters the shared wait without requiring an earlier lag result.
The new settling-progress test and the continuous-progress deadline case both
failed before the correction. Only authenticated monotonic sequence progress can
start this wait; byte-only changes and other evidence failures still return.

The focused lifecycle and caller race checks passed after the correction.
Changed-package Go lint reported zero issues. Both desktop and both phone
projection-recovery cases passed with retries disabled after rebuilding the
runtime. Catalog validation and specification lint passed. Existing screenshots
remain accurate because this follow-up changes no frontend source or layout.
