---
id: "05-wire-orchestrator-producer"
title: "Wire orchestrator (P4) onto wave identity"
status: done
wave: 3
depends_on: ["01-wave-identity-primitives", "04-wire-engine-routed-producers"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-004
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 05: Wire orchestrator (P4) onto wave identity

## Summary

`processOnChildrenCompleted` already reads exactly the wave-member rows
(`ListChildCompletionRows` applies the same predicate `ListWaveMembers`
does) and already confirms terminality on them before evaluating actions.
It needs no new read — only a re-sort by id (its source orders by
`created_at` first) and to set the two encodings on the
`OnChildrenCompletedPayload` the Task 04 seam now carries through.

## In scope

- `childCompletionPayload` (`event_handlers_children_completed.go`) gains a
  `parentID string` parameter; before building `ChildSummaries`, sort a copy
  of `rows` ascending by `ID` and derive `WaveKey`/`WaveString` from that
  sorted id list. `childCompletionOperationID` is unchanged (keeps ordering
  by the rows' existing `created_at` order and its full
  state/step/terminal/`updated_at` derivation).
- Update `childCompletionPayload`'s one call site in
  `evaluateChildrenCompleted` to pass `parent.ID`.

## Out of scope

- P1/P2/P3 (Tasks 03, 04).
- `readyChildCompletionRows`, `allChildrenTerminal`,
  `annotateTerminalChildSteps` — unchanged; they already establish
  terminality on the rows the wave identity is derived from
  (AC-...-002.15 satisfied by construction).
- Any change to the operation ledger, `EvaluateOnly` transition lifecycle,
  or reopen-driven step transition (AC-...-004.3).

## Acceptance

- For a fixed parent and child set, P4's derived `WakeWaveKey` is
  byte-identical to what P1/P2/P3 would derive for the same parent and
  wave-member set (proven together with Task 07's cross-producer test, but
  this task's own unit test covers P4's re-sort in isolation: feed rows in
  `created_at` order that differ from `id` order and assert the derived key
  matches the id-sorted expectation).
- `TestProcessOnChildrenCompleted_DedupedAcrossReinit` (or the equivalent
  existing operation-ledger test) continues to pass unmodified.

## Verification

```bash
cd apps/backend
go test ./internal/orchestrator/... -run TestProcessOnChildrenCompleted -v
go test ./internal/orchestrator/... -run TestChildCompletionPayload -v
```

## Files likely touched

- `internal/orchestrator/event_handlers_children_completed.go`
- `internal/orchestrator/event_handlers_children_completed_test.go` (or
  wherever the existing P4 tests live — confirm exact file before editing)

## Dependencies

Task 01 (`waveidentity`), Task 04 (the engine seam this payload now flows
through — `OnChildrenCompletedPayload`'s new fields and
`QueueRunCallback.Execute`'s type assertion must exist first).

## Risks

None beyond the shared-seam ordering already called out in Task 04 — this
task is a small, additive change to one existing function.

## Parallelism

`sequential`

## Inputs

- System design: "Producers" (P4 orchestrator) section, "The wave-member
  read is the last read, and it carries state" (P4 paragraph).
- `internal/task/repository/sqlite/task.go:2055-2070`
  (`ListChildCompletionRows`) for the `created_at, id` ordering this task
  must correct for.

## Results

Done, in `feat(orchestrator): derive wave identity in the children-completed
producer`. `childCompletionPayload(parentID string, rows
[]models.ChildCompletionRow)` derives `WaveKey`/`WaveString` via a new
`childCompletionWaveIdentity` helper (copies row ids into a fresh slice,
`sort.Strings`, then `waveidentity.WaveKey`/`WaveString` — never mutates
`rows`, so `childCompletionOperationID`'s created_at-ordered derivation,
called earlier in `processOnChildrenCompleted`, is unaffected regardless of
call order). No new read: `readyChildCompletionRows` already applies the
same predicate `ListWaveMembers` does and already confirms terminality
before `rows` reaches `evaluateChildrenCompleted`; it also already gates on
`len(rows) == 0`, so `childCompletionPayload` never runs against zero wave
members in production (AC-...-001.7 holds by construction, no extra guard
needed).

`go test ./internal/orchestrator/...` (full package, ~84s) and
`golangci-lint run ./internal/orchestrator/...
--new-from-rev=cd78236315f28982848de4938d56f7722c7f632f` both clean.
`TestProcessOnChildrenCompleted_DedupedAcrossReinit` passes unmodified.
