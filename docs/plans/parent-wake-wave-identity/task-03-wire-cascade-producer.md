---
id: "03-wire-cascade-producer"
title: "Wire cascade (P1) onto wave identity"
status: done
wave: 3
depends_on: ["01-wave-identity-primitives", "02-wave-identity-persistence"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-004
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 03: Wire cascade (P1) onto wave identity

## Summary

`office/scheduler.cascadeChildrenCompleted` gains the terminality-confirming
wave-member read, derives and persists the wave identity on its direct
insert, gains its own unique-violation classification and coalescing guard
(the runs/service ones from Task 02 do not cover this call site), and
resolves the workflow-authored `on_children_completed` payload for parity
with the engine-routed producers.

## In scope

- `RunContext` gains `WaveKey`, `WaveString string` (JSON `"-"`).
- `cascadeChildrenCompleted`: after the existing `ListChildStates`
  terminal-state loop, read `ListWaveMembers`; queue nothing on a read error
  or any non-terminal member; queue nothing on an empty result
  (AC-...-001.7); otherwise derive the wave identity from the sorted ids.
- `QueueRunCtx`/`QueueRun` (`office/scheduler/run.go`) take the wave
  identity through to the inserted `models.Run`, skip
  `ss.repo.CoalesceRun` when a wave key is present, and classify
  `ss.repo.CreateRun`'s error with `IsWakeWaveUniqueViolation` into a nil
  return + debug log (not the existing `enqueue run: %w` wrap) —
  `runs/service`'s classification from Task 02 does not cover this
  direct-insert call site.
- Payload parity (AC-...-002.16): a new `WorkflowStepGetter` interface
  (`GetStep(ctx, stepID) (*wfmodels.WorkflowStep, error)`), a
  `SchedulerService.SetWorkflowStepGetter` setter wired at boot to
  `workflow/service.Service.GetStep`, and cascade logic that resolves the
  parent's current step via `GetTaskWorkflowStepID` +
  `WorkflowStepGetter.GetStep` + `engine.CompileStep`, finds a `queue_run`
  action on `TriggerOnChildrenCompleted` with a non-nil payload, and merges
  it into the run's persisted payload with the same override precedence
  `engine.queueRunPayload` applies. Any failure at any step: log at debug,
  queue without the merged payload (AC-...-004.2 — the wake is
  unconditional).
- `parent_wake_deduped_total` increment at this task's classification site.

## Out of scope

- P2/P3/P4 (Tasks 04, 05).
- `runs/service`'s own classification and coalescing guard (Task 02,
  already done, unrelated call site).
- Any change to `childrenCompletedIdempotencyKey` or cascade's existing
  no-session behavior (AC-...-004.2's first half, already true today).

## Acceptance

- Two calls to `cascadeChildrenCompleted` for the same parent and the same
  terminal child set produce exactly one `runs` row, and the second call
  logs no failure.
- A wave-member read that observes one non-terminal member (simulated via a
  fake repo) queues nothing; a subsequent call once all members are
  genuinely terminal queues the wake.
- When the parent's current step declares an `on_children_completed`
  `queue_run` action with a payload, that payload's keys appear in the
  queued run's persisted payload; when the step lookup fails, the run is
  still queued without them and a debug log records the omission.

## Verification

```bash
cd apps/backend
go test ./internal/office/scheduler/... -run TestCascadeChildrenCompleted -v
go test ./internal/office/scheduler/... -run TestCascade.*WavePayload -v
```

## Files likely touched

- `internal/office/scheduler/reactivity.go`
- `internal/office/scheduler/reactivity_children_completed_test.go`
- `internal/office/scheduler/run.go`
- `internal/office/scheduler/run_test.go`
- `internal/backendapp/main.go` (wire `SetWorkflowStepGetter`)

## Dependencies

Task 01 (`waveidentity`, `ListWaveMembers`), Task 02 (`models.Run` columns,
`IsWakeWaveUniqueViolation`).

## Risks

- **Payload merge shape.** `RunContext` is a fixed struct, not a generic
  map; merging an arbitrary action payload onto it (marshal → overlay →
  re-marshal, or an equivalent) must not change `encodeRunContext`'s output
  for the common case where no action payload exists — `CoalesceRun`
  compares payload bytes for equality-adjacent decisions elsewhere in the
  codebase, so an unconditional shape change here is a regression risk even
  though `CoalesceRun` itself is skipped for wave-carrying requests.
- **`GetTaskWorkflowStepID` reads the parent's *current* step at wake time,
  not at any earlier decision point** — this is intentional (the design
  states this resolution "reads workflow configuration, not child task
  state," so it may run after the wave-member read), but do not repurpose
  this value for anything identity-bearing; it is payload content only.

## Parallelism

`sequential`

## Inputs

- System design: "Producers" (P1 cascade), "Wake equivalence between
  producers" sections.
- `internal/workflow/engine/phase2_callbacks.go` (`queueRunPayload`,
  `QueueRunCallback.Execute`) for the precedence and shape to match.
- `internal/orchestrator/workflow_store.go:217-232` (`LoadStep`) for the
  `GetStep` + `engine.CompileStep` pattern.
- `internal/office/repository/sqlite/participants.go:55`
  (`GetTaskWorkflowStepID`).

## Results

Done, in `feat(office): wire the cascade producer onto wave identity`.
`cascadeChildrenCompleted` now calls `resolveWaveIdentity` (its own
terminality-confirming `ListWaveMembers` read) and `resolveWaveActionPayload`
(step lookup + `engine.CompileStep`) after the existing `ListChildStates`
loop; `RunContext` carries `WaveKey`/`WaveString`/`ExtraPayload` (all
`json:"-"`); `queueRun` (the shared body behind `QueueRun`/`QueueRunCtx`)
skips `CoalesceRun` and classifies `IsWakeWaveUniqueViolation` when a wave
key is present. `SetWorkflowStepGetter` wired in
`internal/backendapp/main.go` to `services.Workflow` alongside the
existing `SetWorkflowEngineDispatcher` call.

One deviation from the original scope note: rather than changing the
public `QueueRun`/`QueueRunCtx` signatures (`QueueRun` implements
`shared.RunQueuer`, used by other non-wave callers), the wave fields route
through a new unexported `queueRun` helper both public methods call.

`go test ./internal/office/scheduler/...` green (18 new/changed tests
across `run_test.go` and `reactivity_children_completed_wave_test.go`,
plus `newReactivityTestRepo`'s ad hoc `tasks` fixture gained
`archived_at`/`is_ephemeral`/`origin` columns so `ListWaveMembers` can run
against it). `golangci-lint run ./internal/office/scheduler/...
./internal/backendapp/... --new-from-rev=cd78236315f28982848de4938d56f7722c7f632f`
clean.
