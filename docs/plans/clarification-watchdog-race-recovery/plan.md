---
created: 2026-08-24
status: completed
requirements:
  - REQ-TASKS-CLARIFICATION-LIFECYCLE-001
  - REQ-TASKS-CLARIFICATION-SCENARIOS-001
system_design:
  - ../../specs/tasks/system-design/clarification-active-lifecycle.md
legacy_specs: []
---

# Implementation Plan: Clarification Watchdog Race Recovery

## Overview

Make live clarification answer delivery and watchdog recovery one ordered operation. The fix first
arms recovery before ACP can acknowledge the answer, then prevents recovery-owned cancellation frames
from cancelling the fallback context that must hand off that answer.

## Scope

### In scope

- Arm the primary-answer watchdog within the durable live-delivery confirmation boundary.
- Preserve the fallback context across activity caused by its own silent cancellation.
- Keep independent session activity and service shutdown able to interrupt fallback work.
- Add deterministic regressions for both observed races.

### Out of scope

- Changing the 15-second watchdog timeout.
- Changing detached clarification delivery or persistence schemas.
- Changing ACP, MCP, or clarification response envelopes.
- Resuming or modifying the affected historical session.

## Technical approach

### Live delivery ordering

Update `clarification.Resolver` so `ClarificationPrimaryAnswered` is published from the successful
delivery-confirmation callback after durable finalization and before `Store.WaitForResponse` can return
the response to the agent. Keep terminal bundle-message publication after confirmed delivery and remove
the later duplicate primary-answer publication.

### Recovery-owned cancellation activity

Extend `clarificationWatchdogEntry` with concurrency-safe recovery-cancellation phase state. Mark that
phase only around `cancelAgentSilentWithGuard`. `cancelClarificationWatchdogsForSession` must preserve
the matching entry when a live stream frame is caused by that recovery-owned cancellation, while normal
activity and `cancelAllClarificationWatchdogs` retain their current cancellation behavior.

### Regression coverage

Add a resolver delivery test proving that the primary-answer event is published before the live waiter
receives the response. Add an orchestrator test whose silent cancellation synchronously emits the same
`session_info` activity observed in production and prove the answer still reaches exactly one queued or
dispatched replacement.

## Tests

| Acceptance criterion | Evidence |
| --- | --- |
| `AC-TASKS-CLARIFICATION-LIFECYCLE-001.3` | Resolver ordering and orchestrator recovery regressions |
| `AC-TASKS-CLARIFICATION-SCENARIOS-001.1` | Immediate acknowledgement and recovery-owned cancellation scenarios |

## Work orders

- [x] [Task 01: Make clarification watchdog recovery race-free](task-01-race-free-watchdog-recovery.md)

## Verification results

- `go test ./internal/clarification ./internal/orchestrator -run 'TestResolverLiveDeliveryPublishesPrimaryAnswerBeforeWaiterReturns|TestClarificationWatchdogRecoveryIgnoresOwnCancelActivity|TestClarificationWatchdogCancellationInterruptsFallbackLookup|TestHandleAgentStreamEvent_CancelsClarificationWatchdogs' -count=1` passed.
- `go test -race ./internal/clarification ./internal/orchestrator` passed.

## Risks

- Publishing the primary-answer event earlier must not publish terminal clarification messages before
  durable delivery succeeds.
- The recovery-owned phase must be narrow; a broad exemption could ignore genuine agent activity that
  should stop fallback work.
- Cancellation and replacement prompt ownership must retain the prompt-generation guarantees in ADR
  0035.
