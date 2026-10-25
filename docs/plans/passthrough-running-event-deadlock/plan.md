---
created: 2026-08-31
status: done
requirements:
  - REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001
system_design:
  - ../../specs/tasks/system-design/passthrough-queued-prompt-dispatch.md
legacy_specs: []
---

# Implementation Plan: Passthrough Running Event Deadlock

## Overview

Introduce a two-phase passthrough-running lifecycle operation, then use it at
the guarded ordinary queue drain so `agent.running` is published only after the
session guard is released. One work order keeps the lifecycle contract,
orchestrator integration, and regression proof atomic.

## Scope

### In scope

- Preserve the immediate marker behavior for existing unguarded callers.
- Capture an immutable running-event payload before deferred publication.
- Defer publication across the ready handler's session-guard boundary.
- Add lifecycle and orchestrator regression tests for the exact re-entry chain.

### Out of scope

- Global event-bus scheduling changes.
- Lock watchdogs, WebSocket deadlines, and delete-path logging.
- Frontend reconnect behavior for terminal sessions.
- Queue policy or passthrough submit-sequence changes.

## Technical approach

Add a lifecycle primitive in
`internal/agent/runtime/lifecycle/manager_passthrough.go` that marks the
execution running and returns a callback closing over an immutable event
payload. Claim the transition and capture the payload under the execution-store
lock, then persist after releasing it. Refactor `MarkPassthroughRunning` to call
the primitive and publish immediately, preserving all existing callers.

Expose the deferred form through `backendapp.lifecycleAdapter`. In the
orchestrator, detect that narrow capability in the ordinary passthrough branch
of `handleAgentReady`. Store the returned callback before PTY writes. Register
its defer before the code acquires the session guard. Go's LIFO defer order then
releases the guard first. Run the callback even when a PTY write fails after
the status transition.

Do not change `MemoryEventBus`, `handleAgentRunning`, durable lifecycle queue
delivery, or the immediate passthrough marker used by terminal input.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.1` | Extend the ordinary passthrough queued-message test to assert one PTY write, one queue consumption, and the successor running transition. |
| `AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.2` | Add a synchronous re-entry regression in `internal/orchestrator` and lifecycle tests proving publication is deferred, immutable, atomic under competing mutation, at-most-once under concurrent preparation, and immediate callers still publish once. |

## Work orders

- [x] [Task 01: Defer passthrough running publication](task-01-defer-passthrough-running-publication.md)

## Verification results

- `go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'TestHandleAgentReady_PassthroughQueuedMessageSynchronousRunningEvent|TestPreparePassthroughRunning|TestMarkPassthroughRunning' -count=1` passed five tests in all three packages.
- `go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1` passed 5,086 tests.
- `go test -race -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'TestHandleAgentReady_PassthroughQueuedMessageSynchronousRunningEvent|TestHandleAgentReady_PassthroughQueuedMessagePublishesAfterWriteFailure|TestPreparePassthroughRunningDefersAndSnapshotsPublication|TestPreparePassthroughRunningCapturesSnapshotBeforeCompetingMutation|TestPreparePassthroughRunningClaimsTransitionOnceConcurrently|TestMarkPassthroughRunningPublishesOnceAndGuards' -count=1` passed six tests in both packages.

## Risks

- A deferred callback skipped on a PTY error would leave runtime status and
  task-session state inconsistent.
- A callback built from the mutable execution pointer could publish a later
  execution identity or state.
- Releasing the guard before the PTY write would reopen cancellation and
  duplicate-dispatch races. Only event publication can cross the boundary.
