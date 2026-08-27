---
id: "01-resolutions-table"
title: "clarification_resolutions table and claim repository"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/external-question-answering.md"
---

# Task 01: `clarification_resolutions` table and claim repository

Create the durable claim row that makes bundle resolution exactly-once, plus the repository
methods that write and read it. Nothing consumes it yet — this task is the persistence floor.

- **Acceptance:**
  1. `clarification_resolutions` exists on a fresh database (via `initSchema`) and on an existing
     one (via `runMigrations`), with `pending_id` as primary key and `task_id` cascading from
     `tasks(id)`.
  2. `InsertClarificationResolution` returns `claimed=true` for the first insert and
     `claimed=false` plus the stored row for every later insert of the same `pending_id`, with the
     stored row unmodified.
  3. Deleting the owning task removes its resolution rows.

- **Verification:**
  ```
  cd apps/backend && \
    gofmt -l internal/task/repository/sqlite internal/task/models && \
    go test -race ./internal/task/repository/sqlite/... && \
    go build ./...
  ```
  Postgres coverage (skipped without a DSN, run it if one is available):
  ```
  cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" \
    go test -race -run 'Clarification' ./internal/task/repository/sqlite/...
  ```

- **Files likely touched:**
  - `apps/backend/internal/task/repository/sqlite/base_schema.go` (new
    `clarificationResolutionsSchemaDDL`, `initClarificationResolutionsSchema` step appended to the
    `initSchema()` step table after `r.initTaskReviewSchema`; third index in
    `ensureMessageMetadataIndexes` at `:92`)
  - `apps/backend/internal/task/repository/sqlite/base_migrations.go` (`migrateClarificationResolutions`,
    called from `runMigrations()`)
  - `apps/backend/internal/task/models/clarification_resolution.go` (new)
  - `apps/backend/internal/task/repository/sqlite/clarification_resolution.go` (new)
  - `apps/backend/internal/task/repository/interface.go`
  - `apps/backend/internal/task/repository/sqlite/clarification_resolution_test.go` (new)
  - `apps/backend/internal/task/repository/sqlite/schema_replay_test.go`

- **Dependencies:** None.
- **Parallelism:** `parallel-safe` with task 06 (`internal/mcp/handlers`) and task 08
  (`apps/web/lib/settings`) — no shared files. Not parallel with 02, which extends the same
  repository package and reads this table.

- **Inputs:**
  - Spec § *Data model* (`clarification_resolutions`), D4, R1, R2, R6.
  - Plan § *Backend → Schema* for the exact DDL, including the `resume` column the spec's table
    omits; also update the spec's data-model table to carry it.
  - `apps/backend/AGENTS.md` § *Schema & migrations* — the init-before-migrations ordering rule and
    the replay-test requirement.
  - Pattern to copy: `taskReviewSchemaDDL` / `initTaskReviewSchema`
    (`base_schema.go:925-978`) for a task-cascading table; `attachment.go` for a
    conflict-tolerant write.
  - `dialect.JSONExtract` + `r.ro.Rebind` are mandatory for the index expression; do not hand-write
    SQLite JSON syntax.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented in commit `a29c998e0`. Added `clarification_resolutions` (PK `pending_id`, `task_id`
cascading from `tasks(id)`) to both `initSchema` (`base_schema.go`) and `runMigrations`
(`base_migrations.go`), plus `InsertClarificationResolution` in
`internal/task/repository/sqlite/clarification_resolution.go` returning `claimed=false` and the
stored row on every insert after the first.

Verified in this session's final gauntlet (Wave 10): `go test ./internal/task/repository/sqlite/...`
passes, including `clarification_resolution_test.go` (first-insert-wins semantics, task-cascade
delete) and `schema_replay_test.go` (fresh-DB vs migrated-DB schema parity). `go build ./...`
succeeds. `gofmt -l` reports no files. The one wave-1-specific case not independently re-run in this
session is the optional Postgres-DSN coverage (`KANDEV_TEST_POSTGRES_DSN`), which requires an external
Postgres instance not available in this environment — SQLite coverage (the required path) is green.

No external side effects. No cleanup/teardown artifacts beyond normal test databases in `t.TempDir()`.
</content>
