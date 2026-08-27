---
id: "01-preserve-branch-template"
title: "Preserve saved worktree branch templates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/worktree-branch-templates.md"
---

# Task 01: Preserve Saved Worktree Branch Templates

## Acceptance

- A non-empty `worktree_branch_template` survives each startup schema replay.
- A legacy row receives `<trimmed-prefix>{title}-{suffix}` exactly once.
- An empty legacy prefix produces `feature/{title}-{suffix}`.
- SQLite and Postgres implement the same behavior.
- Repository creation and update contracts remain unchanged.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite -run 'Test(WorktreeBranchTemplateMigration|PostgresWorktreeBranchTemplateMigration)' -count=1`
- `cd apps/backend && go test -tags fts5 ./internal/task/service -run 'TestService_(CreateRepository_DefaultWorktreeBranchTemplate|UpdateRepository_WorktreeBranchTemplate)' -count=1`

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/worktree_branch_template_migration_test.go`
- `apps/backend/internal/task/repository/sqlite/worktree_branch_template_migration_postgres_test.go`
- `docs/specs/workspaces/requirements/worktree-branch-templates.md`
- `docs/specs/INDEX.md`
- `docs/plans/worktree-branch-template-persistence/plan.md`
- `docs/plans/worktree-branch-template-persistence/task-01-preserve-branch-template.md`

## Dependencies

None.

## Parallelism

Sequential. The production migration and both dialect tests share one schema
boundary.

## Inputs

- The persistence guarantees and scenarios in the linked specification.
- The configurable branch-name contract in
  `docs/decisions/0032-configurable-worktree-branch-names.md`.
- The schema replay rules in
  `docs/decisions/0027-replayable-schema-migrations.md`.
- The confirmed issue #2611 reproduction. A custom value became
  `feature/{title}-{suffix}` after one `runMigrations` call.

## Output contract

Report the changed files, each command and result, remaining risks, and synchronized
task and plan statuses in the primary session.

## Results

- RED: the focused SQLite migration command reproduced the overwrite of a
  custom template during schema replay.
- GREEN: SQLite migration regressions passed.
- Postgres migration regression is implemented and skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` is unset.
- Existing repository create and update service checks passed.
- `git diff --check` passed.
- Fixup added a second replay to the custom-template regression; both focused
  migration and service commands passed again.
