---
id: "07-narrow-to-upstream-scope"
title: "Narrow the feature to upstream observation only"
status: pending
wave: 6
depends_on: ["03-sync-writer"]
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 07: Narrow the feature to upstream observation only

Remove everything the 2026-08-15 spec amendment cut, and leave the five
upstream-observation columns, the clients that source them, the activation
instant, the no-backfill behaviour, and the writer-health signals working
exactly as they do today.

This is a **subtractive** task. Tasks 01, 02 and 03 stay done; nothing they
built for the five retained columns is re-opened. If a change here alters
observable behaviour of any retained column, that is a defect in this task, not
a consequence of the narrowing.

- **Acceptance:**
  1. `github_task_prs` carries no `disposition`, `disposition_superseded_by_url`
     or `disposition_recorded_at` in its DDL, in `taskPRColumns`, in
     `taskPRColumnsQualified`, or in any writer's column list; none appears on
     the task-PR JSON or the frontend `TaskPR` type (AC-40).
  2. No route matches `/api/v1/github/task-prs/:associationId/disposition`, and
     no service method, store method, permitted-value set, component, or
     translation key exists for recording a disposition (AC-41).
  3. `i18nGuardFiles` no longer lists the deleted component, and
     `check-guard-allowlist.mjs` passes (AC-41b).
  4. No `DROP COLUMN` or other destructive statement is emitted for the three
     removed columns (AC-42).
  5. The five retained columns still migrate, activate, sync, latch, and publish
     exactly as AC-01 through AC-19 and AC-30 require; their tests still pass
     unmodified except where a test file also asserted disposition behaviour.

- **Verification:**
  ```
  make -C apps/backend test
  make -C apps/backend lint
  cd apps/web && pnpm run typecheck && pnpm run lint && \
    pnpm run i18n:check && pnpm run i18n:ratchet
  ```
  E2E: none. The narrowed feature has no user-visible surface (AC-30b); the two
  disposition specs are deleted and not replaced.

## Removal surface (verified by `git grep` on 2026-08-15, not guessed)

### Backend — delete outright

- `apps/backend/internal/github/service_task_pr_disposition.go`
- `apps/backend/internal/github/service_task_pr_disposition_test.go`
- `apps/backend/internal/github/store_task_pr_disposition_test.go`
- `apps/backend/internal/github/controller_task_pr_disposition_test.go`

### Backend — edit

- `store.go` — three columns out of the `github_task_prs` DDL (~145-147) **and**
  out of the `migratePRTablesForMultiRepo` legacy-rebuild `CREATE TABLE` /
  `INSERT ... SELECT` (~615-617; task 01's Results records why that second site
  exists and it is easy to miss); out of `taskPRColumns` (~1357-1359) and
  `taskPRColumnsQualified` (~1373, ~1380); out of `CreateTaskPR` and
  `ReplaceTaskPR` (~1690, ~1697, ~1716, ~1727); delete
  `UpdateTaskPRDisposition` and the outcome-column migration list entry for the
  three. `UpdateTaskPR` already excludes them and must keep the five.
- `models.go` — three `TaskPR` fields, the `TaskPRDisposition*` constants, and
  `validTaskPRDisposition` / `validTaskPRDispositions`. Keep the five fields and
  the AC-15 / latch / invariant doc comments.
- `controller.go` — the route (line 75), `httpSetTaskPRDisposition` (~284-315),
  `parseTaskPRDispositionPatch` (~457-500), and the `DispositionPatch` type.
- `metrics_vars.go` / `metrics_vars_test.go` — `taskPROutcomeDispositionsTotal`
  and `incTaskPROutcomeDisposition`. Keep the syncs counter (AC-38).
- `store_taskpr_schema_drift_test.go` and
  `store_task_pr_outcome_migration_test.go` — narrow every eight-column
  assertion to five. **Do not weaken them**: they must still fail if a retained
  column drops out of a projection.

### Frontend — delete outright

- `apps/web/components/github/pr-disposition-row.tsx`
- `apps/web/components/github/pr-disposition-row.test.tsx`
- `apps/web/e2e/tests/pr/pr-disposition.spec.ts`
- `apps/web/e2e/tests/pr/mobile-pr-disposition.spec.ts`

### Frontend — edit

- `pr-ci-popover.tsx` — the import (line 20) and the `<PRDispositionRow>` mount
  (line 307).
- `pr-detail-panel.tsx` — the import (line 23), the mount (line 475), and the
  explanatory comment above it (~473). `pr-detail-panel.test.tsx` — the
  disposition assertions only; leave the rest of the file intact.
