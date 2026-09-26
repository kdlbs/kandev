# ADR-2026-08-19-recoverable-cross-task-delivery: Persist Peer Delivery Before Admission

**Status:** accepted
**Date:** 2026-08-19
**Area:** backend, protocol, workflow

## Context

Task-to-task messages can be rejected while a target FIFO is full. Asking the source agent to retry makes delivery timing part of an LLM turn, duplicates reports, and can retain an otherwise completed source turn.

## Decision

Task-mode peer messages create or return a durable, idempotent delivery receipt before ordinary queue admission. Capacity saturation returns `pending_capacity`, not an LLM retry instruction. A bounded, lease-based worker promotes the retained payload, records reserve/acknowledgement outcomes, and leaves exhausted or nonretryable rows recoverable with their payload and error. A receipt whose executor acceptance occurred but whose terminal acknowledgement cannot be confirmed is retained as `ambiguous` and never automatically replayed.

The idempotency identity is caller supplied when present, otherwise a stable source-session/source-turn fingerprint. Workflow control deliveries use a committed transition identity and do not consume ordinary peer-message capacity.

## Consequences

Source turns can finish independently of target capacity, and restart/multi-instance recovery has an explicit record to claim. The queue gains durable delivery state and acknowledgement boundaries; delivery is not silently best effort.

## Alternatives Considered

Retry guidance alone remains unbounded and non-idempotent. Capacity bypass can starve ordinary users. Deleting a queued row before executor acceptance loses the only retained payload on transient failure.
