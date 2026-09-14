---
status: current
system: platform
requirements:
  - REQ-PLATFORM-TASK-SUMMARY-CONTENTION-001
---

# Task status summary projection contention System Design

## Purpose and boundaries

The platform system owns the shared task status summary projector. This design
extends [bounded task status delivery](bounded-task-status-delivery.md) with
the projection contention contract. GitHub watch identity and event
idempotency are owned by the integrations system.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-TASK-SUMMARY-CONTENTION-001` | Components; Backoff; Sustained load |

## Components

- `apps/backend/internal/task/statussummary/projector.go`: per-task
  in-process lock (`lockTask`), pending-refresh single-flight coalescing,
  semantic-equal short-circuit (`TaskStatusSummary.SemanticEqual`), and
  bounded-jittered compare-and-set retry backoff
  (`defaultCASRetryBackoff`, `casRetryJitter`, injectable
  `ProjectorConfig.RetryBackoff`).
- `apps/backend/internal/task/statussummary/projector_contention_test.go`:
  concurrency harnesses for distinct and equivalent concurrent events under an
  alternating-reject store, plus backoff bounds, growth, cap, and cancellation
  unit coverage.
- `apps/backend/internal/task/statussummary/projector_load_test.go`:
  sustained-load fixture scaling contention across many tasks with a per-task
  alternating-reject store, asserting zero exhausted-retry handler errors and
  cumulative final summaries.

## Backoff

Rejected compare-and-set attempts wait a small capped exponential delay with
up to +25% jitter before the next retry, honoring context cancellation. The
attempt bound is unchanged; only pacing is added, so tests inject a no-op
backoff to stay instantaneous while exercising the retry path.

## Rebase

On rejection, the writer reloads the authoritative row (via
`rebaseProjectionStateFromCurrent`) and re-derives from it before the next
attempt, never persisting from stale state.

## Sustained load

The load fixture models an external unsynchronized writer (boot
reconciliation or HTTP rebuild analog) continuously racing the live
projector. Per-task rejection guarantees each task's attempts observe
consistent ordering; one global counter would invite cross-task interleaving
artifacts unrelated to the code under test.

## Related decisions

- [Keep PR watches task owned](../../../decisions/2026-08-31-task-owned-pr-watch-identity.md)
