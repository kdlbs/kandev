---
id: "04-wire-engine-routed-producers"
title: "Wire engine-routed producers (P2, P3) onto wave identity"
status: pending
wave: 3
depends_on: ["01-wave-identity-primitives", "02-wave-identity-persistence"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-004
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 04: Wire engine-routed producers (P2, P3) onto wave identity

## Summary

Open the narrow engine seam that lets `OnChildrenCompletedPayload` carry the
two encodings from trigger dispatch through to the inserted run, then wire
P2 (`queueChildrenCompletedRun`) and P3 (`ParentWakeReconciler.reconcileOne`)
to read the wave members and populate that payload. `wakeOperationID`
(P2/P3's existing operation id) is unchanged.

## In scope

- `engine.OnChildrenCompletedPayload` gains `WaveKey`, `WaveString string`.
- `engine.QueueRunRequest` (adapters.go) gains the same two fields.
- `QueueRunCallback.Execute`: type-assert `in.Payload.(OnChildrenCompletedPayload)`
  and copy the two fields onto the `QueueRunRequest` it builds, when present.
  `idempotencyKey`/`queueActionDigest` are unchanged.
- `runsServiceEngineAdapter.QueueRun` (`backendapp/main.go`) copies the two
  fields into the `runs/service.QueueRunRequest` it constructs.
- P2 (`event_subscribers.go`, `queueChildrenCompletedRun`): after
  `AreAllChildrenTerminal`, add the `ListWaveMembers` terminality
  confirmation (queue nothing on a non-terminal member, an empty set, or a
  read error); build `OnChildrenCompletedPayload` with `WaveKey`/`WaveString`
  from the sorted member ids. `childSetKey`/`wakeOperationID` computation is
  unchanged.
- P3 (`scheduler_wake_reconciler.go`, `reconcileOne`/`buildPayload`): same
  addition. The existing `GetChildSetKey`/`GetChildSetKeyTx` re-reads that
  guard the receipt write are unchanged (state-inclusive key, different
  purpose).

## Out of scope

- P1 (Task 03) and P4 (Task 05).
- `ListStuckParents` (Task 06).
- Any change to `wakeOperationID`, `formatChildSetKey`, or
  `parent_child_wake_receipts` semantics (AC-...-004.6).

## Acceptance

- A run queued via P2 for a parent whose children are all terminal carries
  the correct `WakeWaveKey`/`WakeWaveString` on the persisted row.
- A run queued via P3 for the same parent and wave derives the identical
  `WakeWaveKey` as P2 would for that parent (byte-for-byte), so
  `idx_run_wake_wave` actually collapses a P2/P3 race for the same wave
  (proven fully in Task 07; this task's own test proves derivation parity,
  not the race itself).
- A wave-member read observing a non-terminal member (P2 or P3) queues
  nothing and dispatches no engine trigger.
- `TestWakeOperationID_UnifiedAcrossEdgeAndReconcilerPaths` continues to
  pass unmodified.

## Verification

```bash
cd apps/backend
go test ./internal/workflow/engine/... -run TestQueueRunCallback -v
go test ./internal/office/service/... -run TestQueueChildrenCompletedRun -v
go test ./internal/office/service/... -run TestParentWakeReconciler -v
go test ./internal/office/service/... -run TestWakeOperationID_UnifiedAcrossEdgeAndReconcilerPaths -v
```

## Files likely touched

- `internal/workflow/engine/payloads.go`
- `internal/workflow/engine/adapters.go`
- `internal/workflow/engine/phase2_callbacks.go`
- `internal/workflow/engine/phase8_test.go` (or a new test file for the
  `QueueRunCallback` wave-key copy)
- `internal/backendapp/main.go`
- `internal/office/service/event_subscribers.go`
- `internal/office/service/scheduler_wake_reconciler.go`
- `internal/office/service/scheduler_wake_reconciler_test.go`

## Dependencies

Task 01 (`waveidentity`, `ListWaveMembers`), Task 02 (persistence, so the
copied fields land somewhere real).

## Risks

- **Shared engine-seam files.** This task and Task 05 both touch
  `payloads.go`/`phase2_callbacks.go`'s type assertion; land this one first
  so Task 05 builds on an existing seam rather than duplicating it.

## Parallelism

`sequential`

## Inputs

- System design: "Producers" (P2 edge, P3 backstop), "The engine seam is
  narrow" paragraph under "The wave-member read is the last read, and it
  carries state".
- `internal/workflow/engine/engine.go:417-443` (`executeCallback`) —
  confirms `Payload any` already flows `HandleInput` → `ActionInput`
  unmodified, so no change is needed there.

## Results

Pending.
