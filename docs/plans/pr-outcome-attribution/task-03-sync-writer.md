---
id: "03-sync-writer"
title: "Reconcile outcome fields in syncs"
status: done
wave: 3
depends_on: ["02-upstream-field-sourcing"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/pr-outcome-attribution.md"
---

# Task 03: Sync writer

Persist populated observations through the existing sync service and preserve
stored values when a read did not observe the outcome fields.

## Implementation

- Reconcile all five fields in `service_pr_watch.go` with nil-safe comparisons.
- Preserve values for unpopulated reads and write `NULL` for observed absence
  where the upstream contract defines absence as a cleared value.
- Latch `auto_merge_observed_at` with a SQL-level `COALESCE` guard.
- Resolve replace and restore values inside their own write transactions.
- Re-read the committed row before publishing a task-PR update event.
- Apply the same logic to unwatched terminal reconciliation.
- Increment the populated and unpopulated expvar counters at the observation
  decision point.

## Verification

Sync, unwatched reconciliation, writer-health, detach, and metrics tests cover
presence flags, null preservation, latch behavior, event publication, and
concurrent replace/restore writes.
