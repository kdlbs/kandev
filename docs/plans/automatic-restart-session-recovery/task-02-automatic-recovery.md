---
id: "02-automatic-recovery"
title: "Restore eligible sessions automatically"
status: done
wave: 2
depends_on:
  - "01-terminal-history"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.6
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.9
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.10
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.11
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
---

# Task 02: Restore eligible sessions automatically

## Summary

Classify historical terminal bookkeeping before automatic recovery rejects a
candidate. Recover the later incident through the existing worker and native
proof path. Publish committed readiness and one idempotent transcript notice.

## In scope

- Source classification before phase checks in restart presentation and inputs.
- Startup and runtime-replacement triggers with no open browser or task focus.
- First startup after upgrade with existing failed or stranded persisted sessions.
- Missing current notice, absent current block, and generic-block reconstruction.
- Existing native identity, source-death, full-inventory, and admission guards.
- Already-loaded idle successors and repeated recovery entry points.
- Shared committed results for Resume and state-only Retry connection, with
  actual blocked causes. Retry never schedules a restore or launches a replacement.
- Durable informational message identity and commit/publication retry for ordinary
  interrupted restarts, historical-terminal recovery, and explicit native Resume.

## Out of scope

- Rendering or translating the notice, live-instance recovery, and deployment.
- Starting an old instruction or releasing queued work during recovery.
- Widening Retry connection beyond its existing query/reconnect authority.

For AC 001.4 and 002.10, this work order covers persistence and publication only.
Task 03 covers the rendered notice, normal composer, and viewport behavior.

## Acceptance

1. Startup alone restores the later eligible incident after an earlier terminal
   incident, including a session already failed before upgrade. No new interruption,
   prompt, task focus, or manual action triggers repair. Source/native/workspace
   identity and old prompt dispatch count remain stable.
2. Automatic recovery, Resume, and Retry connection share the committed result.
   Retry remains state-only, including while restoration is pending. A ready
   process alone cannot establish successful delivery recovery.
3. Ordinary interrupted restart, historical-terminal recovery, and explicit
   native Resume publish the same canonical notice. One incident keeps one ID
   across SQL/publication interruption and repeat restoration. A distinct follow-up
   prompt is admitted after every independent gate passes.

## Deterministic notice precedent

Use `writeCeilingSurfaceNote` and `ceilingSurfaceMessageID` in
`apps/backend/internal/orchestrator/ceiling_surface.go` as the message precedent.
Use `uuid.NewSHA1`, `uuid.NameSpaceOID`, `MessageTypeStatus`, structured metadata,
and `CreateSessionMessageIdempotent`. The convergence design defines the prefix
and source tuple: session, incarnation, source harness generation, source submission.
Automatic recovery and explicit Resume of the same incident use the same ID.
A later submission uses a different ID. Current execution, action, timestamps,
and recovery revisions are excluded from the key.

Persist metadata `kind: session_restored_after_interruption` and canonical English
`content`: "The session was interrupted. Your conversation is restored. You can continue."
Task 03 translates by metadata kind. It does not parse the English content.

## Verification

Add these three named regressions. Use actual repository and journal evidence
at the proof boundary. Do not replace recovery with a successful mock.

- `automatic_restart_historical_terminal_test.go`:
  `TestAutomaticRestartAfterHistoricalTerminalRecovery` owns automatic recovery,
  upgrade, mixed inventory, cancellation, changed owner, and repeated triggers.
- `manual_recovery_historical_terminal_test.go`:
  `TestManualRecoveryAfterHistoricalTerminalRecovery` covers Resume and Retry
  reading the already committed disposition. A pending Retry cannot schedule or
  launch recovery. Explicit Resume retains its existing guarded restoration path.
