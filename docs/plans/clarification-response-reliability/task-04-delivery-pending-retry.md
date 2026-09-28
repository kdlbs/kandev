---
id: "04-delivery-pending-retry"
title: "Return delivery-pending retry outcomes"
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

# Task 04: Return delivery-pending retry outcomes

## Outcome

An exact retry whose durable clarification outcome is already terminal but whose
response delivery is still marked pending returns that recorded answer or
rejection immediately. It does not create a duplicate response or leave a new
retry waiter behind.

## In scope

- Reconcile an already-recorded answered or rejected outcome when the durable
  message still carries `response_delivery_pending`.
- Preserve the existing idempotent retry and rejection response contract.
- Cover both answered and rejected delivery-pending outcomes with a focused
  handler regression test.

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

- A retry of a durable answered outcome marked delivery-pending returns the
  recorded answer without waiting for a new response.
- A retry of a durable rejected outcome marked delivery-pending returns the
  recorded rejection without waiting for a new response.
- Neither retry creates a pending waiter or duplicates the durable outcome.

## Verification

- `cd apps/backend && go test ./internal/mcp/handlers -run 'TestHandleAskUserQuestion_RetryReturnsDeliveryPendingRecordedOutcome' -count=1`

## Results

Implemented the handler reconciliation and regression coverage for answered and
rejected delivery-pending clarification outcomes.
