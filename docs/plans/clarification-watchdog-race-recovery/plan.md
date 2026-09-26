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

Update `clarification.Resolver` so its construction-supplied synchronous local orchestrator notifier
arms the watchdog from the successful delivery-confirmation callback after durable finalization and
before `Store.WaitForResponse` can return the response to the agent. Keep event-bus publication as
fan-out, mark it as handled by the local notifier to avoid a duplicate watchdog, and keep terminal
bundle-message publication after confirmed delivery. NATS publication is never the watchdog
acknowledgement.

### Recovery-owned cancellation activity

Extend `clarificationWatchdogEntry` with concurrency-safe recovery-cancellation phase state. Mark that
phase only around expected-turn silent cancellation. `cancelClarificationWatchdogsForSession` must
preserve the matching entry only when a live stream frame has an allowed cancellation-acknowledgement
type and matches the cancellation operation's captured execution and prompt-generation identity.
Message, thinking, and tool activity, newer or independent activity, and
`cancelAllClarificationWatchdogs`, retain their cancellation authority. After cancellation, recheck
the captured turn and allow queue handoff only when it remains current or is the exact turn durably
completed by the owned cancellation.

### Regression coverage

Add a resolver delivery test proving that the synchronous local notifier runs before the live waiter
receives the response. Add an orchestrator test whose silent cancellation synchronously emits the same
`session_info` activity observed in production and prove the answer still reaches exactly one queued or
dispatched replacement. Add a concurrent newer-prompt activity regression and a post-cancellation
turn-authority regression.

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