- `lib/types/github.ts` — the three fields (lines 282-286) and the
  `TaskPRDisposition` union (~289-291). **Keep the five fields and their
  `field?: T | null` shape**, and keep the inline comment explaining why they are
  optional (see task 05's surviving decision; making them required breaks 24
  test files).
- `lib/api/domains/github-api.ts` — `patchTaskPRDisposition`.
- `lib/state/slices/github/github-slice.test.ts` — the disposition assertions
  only.
- `src/locales/{en,pseudo,pt-pt,zh-cn}/github.json` — the 10 disposition keys in
  each. Regenerate pseudo with `pnpm run i18n:pseudo` rather than hand-editing.
- `eslint.i18n.options.mjs` — drop line 1410 (AC-41b).

### Do NOT touch — pre-existing, unrelated uses of the word

`git grep disposition` also hits the **code-review findings** feature and
several unrelated subsystems. None of these are in this branch's diff against
`origin/main`, and editing any of them is out of scope:
`internal/mcp/handlers/review.go`, `internal/task/service/review_service.go`,
`internal/task/service/service_sessions.go`, `internal/task/models/models.go`,
`internal/office/**`, `internal/orchestrator/**`, `internal/system/**`,
`internal/agentctl/**`, `apps/web/lib/types/review.ts`,
`apps/web/lib/api/domains/review-api.ts`, `apps/web/lib/desktop/external-links.ts`.
Verify with `git diff --stat $(git merge-base HEAD origin/main) HEAD` before
deleting anything a grep surfaced.

- **Inputs:**
  - Spec: the Amendment history, AC-40, AC-41, AC-41b, AC-42, AC-30, AC-30b, and
    the retired-number rule (AC-20..AC-29c and AC-31..AC-35 are retired and must
    not be reused).
  - The i18n guard's exact semantics, which decide point 3 above:
    `apps/web/scripts/check-guard-allowlist.mjs` flags a removed entry only while
    its path still resolves (line 90) and separately fails an entry matching no
    file (line 132). Deleting the component and dropping the entry together is
    the sanctioned path; either one alone fails the build.
  - Constraint: never weaken a test to make it pass. Where a test file asserts
    both retained and removed behaviour, remove only the removed assertions.

- **Output contract:** files deleted and files edited; exact commands run with
  counts; confirmation that the retained five columns' tests pass unmodified;
  blockers; risks; status update in this file and `plan.md`.

---

## ADDITIVE: the one non-removal change in this task (added 2026-08-16, AC-43)

Spec Review round 1 found a live AC-17 violation in shipped branch code. The human
directed it be fixed here rather than split out, so **this task is subtractive
except for this one item.**

**Defect.** `auto_merge_observed_at` is a latch: once non-`NULL` it must never be
cleared or overwritten (AC-17). `UpdateTaskPR` honours that with a SQL-level
`auto_merge_observed_at = COALESCE(auto_merge_observed_at, ?)`. But
`ReplaceTaskPR` (`apps/backend/internal/github/store.go`, the `DELETE` + `INSERT`
upsert) writes the column as a plain `tp.AutoMergeObservedAt` in the `INSERT`
column list. The inserted row is new, so `COALESCE` has nothing to read: a replace
whose caller struct carries `nil` **silently destroys a latched observation**, and
nothing in the `INSERT` looks wrong on inspection because the value is lost with
the deleted row rather than overwritten in place.

**Required change (AC-43).**
1. `ReplaceTaskPR` SHALL carry the deleted row's `auto_merge_observed_at` forward
   into the replacement row rather than trusting the caller's struct. Read the
   existing value inside the same transaction, before the `DELETE`, and prefer it
   when the caller's value is `nil`.
2. `RestoreTaskPR` (relink after detach, same file) SHALL follow the same rule. It
   already uses `COALESCE` for the latch and already takes pre-resolved values
   from `resolveTaskPROutcomeFields`; confirm both still hold after the narrowing
   edits and that its column list drops only the `disposition*` columns.
3. `CreateTaskPR` is unaffected: a genuinely new association has no prior latch.

**Note on AC-07.** The spec now names **four** writers of `github_task_prs` —
`CreateTaskPR`, `ReplaceTaskPR`, `RestoreTaskPR`, `UpdateTaskPR`. The earlier
"exactly one writer" claim was wrong and has been corrected. When you strip the
three `disposition` columns from writer column lists, strip them from all four,
not just the two the old text named.

**Tests (TDD, per the Build step).** Add a failing test first: seed a row with a
non-`NULL` `auto_merge_observed_at`, call `ReplaceTaskPR` with a struct whose
`AutoMergeObservedAt` is `nil`, and assert the stored value survives. It should
fail against current `main`-plus-this-branch before the fix. Mirror it for
`RestoreTaskPR`. These are the AC-43 surfaces the spec's Verification section now
lists.
