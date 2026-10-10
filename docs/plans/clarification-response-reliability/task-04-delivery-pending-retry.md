---
id: "04-delivery-pending-retry"
title: "Confirm delivery-pending retry outcomes"
status: done
wave: 4
depends_on:
  - "03-bound-and-recover-clarification-submission"
plan: "plan.md"
requirements:
  - REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001
acceptance_criteria:
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.2
  - AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.3
system_design:
  - "../../specs/tasks/system-design/clarification-response-reliability.md"
---

# Task 04: Confirm delivery-pending retry outcomes

## Outcome

An exact retry joins the live delivery of an already-claimed clarification
instead of returning its provisional answer. It returns only after durable
confirmation and the synchronous local watchdog notifier complete. A finalized
outcome with no live confirmation can be replayed; detached answer ownership
continues to prevent a duplicate tool response.

## In scope

- Reconcile an already-recorded answered or rejected outcome when the durable
  message still carries `response_delivery_pending`.
- Preserve the existing idempotent retry and rejection response contract.
- Cover answered, rejected and failed confirmation through the real resolver.
- Cover a finalized live response while its synchronous notifier is blocked,
  including session cancellation.

## Exclusions

- No change to clarification authority, persistence schema, or delivery
  ownership.
- No optimistic client success and no duplicate response delivery.

## Traceability

- `REQ-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.2`
- `AC-TASKS-CLARIFICATION-RESPONSE-RELIABILITY-001.3`
- `docs/specs/tasks/system-design/clarification-response-reliability.md`

## Implementation acceptance

- A retry of an answered or rejected claim marked delivery-pending waits for
  the existing resolver to confirm delivery, without asking the person again.
- A retry of a finalized live response joins any remaining confirmation callback
  and cannot return before the synchronous notifier.
- Session cancellation does not erase an in-flight confirmation before retries
  can join it. Registered waiters retain their original entry through removal.
- A failed confirmation returns an error and restores the current bundle when
  recovery is safe. No answer takes both live and detached paths.

## Verification

- `cd apps/backend && go test -trimpath -race -p 1 ./internal/mcp/handlers -run 'TestHandleAskUserQuestion_RetryDuring' -count=10`
- `cd apps/backend && go test -trimpath -race -p 1 ./internal/clarification ./internal/mcp/handlers ./internal/mcp/server ./internal/task/repository/sqlite -count=1`

## Results

Deterministic real-resolver regressions reproduce the provisional tool response
plus detached resume, false success on confirmation failure, and replay before
the synchronous watchdog notifier completes. The corrected delivery, lost
adoption, cancellation, and pinned-waiter cases pass with `-count=10`.

The four affected backend packages passed ordinary tests, and the PostgreSQL
reattachment lifecycle case passed against an isolated PostgreSQL 17.11 server.
Catalog, specification, public-documentation, SQL portability, formatting, and
current-main delta whitespace checks passed. Broad local lint attempts timed
out or encountered a compiler OOM within the four-GiB limit; the broad race run
was stopped without a full passing result. Under the coordinator's updated
validation direction, broad tests, race coverage, and the expensive Go lint
hook are transferred to hosted CI. The default CI checks must pass on the exact
pushed head before merge; no local broad-validation success is claimed.
