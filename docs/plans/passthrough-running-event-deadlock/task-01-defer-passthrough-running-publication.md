---
id: "01-defer-passthrough-running-publication"
title: "Defer passthrough running publication"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001
acceptance_criteria:
  - AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.1
  - AC-TASKS-PASSTHROUGH-QUEUED-PROMPT-DISPATCH-001.2
system_design:
  - ../../specs/tasks/system-design/passthrough-queued-prompt-dispatch.md
---

# Task 01: Defer Passthrough Running Publication

## Summary

Prevent synchronous `agent.running` delivery from re-entering a session guard
held by `handleAgentReady`. Preserve the current queue, PTY, runtime-status,
and immediate-publication behavior outside that guarded path.

## In scope

- Add the lifecycle transition-and-deferred-publication primitive.
- Expose it through the backend lifecycle adapter without widening unrelated
  agent-manager interfaces.
- Use it for ordinary queued passthrough prompt delivery from
  `handleAgentReady`.
- Add focused lifecycle and orchestrator tests using a synchronous re-entrant
  running-event callback.

## Out of scope

- Asynchronous event-bus delivery.
- Durable lifecycle queue dispatch, frontend terminal reconnects, lock
  watchdogs, and WebSocket timeouts.
- Refactors of other session-guard call sites without a proven re-entry chain.

## Acceptance

- The red regression test times out on the pre-fix
  `handleAgentReady -> MarkPassthroughRunning -> handleAgentRunning` chain.
  After the correction, the test completes with one prompt delivery and an
  available session guard.
- Guarded delivery marks runtime state and writes all PTY chunks before it
  publishes the captured `agent.running` event. Publication still occurs after
  a post-transition PTY write error.
- The lifecycle transition and immutable payload capture are atomic under the
  execution-store lock. Concurrent preparation calls publish at most one
  `agent.running` event, and a competing status mutation cannot relabel it.
- Existing immediate callers publish exactly once, and deferred publication
  uses immutable captured event data.

## Verification

```bash
go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'TestHandleAgentReady_PassthroughQueuedMessageSynchronousRunningEvent|TestPreparePassthroughRunning|TestMarkPassthroughRunning' -count=1
go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1
```

Run both commands from `apps/backend`.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_passthrough_runtime_test.go`
- `apps/backend/internal/backendapp/adapters.go`
- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/event_handlers_passthrough_running_test.go`

## Dependencies

None.

## Risks

- The ordering of deferred callbacks relative to guard release depends on
  explicit defer registration order and needs direct test coverage.
- Adapter-only capability detection must fail safely in tests and alternate
  wiring that does not publish lifecycle events.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/passthrough-queued-prompt-dispatch.md`
- `docs/specs/tasks/system-design/passthrough-queued-prompt-dispatch.md`
- `docs/decisions/2026-08-31-passthrough-running-publication.md`
- `docs/decisions/0035-version-agent-ready-events-by-prompt-generation.md`
- GitHub issue #3177 and the diagnostic reproduction recorded in this task.

## Results

- Added the two-phase lifecycle capability that updates passthrough runtime state and returns a one-shot immutable `agent.running` publication callback.
- Forwarded the capability through the production lifecycle adapter without widening `executor.AgentManagerClient`; the guarded ordinary ready drain uses it and releases its session guard before publication.
- Preserved immediate `MarkPassthroughRunning` behavior for terminal input, prompt delivery, and test/legacy adapters without the optional capability.
- Added lifecycle snapshot/idempotence coverage plus synchronous ready-event re-entry coverage, including publication after a PTY write failure.
- Made the Ready-to-Running claim and event snapshot atomic under the execution-store lock; added deterministic competing-mutation and concurrent-prepare coverage.
- `go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp -run 'TestHandleAgentReady_PassthroughQueuedMessageSynchronousRunningEvent|TestPreparePassthroughRunning|TestMarkPassthroughRunning' -count=1` passed five tests in all three packages.
- `go test -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/backendapp -count=1` passed 5,086 tests.
- `go test -race -tags fts5 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run 'TestHandleAgentReady_PassthroughQueuedMessageSynchronousRunningEvent|TestHandleAgentReady_PassthroughQueuedMessagePublishesAfterWriteFailure|TestPreparePassthroughRunningDefersAndSnapshotsPublication|TestPreparePassthroughRunningCapturesSnapshotBeforeCompetingMutation|TestPreparePassthroughRunningClaimsTransitionOnceConcurrently|TestMarkPassthroughRunningPublishesOnceAndGuards' -count=1` passed six tests.
