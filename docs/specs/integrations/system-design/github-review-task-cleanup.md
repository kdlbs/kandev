---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001
created: 2026-09-23
owners:
  - kandev
---

# GitHub review task cleanup system design

## Purpose and boundaries

`internal/github` owns review-watch deduplication, cleanup policy, and GitHub
feedback requests. The task repository owns `tasks.archived_at`. The cleanup
selector reads that state; it does not change task archive state. Shared
provider backoff remains a Platform contract.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001` | [Eligibility](#eligibility); [Cleanup flow](#cleanup-flow); [Explicit actions](#explicit-actions) |

## Eligibility

Add selectors for **scheduled** review cleanup in `internal/github/store.go`.
Select a review-task record when its `task_id` is empty, its task row is
missing, or its task row exists with `archived_at IS NULL`. Exclude a record
only when a matching task row is archived. Use a left join or an equivalent
predicate so missing tasks and incomplete reservations remain visible.

Keep `ListReviewPRTasksByWatch` and `ListAllReviewPRTasks` as complete inventory
methods. Watch deletion, reset, and explicit cleanup use those inventories.
Scheduled per-watch cleanup and the scheduled global orphan sweep use the new
eligible selectors. The global sweep still skips enabled-watch records that
the per-watch pass handled in the same cycle. A disabled watch remains eligible
through the global sweep.

The dedup row stays in storage while the task is archived. Unarchive makes the
same row eligible again, so the watch cannot create a duplicate task for that
pull request. No schema migration or backfill is required.

## Cleanup flow

Before a scheduled Auto feedback request, check the conditions that retain a
task regardless of PR state: an enabled lifecycle prompt and a user-authored
message. If either is present, skip feedback and keep the task and dedup row.
If either local check fails, fail closed and keep both. Keep Always delete and
Never behavior unchanged. Incomplete reservations have no task to inspect and
retain their terminal-PR cleanup path.

Eligibility comes from the database before client resolution where practical.
An empty eligible set does not resolve a credential or call GitHub. The poller
must check eligibility again on each scheduled pass; archive and unarchive
take effect without a service restart. A task archived after selection may
finish an already-started request, but later passes exclude it.

## Explicit actions

The manual all-workspace and workspace cleanup endpoints continue to inspect
archived records under each watch's policy. Watch deletion and reset continue
to enumerate archived tasks and apply their existing destructive behavior.
Do not route these actions through a scheduled-only selector.

## Failure and recovery

A missing task row remains visible to the cleanup path that removes stale
dedup records. A task-not-found result from the session checker counts as no
user-authored message so cleanup can remove its stale dedup record. Other
task-retention check failures leave both task and record intact. A watch read
failure leaves its records untouched for that cycle.
Provider failures follow the [Platform backoff design](../../platform/system-design/pr-watch-and-bounded-storage.md#provider-failure-backoff-and-health).

## Verification

SQLite-backed service and poller tests cover enabled and disabled watches,
archived and active tasks in one batch, incomplete and missing-task records,
unarchive, Auto retention, and explicit actions. A counting GitHub client
asserts feedback call counts over repeated scheduled cycles.
