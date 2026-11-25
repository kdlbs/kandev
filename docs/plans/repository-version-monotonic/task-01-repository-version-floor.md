---
id: "01-repository-version-floor"
title: "Advance repository versions from the stored row"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.20
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.21
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.22
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.23
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Advance repository versions from the stored row

## Summary

Repository writes advanced `updated_at` from the caller's copy or from the wall
clock. A save built from a stale copy could repeat a version already committed,
and an exact or legacy complete write could move a future-dated version
backwards, weakening the optimistic-concurrency token used by repository
settings saves. Read the committed version inside the write transaction under
the row lock and advance from it.

## In scope

- `writeRepositoryCheckout` and the shared `updateRepository` writer read the
  stored `updated_at` under the transaction write lock (`SELECT ... FOR UPDATE`
  on PostgreSQL; SQLite serializes the writer) and apply the existing
  microsecond floor.
- `UpdateRepository` and `UpdateRepositoryIfUnchanged` run the read and the
  write in one transaction.
- Existing storage and service test files gain an overlapping-save regression,
  exact and legacy future-version regressions, and an env-gated PostgreSQL
  twin.

## Out of scope

- Other repository writers and any schema, public field, event-ordering, or
  advisory-lock framework change.
- Replacing the exact `ExpectedUpdatedAt` precheck or SQL predicate with the
  stored value.

## Acceptance

- Two saves that observed the same future version produce two distinct,
  strictly advancing committed versions.
- Exact `ExpectedUpdatedAt` saves still reject a stale expected value with the
  existing conflict outcome; a missing row keeps its existing outcome.
- The checkout intent, other assigned fields, response/event projection,
  rollback and error behavior, and `created_at` are unchanged.

## Verification

- Focused storage and service regressions pass, including the PostgreSQL twin
  when `KANDEV_TEST_POSTGRES_DSN` is set.
- Changed-delta review approved the implementation head; the documentation-only
  follow-up reuses that result.
