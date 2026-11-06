---
id: "01-index-environment-bindings"
title: "Index delivery environment bindings"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERACTIVE-READS-008
acceptance_criteria:
  - AC-PLATFORM-INTERACTIVE-READS-008.1
  - AC-PLATFORM-INTERACTIVE-READS-008.2
  - AC-PLATFORM-INTERACTIVE-READS-008.3
system_design:
  - ../../specs/platform/system-design/interactive-read-availability.md
---

# Task 01: Index delivery environment bindings

## Summary

Install the environment/task index after the task-session migrations.
Use TDD to prove its access path and upgrade safety through production bootstrap.
Preserve the delivery query and equivalent results on both database engines.

## In scope

- Add a checked `CREATE INDEX IF NOT EXISTS` migration in `runMigrations`.
- Add focused SQLite lifecycle and production-query-plan regressions.
- Add isolated PostgreSQL lifecycle and mixed-binding result regressions.
- Run targeted delivery coverage, the SQL guard, and required store conformance.
- Record the actual commands, skips, query plans, and sentinel outcomes.

## Out of scope

- Query rewrites, replacement tables, unique constraints, and new dependencies.
- Probe deadlines, admission gates, delivery cadence, UI changes, and manual database repair.
- Regeneration of immutable previous-stable SQL fixtures without a release-baseline change.

## Acceptance

1. Fresh initialization, missing-index upgrades, and same-database replay create
   exactly one non-unique index with ordered keys `task_environment_id, task_id`.
   Older schemas without the environment column initialize successfully.
   Both legacy session rebuild paths retain the final index and sentinel data.
   Existing identical operator-created indexes replay successfully.
2. The production sweep query uses a covering search for `binding` on both keys
   in the analyzed SQLite fixture. Its maximum timestamp remains correct.
   Mixed matching and unrelated bindings preserve existential membership.
   Own environments with null provenance remain visible. Unrelated environments
   and deleted repository bindings cannot supply the winning observation.
   PostgreSQL preserves the same results and schema lifecycle.
3. Every required command passes after implementation. Record PostgreSQL skips
   explicitly and obtain real PostgreSQL results before marking the work done.
   Required migration failures continue to stop startup.

## Implementation notes

Add the index after `migrateTurnChangeHistorySchema` and before the final
`r.migrate.Err()` check in `base_migrations.go:runMigrations`.
Use `r.migrate.ApplyContext(ctx, ...)` and return unexpected errors.
No extra helper or index in pre-migration `initSessionSchema` is necessary.

Reuse `newInternalTestRepo` for the exact query and task bootstrap.
For the planner fixture, seed 3,500 environments, bindings, and snapshots.
Give the environments other owner tasks and a low-selectivity bound task.
Run `ANALYZE` before `EXPLAIN QUERY PLAN`.
Do not copy the query into the test or disable planner features.
Assert the binding search and both equality keys without freezing the full plan.

For lifecycle tests, use separate fresh databases for each legacy scenario.
Cover an existing final-shaped table without the index and a pre-column table.
Use legacy `workflow_step_id` and `agent_execution_id` columns to trigger each
existing table-rebuild migration through `NewWithDB`.
Preserve sentinel IDs, environment bindings, state, timestamps, and snapshot data
when those columns exist in the source shape.
For a pre-column row, assert the established environment-healing outcome instead
of expecting a value that the old schema cannot store.

Use `testutil.OpenIsolatedPostgres` for PostgreSQL coverage.
Inspect its index definition and replay the real constructor.
Do not require PostgreSQL to choose the SQLite plan on tiny data.
Keep existing result regressions in the targeted delivery suite.
The stable upgrade fixtures retain their historical bytes and checksums.
This additive index requires no new table inventory or manifest entry.

## Verification

Run from the repository root. The named new tests are implementation deliverables.
First run the regression commands before changing production code and record RED.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/delivery -run '^TestDeliveryGitSnapshotObservationUsesEnvironmentTaskIndex$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestTaskSessionEnvironmentTaskIndexFreshUpgradeReplay$' -count=1 -v)
```

After implementation, run these commands with `KANDEV_TEST_POSTGRES_DSN` set
to a disposable PostgreSQL server. The isolated-schema helper owns test cleanup.

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/delivery -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/repository/sqlite -run '^(TestTaskSessionEnvironmentTaskIndexFreshUpgradeReplay|TestPostgresTaskSessionEnvironmentTaskIndexFreshUpgradeReplay|TestSQLiteSchemaReinitializes|TestGitSnapshotEnvironmentMigration.*)$' -count=1 -v)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/base_migrations.go apps/backend/internal/task/repository/sqlite/task_session_environment_index_test.go apps/backend/internal/task/repository/sqlite/task_session_environment_index_postgres_test.go apps/backend/internal/delivery/sweep_query_plan_test.go apps/backend/internal/worktree/store.go apps/backend/internal/worktree/manager_cleanup_shared_reference_test.go
(cd apps/backend && go test -trimpath -race ./internal/worktree -run '^TestCleanupWorktrees_PreservesParentWorkspaceWhenInheritedSubtaskDeleted$' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/task/inventoryrepair -run '^TestLegacyAliasLifecycle$' -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/delivery-sweep-environment-index
```

