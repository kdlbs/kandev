---
id: "01-schema-and-activation"
title: "Outcome columns, activation instant, and column projection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/pr-outcome-attribution/spec.md"
---

# Task 01: Outcome columns, activation instant, and column projection

Add the eight nullable columns to `github_task_prs`, stamp the durable
activation instant in `kandev_meta`, and extend every read projection and write
column list so the columns are visible without any backfill.

- **Acceptance:**
  1. A database lacking any of the eight columns gains them on startup via
     `ALTER TABLE ... ADD COLUMN` with no `NOT NULL` and no `DEFAULT`; a second
     startup completes without error and alters no row; a non-duplicate `ADD
     COLUMN` failure aborts startup with the error (AC-01, AC-02, AC-03).
  2. No `UPDATE` or backfill runs against `github_task_prs`; a pre-existing
     terminal row is `NULL` in all eight columns after two startups (AC-04).
  3. `kandev_meta` holds `github_task_pr_outcome_activated_at` in RFC 3339 form
     after the first migration that adds a column, and its value is unchanged by
     every later startup (AC-05, AC-06). All eight columns appear in
     `taskPRColumns`, `taskPRColumnsQualified`, `CreateTaskPR`, and
     `ReplaceTaskPR`; `UpdateTaskPR` gains only the five sync-owned ones (AC-07).
  4. The column group carries doc comments recording the writer-health
     invariants (AC-36, AC-37) and their activation-scoped limit (AC-39), the
     `NULL` versus `'unknown'` rule, the AC-15 `closed_by_login` gap, and the
     "latched observation, never a merge cause" note on
     `auto_merge_observed_at`, so a downstream extract author reads them beside
     the fields.

- **Verification:**
  ```
  cd apps/backend && gofmt -l internal/github internal/persistence && \
    go test ./internal/github/... ./internal/persistence/... && \
    make lint
  ```

- **Files likely touched:**
  - `apps/backend/internal/persistence/meta.go` — exported `ReadMetaKey`,
    `WriteMetaKeyIfAbsent` (`INSERT ... ON CONFLICT(key) DO NOTHING`).
  - `apps/backend/internal/persistence/meta_test.go`
  - `apps/backend/internal/github/store.go` — `createTablesSQL` DDL,
    `addTaskPROutcomeColumns` called from `initSchemaUpgrades`, activation stamp
    in `initSchema`, `taskPRColumns`, `taskPRColumnsQualified`, `CreateTaskPR`,
    `ReplaceTaskPR`, `UpdateTaskPR`, new `UpdateTaskPRDisposition`.
  - `apps/backend/internal/github/models.go` — eight `TaskPR` fields, the
    permitted-value set, `validTaskPRDisposition`, invariant doc comments.
  - `apps/backend/internal/github/store_task_pr_outcome_migration_test.go` (new)
  - `apps/backend/internal/github/store_taskpr_schema_drift_test.go` (extend)

- **Dependencies:** None.
- **Parallelism:** sequential — it is the only task touching `store.go`'s
  schema and every later task reads the columns it adds.

- **Inputs:**
  - Spec: Data model, AC-01 through AC-07, Persistence guarantees, Failure modes.
  - Plan: "Schema and activation (task 01)".
  - Patterns: fail-loud migration `addTaskCIRoundColumns` (`store.go:1008`) with
    `s.tableColumns` precheck; `dbutil.IsDuplicateColumnError` per
    `docs/decisions/0027-replayable-schema-migrations.md`; replay test shape in
    `internal/task/repository/sqlite/task_external_id_migration_test.go`.
  - Anti-pattern: `applyIdempotentSchemaColumns` (`store.go:484`) swallows every
    error with `_, _ = s.db.Exec(...)`. These columns must NOT go there — they
    enter `taskPRColumns`, so a silent failure turns every task-PR read into a
    scan error rather than a missing value.

- **Output contract:** summary of the migration path and where the activation
  stamp is written; files changed; the exact test commands run with pass counts;
  any blocker; risks; set this file's `status` and the `plan.md` checkbox in the
  same conversation.

## Results

**Status: done.**