- `session_recovery_notice_test.go`:
  `TestRecoveryNoticePublicationIdempotent` covers ordinary interrupted restart
  without older terminal history, historical-terminal recovery, and explicit
  native Resume including criterion 002.6. Restore one incident twice through
  different entry points and require one ID. Restore a later incident and require
  a different ID. Exercise commit/publication loss and retry. Uninterrupted live
  reattachment and Resume without an interrupted incident emit no such notice.

```bash
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/orchestrator -run '^Test(AutomaticRestartAfterHistoricalTerminalRecovery|ManualRecoveryAfterHistoricalTerminalRecovery|RecoveryNoticePublicationIdempotent|SilentRestoreRecoveryRescansOnTrigger|NativeReadinessBootSettlesOrExposesRecoveryBeforeFirstSend|NativeReadinessKeepsFenceWhenProofOrFreshCapabilityRefuses)$' -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'Test(NativeResume|LoadedNative)' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths.
Include a mixed inventory, cancellation, changed owner, and repeated-trigger case
inside `TestAutomaticRestartAfterHistoricalTerminalRecovery`.
Its upgrade cases seed persisted state before backend startup, including a failed
session and an already-loaded successor with retired journal work and an open SQL block.

## Files likely touched

- `apps/backend/internal/orchestrator/restart_recovery_presentation.go`
- `apps/backend/internal/orchestrator/silent_restart_recovery_inputs.go`
- `apps/backend/internal/orchestrator/silent_restart_recovery.go`
- `apps/backend/internal/orchestrator/session_native_delivery_recovery.go`
- `apps/backend/internal/orchestrator/session_delivery_recovery.go`
- `apps/backend/internal/orchestrator/automatic_restart_historical_terminal_test.go` (new)
- `apps/backend/internal/orchestrator/manual_recovery_historical_terminal_test.go` (new)
- `apps/backend/internal/orchestrator/session_recovery_notice_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/`, only the transaction/message
  persistence seam needed for idempotent notice publication.

## Dependencies

Task 01 supplies the historical terminal classifier and guarded write behavior.

## Risks

Do not discard errors as ordinary ineligibility after a repair attempt.
The existing recovery worker owns cancellation, capacity, and retries. Reuse its
ownership instead of adding per-reader or per-button restore goroutines.
Cancellation of one waiting browser request must not cancel backend-owned work.
An active successor or allocated candidate keeps its existing ownership.

## Parallelism

`sequential`

## Inputs

- Both referenced designs and Task 01 results.
- Existing `silent_restart_recovery_trigger_test.go`, native readiness tests,
  `loaded_native_reconciliation.go`, and lifecycle-only message conventions.
- `ceiling_surface.go`: deterministic message ID, status metadata, content fallback,
  and idempotent creation. Reuse this message pattern without its ceiling semantics.
- Existing journal-shutdown and journal-growth packages remain earlier evidence,
  not proof of this correction.

## Results

- The race-enabled orchestrator regression matrix passed for automatic and
  manual historical-terminal recovery, notice idempotence, silent-restore
  rescans, and native-readiness fences.
- The race-enabled lifecycle matrix passed for native resume, loaded-native
  reconciliation, and persisted binding selection across multiple journal
  submissions.
- The race-enabled SQLite reconstruction and historical-terminal matrix passed.
- `bash scripts/lint-go-changed` passed for all changed Go paths with 0 issues.
- `git diff --check` passed.
- Peer-review follow-up added regressions for authenticated notice publication
  after completed and settled source labels, later-incident notice creation, and
  pending-marker discovery after metadata updates. Focused race-enabled SQLite
  and orchestrator suites passed. The required PostgreSQL matrix passed all four
  suites: historical-terminal recovery, authenticated missing-binding resume,
  recovered-notice restoration after execution loss, and native resume after
  silent restore. The PostgreSQL pending-marker candidate test also passed.
- `make -C apps/backend build` passed. Changed-Go lint passed for all updated Go
  paths with 0 issues after `gofmt`. The final race-enabled SQLite and
  orchestrator checks passed.
