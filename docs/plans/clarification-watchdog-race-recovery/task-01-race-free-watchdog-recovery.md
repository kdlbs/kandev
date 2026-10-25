---
id: "01-race-free-watchdog-recovery"
title: "Make clarification watchdog recovery race-free"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-SCENARIOS-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-LIFECYCLE-001.3
  - AC-TASKS-CLARIFICATION-SCENARIOS-001.1
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
---

# Task 01: Make Clarification Watchdog Recovery Race-Free

## Summary

Order live-answer watchdog registration before ACP response release. Preserve fallback recovery across
the stream frames produced by its own silent cancellation while keeping all independent cancellation
paths effective.

## In scope

- Write the two deterministic regression tests before changing production code.
- Supply a synchronous local primary-answer notifier to the resolver and invoke it inside the
  successful live delivery-confirmation boundary; retain the event bus as fan-out without duplicate
  local watchdog registration.
- Track the narrow recovery-owned silent-cancellation phase on the watchdog entry and classify only
  exact cancellation acknowledgement frames by the cancellation operation's execution/prompt-generation
  identity. Message, thinking, and tool frames remain authoritative activity.
- Bind cancellation to the captured clarification turn and recheck turn authority after cancellation
  before queueing the replacement; keep current-turn, queue, and shutdown behavior unchanged.

## Out of scope

- Timeout tuning.
- Schema, API, frontend, or executor changes.
- Detached-response flow changes.

## Acceptance

- A live waiter cannot receive the clarification answer before the synchronous local notifier has armed
  its watchdog, and the fan-out event is published exactly once without a duplicate local watchdog.
- Stream activity emitted synchronously by fallback's silent cancellation does not cancel its recovery
  context; the answer reaches exactly one replacement queue or dispatch.
- Independent live activity, successor-turn authority, and service shutdown still cancel or reject
  in-flight watchdog fallback work.

## Verification

```bash
go test ./internal/clarification ./internal/orchestrator -run 'TestResolverLiveDeliveryPublishesPrimaryAnswerBeforeWaiterReturns|TestClarificationWatchdogRecoveryIgnoresOwnCancelActivity|TestClarificationWatchdogCancellationInterruptsFallbackLookup|TestHandleAgentStreamEvent_CancelsClarificationWatchdogs'
go test -race ./internal/clarification ./internal/orchestrator
```

## Files likely touched

- `apps/backend/internal/clarification/resolver.go`
- `apps/backend/internal/clarification/resolver_delivery_test.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification.go`
- `apps/backend/internal/orchestrator/event_handlers_clarification_test.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`

## Dependencies

None.

## Risks

- Event ordering is part of durable live-delivery confirmation and must not be moved outside that
  boundary.
- Recovery-owned activity classification must not suppress explicit user or coordinator cancellation.

## Parallelism

`sequential`

## Inputs

- `docs/specs/tasks/requirements/clarification-active-lifecycle.md`
- `docs/specs/tasks/requirements/clarification-active-lifecycle-scenarios.md`
- `docs/specs/tasks/system-design/clarification-active-lifecycle.md`
- `docs/decisions/2026-08-14-current-turn-clarification-ownership.md`
- `docs/decisions/0035-version-agent-ready-events-by-prompt-generation.md`
- Production ACP and backend timeline from task `24e6e498-a816-45dc-926b-e8b32c8bc5e9`.

## Results

- Added `TestResolverLiveDeliveryPublishesPrimaryAnswerBeforeWaiterReturns`, which proves the primary-answer event is published before the live waiter returns and is published exactly once.
- Added `TestResolverLiveDeliveryNotifiesWatchdogBeforeWaiterReturns`, which proves the synchronous local
  watchdog notifier runs before the live waiter returns.
- Added `TestResolverLiveDeliveryUsesSynchronousNotifierBeforeAsyncFanout`, which proves an async/NATS-like
  event-bus publication cannot substitute for the construction-supplied local notifier.
- Added `TestClarificationWatchdogRecoveryIgnoresOwnCancelActivity`, which synchronously emits `session_info` from silent cancellation and proves the watchdog context survives to one replacement prompt.
- Kept primary-answer fan-out publication in the successful durable delivery-confirmation callback and
  added the synchronous local watchdog notifier at that boundary.
- Added a synchronous local watchdog notifier and an inline-handled bus marker so NATS fan-out cannot
  release the waiter before local watchdog registration or arm a duplicate watchdog.
- Added an atomic recovery-cancellation phase marker keyed to exact cancellation acknowledgement frames
  and the cancellation operation's execution/prompt-generation identity; message, thinking, tool,
  independent, and service-wide activity remain authoritative.
- Bound silent cancellation to the captured clarification turn and added a post-cancel authority check
  that accepts owned completion but rejects a successor turn.
- Added regressions for newer prompt activity, same-execution message activity, post-cancel authority,
  normal-agent-frame classification, and idempotent blocked-cancel cleanup.
- Targeted regression command passed.
- Race-enabled clarification and orchestrator suites passed.
