---
id: "01-replay-retained-cancellation"
title: "Replay the retained cancellation before resume"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.7
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.11
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 01 Replay the retained cancellation before resume

## Summary

Use the existing terminal replay path when the startup worker finds a
verifiable cancellation for a stopped Worktree session. Resume the same
native conversation after the terminal effect settles.

## In scope

- Extend the existing legacy restart probe for a cancellation with an exact
  `agent_delivery` block and no `agent_delivery_recovery` snapshot. The block
  stream may be empty only when the exact authenticated submission proves it.
- Reuse the stored-submission lookup, owner and payload checks, and unresolved
  inventory checks. Require a stopped, resumable local execution.
- Reserve resume capacity before settlement. Pause the queue, run retained
  replay, and reconcile terminal effects and queue claims through the existing
  paths. Recheck all open recovery blocks before normal resume admission.

## Out of scope

- Discovery from generic blocks with no submission ID.
- Changes to clarification state, copy, or provider warnings.
- New persistence, manual recovery actions, and remote recovery routes.

## Acceptance

1. Startup with SQLite at `N` and a retained cancellation at `N+1` receives
   and projects the event once. The SQL submission becomes `cancelled` and
   its block records the terminal sequence and outcome.
2. With valid current native evidence, startup reuses the saved conversation
   and workspace. The session becomes available in Chat without a prior
   prompt call, a replacement conversation, or queue auto-run.
3. Repeated startup triggers preserve one terminal effect and one native
   launch. Missing or conflicting evidence keeps recovery blocked.
4. A received-but-unprojected cancellation replays from the projected cursor.
   Capacity refusal leaves the submission and block pending without a generic
   ceiling deferral. An independent open block prevents native launch. Terminal
   reconciliation retires the exact matching queue claim.

## Verification

First add `TestAutomaticRestartReplaysRetainedCancellation` in
`legacy_native_restart_test.go`. Reuse the stopped-session fixture with real
SQLite and a reopened journal. Use `CancelSubmission` to write the event.

The primary case uses a queue-origin submission, a saved native ID, a stopped
executor, and no recovery snapshot. Start the recovery pass without a client.
The first run on current code must leave the cursor at `N`. After the fix,
verify the terminal effect, cleared block, saved native ID, and zero prompt
calls. Repeat the startup pass to verify the same terminal effect and no
extra launch. Use a mismatched hash to verify that the block stays open.

The fixture creates the dispatch-error block through `deliveryRecoveryError`,
with an exact submission ID and empty stream, and persists a matching queue
claim. Subcases cover a conflicting stream, received-but-unprojected terminal,
an independent block, and full capacity.

Run the existing replay, legacy resume, and terminal projection tests for
the shared behavior. Use the existing desktop and phone recovery cases for
the composer.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestAutomaticRestartReplaysRetainedCancellation$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^Test(AutomaticRestartReplaysRetainedCancellation|LegacyNativeRestartWithoutRecoveryNotice|TerminalSettlementPublishesWithoutRuntimeRecoveryRecord|SilentRestartWorkerSkipsIneligibleCandidates)$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRetrySessionDeliveryWithoutExecutionReplaysRetainedTerminal$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/legacy_native_restart.go apps/backend/internal/orchestrator/legacy_native_restart_test.go
(cd apps && pnpm install --frozen-lockfile)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project chromium e2e/tests/session/automatic-restart-recovery.spec.ts -- --grep "restores an ordinary interrupted session on startup"
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --project mobile-chrome e2e/tests/session/mobile-automatic-restart-recovery.spec.ts -- --grep "restores an ordinary interrupted session on startup"
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/legacy_native_restart.go`
- `apps/backend/internal/orchestrator/legacy_native_restart_test.go`

## Dependencies

None.

## Risks

Pause the queue before settlement. Use `ResumeTaskSessionWithOptions` with
the saved native ID required and no initial prompt. This uses ordinary
admission and keeps operator-managed queue claims parked.

## Parallelism

`sequential`

## Inputs

- [Durable restart reconciliation requirements](../../specs/platform/requirements/durable-restart-reconciliation.md)
- [Retained cancellation before native resume](../../specs/platform/system-design/durable-restart-reconciliation.md#retained-cancellation-before-native-resume)
- `legacy_native_restart_test.go` for stopped native resume and queue fixtures.
- `retained_delivery_recovery_test.go` for authenticated retained replay.

## Results

Implemented retained cancellation replay before native resume in
`legacy_native_restart.go`. The probe now requires the exact open delivery
block, matching journal identity and payload hash, a stopped resumable local
Worktree execution, and a cursor that does not exceed the retained terminal.
It replays through identity-bound delivery recovery, verifies the cancelled
submission and terminal block settlement, then resumes the saved native
conversation without an initial prompt. It accepts the real empty-stream
dispatch-error block shape only when the authenticated submission proves the
stream, allows receipt to lead projection within the retained high-water
range, reserves capacity before settlement, reconciles the queue claim, and
checks recovery blocks again before launch.

`TestAutomaticRestartReplaysRetainedCancellation` covers successful replay,
repeat startup, saved conversation reuse, zero prompt calls, and a mismatched
hash that remains blocked. It also covers the independent-block guard,
capacity refusal without a deferral, duplicate receipt after a crash between
receipt and projection, rejection of a conflicting stream, and retirement of
the matching queue claim.

Verification passed:

- Focused Go regression and the requested orchestrator race test group.
- Retained-terminal lifecycle race test.
- `make -C apps/backend build`.
- Frozen-lockfile pnpm install.
- Desktop Chromium and mobile Chrome restart recovery E2E cases, one test each.
- `git diff --check`.
- The changed-file lint wrapper could not start during the review follow-up
  because a `golangci-lint` process was already running in another worktree.
  An earlier 10-minute changed-package lint run completed with zero issues
  before these final review repairs.
