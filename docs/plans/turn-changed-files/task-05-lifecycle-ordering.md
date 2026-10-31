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
2. Accepted end capture precedes successor prompt writes. Content preservation or bounded final failure precedes source teardown across terminal paths.
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

The completed [preservation retry package](../turn-change-preservation-retry/plan.md) refines temporary content failure and restart resumption.
Its runtime work order separates the accepted-end prompt fence from preservation before source destruction.
The results below remain historical validation of the original implementation.

The [history integrity attribution work order](../turn-change-history-integrity/task-03-writer-attribution.md)
implements canonical writer intervals. Historical validation below remains unchanged.

Implemented generation-bound admission, terminal capture fences, checkout-manifest refresh, overlap attribution, and nonblocking failure persistence. The full backend suite passed. Race-enabled dispatch-completion, prompt-steer, cancellation, turn-change, and settlement tests passed; `internal/task/changes` passed its complete race suite, and the new launch/admission checkout-manifest lifecycle tests passed under `-race`.

### Review follow-up (2026-10-08)

The terminal assistant anchor now comes from durable per-generation assistant-message tracking and survives protocol correlation reset and no-newline flush; it is cleared only after durable finalization. Terminal ownership and accepted endpoints persist independently from compare/export, bounded failure settlement uses a fresh persistence context, and startup reconciliation settles unfinished rows without another capture. Automated task turns use policy/capture, while run-owned executions remain excluded. Overlap intervals are stored for both turns using checkout/worktree aliases. Focused lifecycle and coordinator tests passed for these cases; no new executor-matrix evidence was added.

Terminal persistence retries now retain the same terminal identity and message anchor with capped exponential backoff until persistence succeeds or manager shutdown cancels the attempt. The generation fence remains closed during retries and opens after success. A focused race-enabled lifecycle test verifies retry, anchor removal, and successor admission.

### PR CI fixup (2026-10-10)

The terminal dispatch-failure path must tolerate an agentctl client that has disappeared. Persist the affected repository endpoints as unavailable and complete terminal settlement without panicking or blocking task cleanup.

Regression: `TestCoordinatorFinalizesUnavailableWhenTerminalAgentctlClientIsMissing` passes a typed-nil agentctl client after start admission and verifies unavailable terminal state.

```bash
cd apps/backend
go test -tags fts5 ./internal/task/changes -run TestCoordinatorFinalizesUnavailableWhenTerminalAgentctlClientIsMissing -count=1
go test -trimpath -race -tags fts5 ./internal/task/changes -run TestCoordinatorFinalizesUnavailableWhenTerminalAgentctlClientIsMissing -count=1
go test -tags fts5 ./internal/agent/runtime/agentctl -run TestTurnCheckpointClient -count=1
```

The typed-nil agentctl regression passed normally and under the race detector. Both affected Go packages passed, the lifecycle race suite passed, and the PR-watcher cleanup E2E passed against the rebuilt backend.

### Conflict integration (2026-10-19)

Merged main `72e9eec9b300918863f7d1da89f80303bf694335` while preserving durable
prompt ownership before capture admission and provider dispatch. Shutdown retains
both retry registration fencing and manager-context cancellation. Required-store
and upgrade inventories retain the turn tables alongside main's new tables.

The durable-binding regression assertions reject capture before the binding write
and reject capture after a failed write. The focused lifecycle race selection
(`TurnChange|DurablePromptTurnBinding|StopWaitsForRetriedTerminalCapture|StopPreservesActiveTurn|Cancel|SessionTurnSettlement`)
passed. A reversed-order mutation failed both binding assertions as expected.
Twenty-nine focused frontend tests and specification/catalog checks passed.
Longer local checks were interrupted; their incomplete runs are not pass evidence.
Updated-head required CI remains the delivery gate. Work order 09's qualification
limits remain unchanged.

### Backend-shutdown delivery semantics (2026-10-20)

Exact-head CI at `6d6aea8dd53d` failed the desktop continuation restart case with
agent survival disabled: the session became idle but never reached the restored
delivery phase. The same assertion failed locally with retries disabled. The
checkpoint stop path sent ACP cancellation before process shutdown, changing the
continuation submission to cancelled and removing its automatic-recovery input.

Backend shutdown now uses its existing process-stop path before terminal capture,
with the retained HTTP checkpoint transport available until executor cleanup. It
does not issue a prompt cancellation. Explicit Stop retains cancel, capture, stop
ordering. Forced or unconfirmed shutdowns do not accept a terminal checkpoint;
existing startup reconciliation marks unfinished capture unavailable. The native
restore assertion and the landed delivery ownership/settlement guards are unchanged.

The new race regression rejected the old shutdown order, then passed for confirmed,
unconfirmed, and forced shutdown plus explicit Stop. The existing stop, retry, and
turn-capture regression selection also passed under the race detector. Fresh backend,
delivery fixture, and plugin artifacts passed both restart browser cases, the full
15-test desktop continuation spec, and its two-test phone counterpart with retries
disabled. Browser runs used isolated port slot 25. Commands:

```bash
cd apps/backend
GOMAXPROCS=2 go test -p 2 -trimpath -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestTurnChangeCapturePreservesShutdownDeliveryOutcome|TestStopPreservesActiveTurnBeforeExecutorCleanup|TestStopWaitsForRetriedTerminalCaptureBeforeReturning|TestStopAgentWithReason' -count=1
cd ../web
E2E_PORT_OFFSET=25 pnpm e2e:raw --project=chromium tests/session/provider-interruption-continuation.spec.ts --retries=0
E2E_PORT_OFFSET=25 pnpm e2e:raw --project=mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts --retries=0
```

Public documentation already promises verified background native restoration in
`docs/public/tasks-and-workflows.md`; this fix restores that behavior without new
copy or controls. Work order 09's executor, storage, and performance qualification
limits remain open. Fresh required CI on the remediation commit remains necessary.