Implemented exactly as planned, with one deliberate deviation from the
literal "gate the activation stamp behind the ALTER-added bool" wording:
`activateTaskPROutcomeTracking` calls `persistence.WriteMetaKeyIfAbsent` on
every boot rather than only when `addTaskPROutcomeColumns` performed a real
`ALTER`. Reason: a fresh install gets the eight columns inline via
`createTablesSQL`, not an `ALTER`, so gating on the ALTER-bool would leave
`github_task_pr_outcome_activated_at` permanently unset on every new
install — the opposite of AC-05's intent. `WriteMetaKeyIfAbsent`'s
`INSERT ... ON CONFLICT DO NOTHING` makes the call a no-op after the first
boot, so this is both simpler and correct for both fresh-install and
legacy-upgrade paths (AC-06 still holds).

Also added `persistence.EnsureMetaTable` (exported wrapper) since
`internal/github`'s own isolated unit tests construct a `Store` directly via
`NewStore`, bypassing `persistence.Provide`'s guarantee that `kandev_meta`
already exists — a cheap, idempotent `CREATE TABLE IF NOT EXISTS` call kept
that boot-order assumption from leaking into every existing `newTestStore`
call site.

**Files changed:**
- `apps/backend/internal/persistence/meta.go` — `ReadMetaKey`, `WriteMetaKeyIfAbsent`, `EnsureMetaTable`.
- `apps/backend/internal/persistence/meta_test.go` — write-once + read-absent tests.
- `apps/backend/internal/github/store.go` — DDL, `addTaskPROutcomeColumns`, `activateTaskPROutcomeTracking`, `taskPRColumns`/`taskPRColumnsQualified`, `CreateTaskPR`/`ReplaceTaskPR`/`UpdateTaskPR` column lists, new `UpdateTaskPRDisposition`, and the `migratePRTablesForMultiRepo` legacy-rebuild path (had to add the eight columns to that rebuild's `CREATE TABLE`/`INSERT...SELECT` too — see Results note below).
- `apps/backend/internal/github/models.go` — `TaskPR` gains the eight fields, `TaskPRDisposition*` constants, `validTaskPRDisposition`.
- `apps/backend/internal/github/store_task_pr_outcome_migration_test.go` (new)
- `apps/backend/internal/github/store_taskpr_schema_drift_test.go` (extended)
- `apps/backend/internal/github/store_task_pr_disposition_test.go` (new) — disjoint-writer pinning, behavioral not SQL-text-based (see rationale below).

**Unplanned fix required:** `migratePRTablesForMultiRepo`'s legacy-constraint
table rebuild (`github_task_prs` → `github_task_prs_new`) runs after
`addTaskPROutcomeColumns` in the boot sequence and previously dropped the
eight new columns on any database still carrying the pre-multi-repo
`UNIQUE(task_id, pr_number)` constraint — caught by the pre-existing
`TestLegacyTaskPRRebuildPreservesDetachedAt` test going red. Fixed by adding
the eight columns to both the replacement `CREATE TABLE` and the
`INSERT...SELECT` copy list, selected directly (not `COALESCE`) since
`addTaskPROutcomeColumns` already guarantees they exist on the source table
by that point in the boot sequence.

**Disjoint-writer pinning test — deviation from plan:** the plan asked for a
test asserting `UpdateTaskPR`'s SQL string contains no `disposition`
substring. Implemented as a behavioral test instead (write via one path,
assert the other path's columns are unaffected) — equally strong evidence of
the invariant, and doesn't require introspecting Go source text at runtime.

**Commands run:**
```
cd apps/backend && gofmt -l internal/github internal/persistence   # clean
go build ./...                                                     # ok
go test ./internal/github/... ./internal/persistence/... -count=1  # ok, 72.6s + 0.6s
golangci-lint run ./internal/github/... ./internal/persistence/... --timeout=5m
```
First lint run reported 2 false-positive `unused` warnings for
`validTaskPRDisposition`/`validTaskPRDispositions` (consumed by task 04,
same session) — resolved once task 04 landed. Also hit and fixed a
`nolint_filter` false positive: a doc comment that literally contained the
string `//nolint:cyclop` was misparsed by golangci-lint as a directive on
that comment line; reworded to avoid the literal token.

**Security/trust and external side effects:** None. Schema-only change to a
local SQLite database; no new external calls, no new permission surface.
