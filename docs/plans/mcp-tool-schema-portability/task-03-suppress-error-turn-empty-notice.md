---
id: "03-suppress-error-turn-empty-notice"
title: "Suppress the duplicate empty-turn notice on error-terminated turns"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-EMPTY-TURN-NOTICE-001
acceptance_criteria:
  - AC-UI-EMPTY-TURN-NOTICE-001.9
  - AC-UI-EMPTY-TURN-NOTICE-001.10
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 03: Suppress the duplicate empty-turn notice on error-terminated turns

## Summary

A turn that ends in a recoverable agent failure produces two "The agent finished
without producing any output." notices instead of none. The recoverable-failure
path completes the failed turn first (empty, notice #1), then creates the
recovery status message, which finds no active turn and lazily opens a second
turn to hold it (empty, notice #2). Make the recovery entry attach to the turn
that failed and have that turn report `had_output=true`, so no empty-turn notice
appears and no second synthetic turn is created.

## In scope

- In `handleRecoverableFailureLockedState`
  (`internal/orchestrator/event_handlers_agent.go`), capture the failed turn's ID
  before completing it and persist the recovery status message against that turn
  ID instead of letting `persistRecoveryStatusMessage` resolve `""` through
  `getActiveTurnID` (which lazily starts a new turn via `startTurnForSession`).
- Ensure the failed turn reports `had_output=true` at completion because its
  recovery/error entry is the turn's outcome. Recovery status messages
  (`MessageTypeStatus` carrying recovery/error metadata) currently do not count
  as output in `turnHadAgentOutput` (`internal/task/service/service_turns.go`);
  make the completion of an error-terminated turn report output without changing
  what counts as output for a clean empty turn.
- Preserve the clean empty-turn notice for a genuinely empty `end_turn` (for
  example an unrecognized `/command`): exactly one notice, unchanged.

## Out of scope

- Any frontend change: `computeEmptyTurnNotice` and the `status`-message renderer
  are already wired.
- Changing the completion/cancellation guards, successor-turn preservation, or
  the CI auto-fix reconciliation on this path.
- The error-detail surfacing owned by Task 04.
- Broadening what counts as agent output for a non-error turn.

## Acceptance

- A recoverable agent failure completes exactly one turn, with the recovery/error
  entry attached to that turn, and it reports `had_output=true`.
- No empty-turn notice is emitted for an error-terminated turn or for a separate
  recovery-message turn.
- A clean empty turn (no error) still emits exactly one notice with the existing
  adaptive text.

## Verification

Add failing tests first, then implement. Run from the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator ./internal/task/service -count=1)
(cd apps/backend && go test ./internal/orchestrator -run 'Recoverable|RecoveryStatus|EmptyTurn|HadOutput' -count=1)
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/task/service/service_turns.go`
- Orchestrator recovery/turn tests alongside the above
  (`event_handlers_agent_test.go` / `service_turns_test.go` or a new adjacent
  test file if the existing one is near the 800-line limit).

## Dependencies

None. Independent of Tasks 01, 02, and 04.

## Risks

Turn settlement ordering on the failure path is sensitive. Attaching the recovery
message to the failed turn must not race a Send Now / FIFO successor or a
cancellation, and must not leave a turn open. Cover the failure path plus the
existing clean empty-turn scenarios to prove exactly one notice in each case.

## Parallelism

`sequential`. Owns the recoverable-failure turn settlement and the `had_output`
computation for error-terminated turns.

## Inputs

- [Empty-turn notice requirement](../../specs/ui/requirements/empty-turn-notice.md),
  AC .9 and .10 and the error-terminated turn note.
- [Session recovery failures design](../../specs/agents/system-design/session-recovery-failures.md).
- `handleRecoverableFailureLockedState`, `createRecoveryStatusMessage`,
  `persistRecoveryStatusMessage`, `getActiveTurnID`, `completeTurnForSession`,
  `CompleteTurn`, `turnHadOutput`, `turnHadAgentOutput`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Implemented. Added `models.TurnMetaKeyErrorTerminated` (`"error_terminated"`).
`handleRecoverableFailureLockedState` now marks the active turn error-terminated
via `markTurnErrorTerminated`, captures its ID, and passes it through
`createRecoveryStatusMessage` / `persistRecoveryStatusMessage` so the recovery
entry attaches to the failed turn instead of a lazily-opened second turn.
`turnHadOutput` treats an error-terminated turn as having output, so its
completion reports `had_output=true` and no empty-turn notice is emitted. Tests:
`event_handlers_recoverable_turn_test.go` (recovery attaches to the failed turn,
turn marked error-terminated) plus turn-output assertions in the task/service
package. `go test ./internal/orchestrator ./internal/task/service
./internal/task/models` passes.
