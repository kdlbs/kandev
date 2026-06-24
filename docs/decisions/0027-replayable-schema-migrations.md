# 0027: Replayable schema migrations across SQLite and Postgres

**Status:** accepted
**Date:** 2026-06-24
**Area:** backend

## Context

Kandev runs schema initialization during backend startup. That path must be safe
on a fresh database and safe when replayed against an existing database.

Issue #1353 exposed a gap in that contract for Postgres deployments: the
worktree store added `task_session_worktrees.branch_slug` with an idempotent
SQLite-only duplicate-column check. SQLite reports duplicate `ADD COLUMN`
attempts as an error string containing `duplicate column name`; Postgres reports
the same condition as SQLSTATE `42701` (`duplicate_column`). The first boot
succeeded, but a redeploy replayed startup schema initialization and aborted on
the duplicate Postgres column.

SQLite passing is not enough for startup schema code that also runs on Postgres.

## Decision

Schema replay error classification lives in `internal/db`, not in individual
repository or store packages.

- Use `db.IsDuplicateColumnError(err)` for `ALTER TABLE ... ADD COLUMN` replay
  handling. It classifies SQLite duplicate-column strings and Postgres SQLSTATE
  `42701`.
- Use `db.IsAlreadyExistsError(err)` for broader migration logging where an
  existing table, index, column, or object means the statement has already been
  applied. It classifies the SQLite strings used by the legacy migration path and
  the relevant Postgres duplicate-object SQLSTATEs.
- Do not add new local `strings.Contains(err.Error(), ...)` migration classifiers
  in schema-owning packages. If a new dialect-specific replay case is needed,
  add it to `internal/db` with tests.

Every startup schema owner touched by a schema change must have replay coverage
in the same PR:

1. Initialize schema on a fresh SQLite database.
2. Initialize the same schema on the same SQLite database again.
3. If the schema owner supports Postgres, run the same fresh-plus-replay test
   against Postgres using `internal/testutil.OpenIsolatedPostgres`.
4. When another repository owns prerequisite tables, the test must use the real
   startup order instead of hand-creating a partial table.

## Consequences

**Easier:**

- Postgres and SQLite replay behavior is tested at the schema-owner boundary
  instead of inferred from one driver.
- Future migration code has one helper package to extend when dialects differ.
- Startup redeploy failures from already-applied schema changes are caught by
  focused package tests.

**Harder:**

- Schema tests need an extra same-connection replay step.
- Postgres replay coverage depends on `KANDEV_TEST_POSTGRES_DSN` being available
  in the environment running the integration tests.
- Existing startup schema owners that are not touched by this issue may still
  lack replay tests. Backfill them in focused follow-up work instead of
  broadening unrelated bug fixes.
- The current imperative migration style remains; this ADR does not introduce a
  numbered migration framework.

## Alternatives Considered

- **Keep local classifiers in each package.** Rejected because it caused this
  bug: the worktree store knew about SQLite but not Postgres.
- **Only add a Postgres case to the worktree store.** Rejected as too narrow;
  the same replay pattern exists in several startup schema paths.
- **Move immediately to a migration framework.** Deferred. A migration framework
  could improve ordering and history, but it is a larger refactor than needed to
  prevent this class of replay bug.
