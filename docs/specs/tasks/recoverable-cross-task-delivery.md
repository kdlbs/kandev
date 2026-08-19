---
status: building
created: 2026-08-19
owner: kandev
---

# Recoverable Cross-Task Delivery

## Why

Agents need to report to busy tasks without retaining their own turns or repeatedly guessing when target capacity will return.

Decision: [ADR-2026-08-19-recoverable-cross-task-delivery](../../decisions/2026-08-19-recoverable-cross-task-delivery.md).

## What

- `message_task_kandev` accepts an optional `idempotency_key` and returns a durable `delivery_id` plus its current state.
- A saturated target returns a successful `pending_capacity` receipt. Kandev retries admission independently of the sender with a finite budget.
- Repeating the same key, or the same derived source-turn fingerprint, returns the same receipt and never creates another prompt.
- Executor acceptance acknowledges a reserved delivery. Transient failures remain retryable; exhausted or nonretryable failures remain recoverable with retained payload and error.
- Source and target task contexts can inspect a delivery; ordinary cross-workspace messaging remains supported.

## Data model

`message_deliveries` records immutable source/target attribution, payload metadata, queue entry identity, attempts, lease, error, timestamps, and state: `pending_capacity`, `queued`, `reserved`, `delivered`, `retry_wait`, `recoverable`, `terminal_failed`, or `cancelled`. Its idempotency key is unique per sender session and source turn.

## Failure modes

Capacity does not reject the accepted delivery. A worker crash releases its lease for another bounded claim. Archive/delete or generation invalidation cancels the row and never revives it. Kandev never logs that it lost a message while deleting the only payload.

## Persistence guarantees

Receipts, retry state, and retained recoverable payload survive backend restart. Runtime worker state and leases may expire and be reclaimed; the source turn is never required to remain active.

## Scenarios

- **GIVEN** a full target queue, **WHEN** an agent sends a report, **THEN** it receives one `pending_capacity` receipt and may finish its turn.
- **GIVEN** a duplicate same-turn send, **WHEN** it repeats the call, **THEN** the same `delivery_id` is returned and one target prompt is ultimately admitted.
- **GIVEN** exhausted automatic attempts, **WHEN** capacity remains unavailable, **THEN** the delivery is recoverable with its payload and last error intact.

## Out of scope

- Unlimited retry, capacity bypass for ordinary reports, or a frontend recovery panel.
