---
id: "02-startup-cancellation"
title: "Cancel startup attempts"
status: done
wave: 2
depends_on: ['01-load-failure']
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.3
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.4
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 02: Cancel startup attempts

## Summary

Bind startup and its pending prompt to a cancellable attempt. Reject every
late continuation after cancellation, including callbacks on reused executions.

## In scope

- Own the operation context independently of browser requests and cancel it on explicit Stop or service shutdown.
- Validate attempt identity at manual and lazy resume, handler retry, token publication, and dispatch acceptance.
- Use captured execution identity for cleanup and preserve unrelated queued work.

## Out of scope

Provider internals, new queue policy, schema changes, and live task repair.

## Acceptance

- Cancellation before readiness prevents all old prompt, token, state, and failure publication.
- A later retry dispatches once even when old success or failure arrives afterward.
- Browser disconnect does not cancel accepted work, and cleanup remains bounded without lock inversion.

## Verification

```bash
(cd apps/backend && go test -race ./internal/orchestrator -count=1 -timeout=20m)
(cd apps/backend && go test -race ./internal/orchestrator/executor -count=1 -timeout=15m)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -count=1 -timeout=15m)
(cd apps/backend && go test -race ./internal/task/handlers -count=1 -timeout=15m)
```

Run new regressions before production changes and record the expected failure.

## Files likely touched

- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/resume_attempt.go (new narrow helper)`
- `apps/backend/internal/orchestrator/task_operations_resume_cancellation_test.go (new)`
- `apps/backend/internal/orchestrator/executor/executor.go`
- `apps/backend/internal/agent/runtime/lifecycle/session.go`
- `apps/backend/internal/task/handlers/message_handlers.go`

## Dependencies

01-load-failure.

## Risks

Preserve supported provider behavior and avoid lock inversion. Late cleanup
must not mutate the current attempt. Use deterministic barriers, not sleeps.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/agents/requirements/session-recovery-failures.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md).
- [Plan evidence and regression map](plan.md).
- Existing resume tests, cancellation helpers, and contribution recovery card.

## Results

Implemented process-local resume attempt ownership across manual resume, lazy
resume, prompt admission, cancellation, shutdown, token publication, and late
startup callbacks. A cancelled attempt is fenced by identity and exact
execution cleanup, so it cannot dispatch or stop a replacement execution.
Request disconnects remain detached from accepted recovery work, while explicit
cancellation interrupts startup and readiness waits.

The runtime evidence now exercises the service paths rather than only registry
helpers: cancellation at the continuation and provider-acceptance barriers,
readiness cancellation, old boot/token/failure callbacks after cancellation
and same-execution replacement, browser disconnect, shutdown, exact cleanup,
and parked Auto-run-off work. The lifecycle, orchestrator, executor, and
message-handler race suites pass.
