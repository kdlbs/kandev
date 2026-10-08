---
id: turn-changed-files-05
title: Generation-bound admission and terminal capture fences
status: done
wave: 5
depends_on:
  - turn-changed-files-01
  - turn-changed-files-02
  - turn-changed-files-03
  - turn-changed-files-04
plan: plan.md
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-002
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-004
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-001.4
  - AC-TASKS-TURN-CHANGES-001.5
  - AC-TASKS-TURN-CHANGES-001.6
  - AC-TASKS-TURN-CHANGES-001.7
  - AC-TASKS-TURN-CHANGES-002.1
  - AC-TASKS-TURN-CHANGES-002.9
  - AC-TASKS-TURN-CHANGES-003.1
  - AC-TASKS-TURN-CHANGES-003.2
  - AC-TASKS-TURN-CHANGES-003.3
  - AC-TASKS-TURN-CHANGES-003.4
  - AC-TASKS-TURN-CHANGES-003.5
  - AC-TASKS-TURN-CHANGES-003.6
  - AC-TASKS-TURN-CHANGES-004.5
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Generation-bound admission and terminal capture fences

## Summary

Integrate immutable capture into runtime-owned dispatch and every terminal cleanup path without changing workflow authority.

## Scope and owned files

- Runtime capability/facade and construction-supplied synchronous admitted-prompt/terminal callbacks.
- `internal/agent/runtime/lifecycle/session.go`, `manager_events.go`, stop/cancel/error/foreground-idle and cleanup entry points through small new helpers.
- Orchestrator reservation/adoption/dispatch hooks and new modular `turn_changes` coordinator.
- `service.go`, `task_operations.go`, READY/COMPLETE handlers only for narrow integration calls.
- Queue/deferred/automated initiating-identity propagation and persisted capture policy.
- Correlated restart reconciliation and shared-checkout overlap intervals.

## Exclusions

No completion-gate, autopilot, agent/task state authority, queue policy, or general callback lease redesign.

## Implementation acceptance

1. Persist policy and fresh starts after generation admission but before external provider dispatch; off policy causes zero capture/ref/export writes.
2. Accepted end capture/export completes or records bounded failure before successor prompt writes and teardown, across READY/COMPLETE, cancel, failure, interruption, and process exit.
3. Duplicate/stale events, callback lock races, restart without boundary proof, mid-turn toggles, and overlapping checkouts preserve exact ownership and availability.

## Verification

Use real lifecycle completion entry points and controlled barriers. Run both completion/acknowledgement orders and asynchronous/NATS-like bus fakes.
Do not infer ordering from sleeps or status alone. Assert provider/cleanup spies cannot run across blocked capture.
Cover turn adoption, launch/model switch/resume/passthrough, foreground-idle, dispatch errors after effects, and replaced startup attempts.
A continuation/steer in the same turn retains its endpoints and policy; correlated generation lineage must not recapture the baseline.

```bash
cd apps/backend
go test -trimpath -race ./internal/orchestrator ./internal/agent/runtime ./internal/agent/runtime/lifecycle ./internal/task/changes -run 'TurnChangeCapture|TurnChangeAuthority|TurnChangeRecovery|TurnChangeOverlap' -count=1
go test -trimpath -race ./internal/agent/runtime/lifecycle -run 'DispatchCompletion|SendPromptSteer|SessionTurnSettlement|Cancel' -count=1
```

Record admitted generation, terminal owner, successor dispatch, and teardown ordering from test barriers.

## Dependencies and risks

Depends on policy, persistence, capture, and durable export. Do not hold SQL transactions or prompt/store mutexes during Git/HTTP I/O.
Do not recursively acquire startup callback leases. Stop issues cancellation before its bounded preservation attempt.
Unknown generation-zero or dynamic work cannot receive a late fabricated baseline.

## Results

Implemented generation-bound admission, terminal capture fences, checkout-manifest refresh, overlap attribution, and nonblocking failure persistence. The full backend suite passed. Race-enabled dispatch-completion, prompt-steer, cancellation, turn-change, and settlement tests passed; `internal/task/changes` passed its complete race suite, and the new launch/admission checkout-manifest lifecycle tests passed under `-race`.

### Review follow-up (2026-10-08)

The terminal assistant anchor now comes from durable per-generation assistant-message tracking and survives protocol correlation reset and no-newline flush; it is cleared only after durable finalization. Terminal ownership and accepted endpoints persist independently from compare/export, bounded failure settlement uses a fresh persistence context, and startup reconciliation settles unfinished rows without another capture. Automated task turns use policy/capture, while run-owned executions remain excluded. Overlap intervals are stored for both turns using checkout/worktree aliases. Focused lifecycle and coordinator tests passed for these cases; no new executor-matrix evidence was added.

Terminal persistence retries now retain the same terminal identity and message anchor with capped exponential backoff until persistence succeeds or manager shutdown cancels the attempt. The generation fence remains closed during retries and opens after success. A focused race-enabled lifecycle test verifies retry, anchor removal, and successor admission.
