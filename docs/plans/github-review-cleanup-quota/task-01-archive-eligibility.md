---
id: "01-archive-eligibility"
title: "Make scheduled cleanup archive-aware"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.1
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.2
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.3
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.4
  - AC-INTEGRATIONS-GITHUB-REVIEW-CLEANUP-001.5
system_design:
  - ../../specs/integrations/system-design/github-review-task-cleanup.md
---

# Task 01: Make scheduled cleanup archive-aware

## Summary

Scheduled review cleanup stops polling archived tasks while keeping their
dedup records. Active tasks and orphan recovery still work, and explicit
watch actions retain their current archived-task behavior.

## In scope

- Add scheduled-only review-task selectors. Preserve the complete inventory
  selectors used by manual cleanup, watch deletion, and reset.
- Apply the selectors to enabled-watch cleanup and the global disabled-watch
  sweep. Keep the existing no-double-fetch rule for enabled watches.
- Check local Auto retention conditions before scheduled feedback. Fail closed
  when a local check fails.
- Update the review-watch cleanup paragraph in public integration docs.

## Out of scope

- Circuit admission, rate tracking, and issue-watch cleanup.
- Database migration or deletion of historical records.

## Acceptance

1. A mixed SQLite fixture with archived, active, empty-reservation, and
   missing-task records calls feedback only for eligible rows. The archived
   row remains after two enabled or disabled scheduled passes.
2. After unarchive, the same dedup row is eligible without a second task.
   Auto retention skips feedback when a user message or lifecycle prompt
   requires the task to remain, and a failed check keeps the row.
3. Manual cleanup, watch deletion, and reset continue to enumerate archived
   rows. Public docs distinguish routine cleanup from reset.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run 'Test(ListScheduledReviewPRTasks|CleanupMergedReviewTasks_Archived|CleanupAllOrphanedReviewTasks_Archived|CleanupReviewTasks_Unarchive|CleanupReviewTasks_AutoRetention|CleanupAllReviewTasks_Archived|ResetReviewWatch_Archived)' -count=1)
node scripts/validate-public-docs.mjs
git diff --check
```

## Files likely touched

- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/service_cleanup.go`
- `apps/backend/internal/github/service_cleanup_policy_test.go`
- `apps/backend/internal/github/review_pr_task_test.go`
- `apps/backend/internal/github/poller_test.go`
- `docs/public/integrations.md`

## Dependencies

None.

## Risks

Shared inventory filtering could change explicit destructive actions. Keep
scheduled eligibility separate from complete inventory.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-review-task-cleanup.md)
- [System design](../../specs/integrations/system-design/github-review-task-cleanup.md)
- Existing cleanup policy and reset tests in `internal/github`.

## Results

Added scheduled-only review-task selectors that exclude archived tasks while
retaining empty reservations and rows for hard-deleted tasks. Scheduled Auto
cleanup checks lifecycle prompts and user-authored messages before feedback;
explicit cleanup and reset continue to use complete inventories. Public
integration guidance now describes archive, unarchive, cleanup, and reset
behavior.

Verification passed: the targeted GitHub cleanup Go tests, both public-doc
validators, and `git diff --check`.
