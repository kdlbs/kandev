# ADR-2026-10-03-child-turn-stalled-producer-delivery: Classify child-turn stalls at the producer

**Status:** accepted
**Date:** 2026-10-03
**Area:** workflow

## Context

A child turn can settle without advancing its task step. The parent needs one
attributed alert for an actionable settlement, without waking for every ordinary
turn or waiting for a periodic monitor. The design compared classification at
the task-system producer with a durable candidate feed consumed by a monitor.
The supplied three-day turn sample does not establish the actionable rate or
actual token and slot costs.

## Decision

The owner selected producer-qualified direct parent delivery (option A) on
2026-10-03. The task-system producer classifies a live settlement using
structured evidence available at settlement time, suppresses ordinary turns,
and admits a qualified alert to the parent's existing queue. It retains the
settlement identity for deduplication. It preserves the normal parent queue's
authorization, capacity, and FIFO rules, and adds keyed retry and readback at
that admission seam. A cause requiring
unstructured interpretation remains an explicit unresolved diagnostic; this
decision does not authorize a paid classifier.

## Consequences

Classification is coupled to turn settlement and must preserve ordinary-turn
latency. Queue admission and the source-turn marker must survive retries and
ambiguous insertion without creating duplicate parent items. The design does
not add a candidate feed, monitor consumer, lease, or separate receipt table.
Operators still need bounded diagnostics for pre-queue failures and delivery
lag. Desktop and phone must expose the same attributed alert and child link.
The owner requested implementation on 2026-10-04.

## Alternatives Considered

- **Internal candidate feed with monitor classification (option B):** Separates
  classification from the producer and could reduce parent alerts if the
  monitor filters more candidates. It adds feed storage, consumer ownership,
  leases, lag, and retry boundaries. Its actual reduction and cost were not
  measured. The owner selected A; no other selection reason was stated.
