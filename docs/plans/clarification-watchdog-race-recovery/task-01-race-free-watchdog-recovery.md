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
- Move primary-answer event publication into the successful live delivery-confirmation boundary.
- Track the narrow recovery-owned silent-cancellation phase on the watchdog entry.
- Keep current-turn, prompt-generation, queue, and shutdown behavior unchanged.

## Out of scope

- Timeout tuning.
- Schema, API, frontend, or executor changes.
- Detached-response flow changes.

## Acceptance

- A live waiter cannot receive the clarification answer before the primary-answer event has armed its
  watchdog, and the event is published exactly once.
- Stream activity emitted synchronously by fallback's silent cancellation does not cancel its recovery
  context; the answer reaches exactly one replacement queue or dispatch.
- Independent live activity and service shutdown still cancel in-flight watchdog fallback work.

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
- Added `TestClarificationWatchdogRecoveryIgnoresOwnCancelActivity`, which synchronously emits `session_info` from silent cancellation and proves the watchdog context survives to one replacement prompt.
- Moved primary-answer publication into the successful durable delivery-confirmation callback.
- Added an atomic recovery-cancellation phase marker and ignored only that phase's own stream activity; independent activity and service-wide cancellation remain authoritative.
- Targeted regression command passed.
- Race-enabled clarification and orchestrator suites passed.