Run the documentation coverage preflight from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = execFileSync('git', ['diff', 'HEAD', '--name-only', '-z'], {encoding: 'utf8'}).split('\0').filter(Boolean);
const added = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], {encoding: 'utf8'}).split('\0').filter(Boolean);
const paths = [...new Set([...changed, ...added])];
const referenced = [
  'docs/plans/delivery-sweep-environment-index/plan.md',
  'docs/plans/delivery-sweep-environment-index/task-01-index-environment-bindings.md',
  'docs/plans/delivery-sweep-environment-index/evidence.md',
  'docs/specs/platform/requirements/interactive-read-availability.md',
  'docs/specs/platform/system-design/interactive-read-availability.md',
];
const fileContents = {};
for (const p of [...new Set([...paths, ...referenced])]) {
  if (p.endsWith('.md') && fs.existsSync(p)) fileContents[p] = fs.readFileSync(p, 'utf8');
}
const changedFiles = paths.map(filename => ({filename, status: added.includes(filename) ? 'added' : 'modified'}));
const result = validateCoverage({changedFiles, fileContents});
console.log(JSON.stringify({status: result.status, errors: result.errors}, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

This command inspects the uncommitted implementation diff, including new tests.
Resolve all requirement and design references before marking the work done.
Repeat the diagnostic comparison only if the final plan differs from the
validated access path. Do not turn its observed latency into a CI timer.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/task_session_environment_index_test.go` (new)
- `apps/backend/internal/task/repository/sqlite/task_session_environment_index_postgres_test.go` (new)
- `apps/backend/internal/delivery/sweep_query_plan_test.go` (new)
- `apps/backend/internal/worktree/store.go`
- `apps/backend/internal/worktree/manager_cleanup_shared_reference_test.go`
- `docs/plans/delivery-sweep-environment-index/plan.md`
- `docs/plans/delivery-sweep-environment-index/task-01-index-environment-bindings.md`

## Dependencies

None. Existing bootstrap, legacy rebuilds, and test helpers provide the inputs.

## Risks

- An index before either session rebuild can disappear during that boot.
- A small planner fixture can select a scan despite a valid index.
- PostgreSQL DDL takes a startup lock. Avoid concurrent index syntax inside the migration flow.
- Available DSN credentials must allow isolated schema creation and cleanup.

## Parallelism

`sequential`

## Inputs

- [Availability requirements](../../specs/platform/requirements/interactive-read-availability.md), REQ-PLATFORM-INTERACTIVE-READS-008.
- [Availability design](../../specs/platform/system-design/interactive-read-availability.md#delivery-environment-lookup).
- [ADR 0027](../../decisions/0027-replayable-schema-migrations.md).
- [Investigation evidence](evidence.md).
- `apps/backend/internal/delivery/sweep.go:mostRecentGitSnapshotObservationQuery`.
- `apps/backend/internal/delivery/observe.go:SnapshotsForPair`.
- `apps/backend/internal/task/repository/sqlite/schema_replay_test.go` and `message_pending_index_test.go`.

## Results

Completed on 2026-11-05.

- Added the checked `idx_task_sessions_environment_task` creation after
  `migrateTurnChangeHistorySchema` and before the final required-migration
  check. Both legacy session-table rebuilds and the environment-column
  migration therefore precede it.
- RED was recorded before the production change: the delivery plan test
  rejected `SCAN binding`, and SQLite lifecycle coverage found no index.
- After the change, the analyzed SQLite plan is
  `SEARCH binding USING COVERING INDEX idx_task_sessions_environment_task (task_environment_id=? AND task_id=?)`.
  The fixture's exact maximum observation remains unchanged.
- SQLite coverage passed for fresh creation, replay, a missing index, an
  identical operator-created index, the workflow-step and agent-execution
  rebuilds, and a pre-column database. Sentinel IDs, environment bindings,
  state, timestamps, and snapshots survived; the pre-column binding healed to
  the task's environment.
- PostgreSQL coverage passed against a disposable server for fresh creation,
  replay, missing-index upgrade, an identical operator-created index, and a
  pre-column database. The mixed-binding eligibility fixture also passed on
  PostgreSQL.
- `go test -trimpath -tags fts5 -race ./internal/delivery -count=1` passed with
  `KANDEV_TEST_POSTGRES_DSN` set, so no PostgreSQL subtest skipped.
- The targeted SQLite race command passed with the same DSN.
- `go test -trimpath -tags fts5 ./internal/task/repository/sqlite -count=1`,
  `go build -tags fts5 ./...`,
  `go run ./cmd/sqlguard ./internal`,
  `go test -trimpath -tags fts5 -race ./internal/persistence/storeconformance -count=1`,
  `go test -trimpath -race ./internal/worktree -count=1`,
  `go test -trimpath -race ./internal/task/inventoryrepair -count=1`, and the
  file-scoped `bash scripts/lint-go-changed` (0 issues) passed.
- The fixup corrected task-scoped worktree inventory so a shared environment's
  borrower session cannot make owner cleanup skip its worktree. The focused
  worktree and inventory-repair regressions passed.
- Catalog validation, all specification lint, and `git diff --check` passed.
  The disposable PostgreSQL server was removed after verification.
