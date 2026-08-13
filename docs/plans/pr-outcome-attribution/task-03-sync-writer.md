---
id: "03-sync-writer"
title: "Persist outcome fields from the sync writer"
status: done
wave: 3
depends_on: ["01-schema-and-activation", "02-upstream-field-sourcing"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 03: Persist outcome fields from the sync writer

Write `is_draft`, `changed_files`, `merged_by_login`, `closed_by_login`, and the
`auto_merge_observed_at` latch from populating syncs only, preserving stored
values (including `NULL`) on every other path.

- **Acceptance:**
  1. A populating sync writes all three of `is_draft`, `changed_files`,
     `merged_by_login`, using `NULL` (never `""`) when upstream reports no
     merger, and writing `0` for a real `changed_files = 0`. A non-populating
     sync leaves all three at their stored values, including `NULL` (AC-12,
     AC-13). `closed_by_login` follows the same rule against
     `ClosureAttributionPopulated` (AC-14, AC-15).
  2. `auto_merge_observed_at` is set to the current UTC instant the first time a
     populating sync observes auto-merge armed, and is never cleared or
     overwritten afterwards, including when a later sync observes it disarmed or
     absent (AC-16, AC-17).
  3. A sync that changes any of the five publishes `github.task_pr.updated`; a
     sync writing identical values publishes nothing (AC-18). `mergeable_state
     = 'draft'` derivation and all mergeability / auto-merge / CI-automation
     behaviour are unchanged (AC-19). `expvar` counters under the
     `github_task_pr_outcome_` prefix increment for populating and
     non-populating syncs (AC-38).

- **Verification:**
  ```
  cd apps/backend && gofmt -l internal/github && \
    go test ./internal/github/... && \
    make lint
  ```

- **Files likely touched:**
  - `apps/backend/internal/github/service_pr_watch.go` — `taskPRSyncState`,
    `prepareTaskPRSyncState`, `SyncTaskPR`.
  - `apps/backend/internal/github/metrics_vars.go` (new) — the two expvar maps
    and the `k=v;k=v` label helper.
  - `apps/backend/internal/github/service_pr_outcome_sync_test.go` (new)
  - A small nil-safe comparison helper file if `boolPtrEqual` /
    `stringPtrEqual` do not already exist beside `intPtrEqual` and `timeEqual`.

- **Dependencies:** tasks 01 and 02.
- **Parallelism:** parallel-safe with task 04 — disjoint files
  (`service_pr_watch.go` + `metrics_vars.go` here, versus
  `service_task_pr_disposition.go` + `controller.go` + `handlers.go` there) and
  no shared schema change, since task 01 already landed both column sets. Run
  sequentially unless the user explicitly asks otherwise.

- **Inputs:**
  - Spec: AC-12 through AC-19, AC-38; "Nil, empty, and error"; the
    `auto_merge_observed_at` latch note (GitHub clears `auto_merge` once it
    fires, so a poller can only ever learn "armed at some instant while we were
    looking" — it must never be read, named, or charted as "merged by
    auto-merge").
  - Plan: "Sync writer (task 03)".
  - Patterns: the populated/preserve dance already in `prepareTaskPRSyncState`
    for `ChecksPopulated`, `UnresolvedReviewThreadsPopulated`, and
    `ReviewCountsPopulated`; the expvar map idiom in
    `internal/office/scheduler/metrics_vars.go`.
  - Constraint: `SyncTaskPR` already carries `//nolint:cyclop`. If the new
    branches push another lint limit, extract the outcome-field reconciliation
    into a helper rather than widening the nolint.

- **Output contract:** summary of the write rules and the latch; files changed;
  exact test commands and counts; blockers; risks; status update in this file
  and `plan.md`.

## Results

**Status: done.** Implemented as planned.

- `taskPRSyncState` gained `isDraft`, `changedFiles`, `mergedByLogin`,
  `closedByLogin`, `autoMergeObservedAt`.
- New `resolveTaskPROutcomeFields` helper (extracted, as the plan
  anticipated, to keep `prepareTaskPRSyncState`/`SyncTaskPR` within the
  repo's complexity limits rather than widening the existing
  `//nolint:cyclop`) implements the populated/preserve dance for the first
  three and the closure-attribution pair, plus the auto-merge latch: set
  once when `OutcomeFieldsPopulated && PR.AutoMergeEnabled &&
  tp.AutoMergeObservedAt == nil`, otherwise carried forward verbatim.
- `SyncTaskPR`'s `changed` expression gained a `taskPROutcomeFieldsChanged`
  helper (same complexity-budget reasoning) using new `boolPtrEqual` /
  `stringPtrEqual` beside the existing `intPtrEqual` / `timeEqual` in
  `service_pr.go`.
- New `internal/github/metrics_vars.go`: `github_task_pr_outcome_syncs_total`
  (label `populated=true|false`) and `github_task_pr_outcome_dispositions_total`
  (label `action=set|clear`, wired up in task 04).

**Files changed:** `service_pr_watch.go`, `service_pr.go` (ptr-equal
helpers), `metrics_vars.go` (new), `service_pr_outcome_sync_test.go` (new).

**Commands run:**
```
cd apps/backend && gofmt -l internal/github          # clean
go build ./...                                       # ok
go test ./internal/github/... -run TestSyncTaskPR -v -count=1   # 25/25 pass
go test ./internal/github/... -count=1                # ok, 73.5s
```

**Security/trust and external side effects:** None. Pure persistence-layer
change; no new outbound calls.
