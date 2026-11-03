---
id: "01-delete-workflow-steps"
title: "Delete workflow steps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-DELETE-STEPS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-DELETE-STEPS-001.1
  - AC-TASKS-WORKFLOW-DELETE-STEPS-001.2
  - AC-TASKS-WORKFLOW-DELETE-STEPS-001.3
  - AC-TASKS-WORKFLOW-DELETE-STEPS-001.4
system_design:
  - ../../specs/tasks/system-design/workflow-delete-steps.md
---

# Task 01: Delete workflow steps

## Summary

Delete a workflow's steps inside the workflow delete transaction and prune
existing orphan steps once on upgrade.

## In scope

- `DELETE FROM workflow_steps WHERE workflow_id = ?` in `DeleteWorkflow`.
- The same delete for the selected workflows in `DeleteWorkflowsByWorkspace`.
- `pruneOrphanWorkflowStepsOnce` called from `runMigrations`.

## Out of scope

- Initialization order, foreign keys, and participant rows.

## Acceptance

- Deleting a workflow removes its steps and keeps other workflows' steps.
- A not-found delete removes no steps.
- The bulk delete removes only the deleted workflows' steps.
- The first boot without the marker removes orphan steps and writes the marker;
  a later boot keeps existing steps.

## Verification

```bash
# From apps/backend:
go test -trimpath -tags fts5 -p=1 ./internal/task/repository/sqlite -run '^(TestDeleteWorkflowRemovesOnlyItsStepsAndBindings|TestDeleteWorkflowMissingRollsBackStepAndBindingDeletes|TestDeleteWorkflowsByWorkspaceRemovesOnlySelectedSteps|TestRunMigrationsPrunesWorkflowStepOrphansOnce|TestPostgresWorkflowStepOrphanPruneStartsAndSerializes)$' -count=1
# From the repository root:
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/workflow.go`
- `apps/backend/internal/task/repository/sqlite/workflow_step_orphan_migration.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`

## Dependencies

- None.

## Risks

- See the plan.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/workflow-delete-steps.md)
- [System design](../../specs/tasks/system-design/workflow-delete-steps.md)

## Original source results

These results belong to the original integrated source. The independent port
keeps the source history but does not import permanent tests or fixture changes.


- RED at base `e29a5b364` (clean detached worktree, new tests copied in
  unchanged): `TestDeleteWorkflow_RemovesItsSteps` (2 steps left, want 0),
  `TestDeleteWorkflowsByWorkspace_RemovesTheirSteps` (1 left, want 0) and
  `TestMigration_PrunesOrphanWorkflowStepsOnce` (2 orphan steps, want 0)
  failed. `TestDeleteWorkflow_NotFoundKeepsSteps` passed before and after, as
  the rollback control.
- GREEN from `apps/backend`:
  `go test -trimpath -tags fts5 -p=1 ./internal/task/repository/sqlite -run 'TestDeleteWorkflow_|TestDeleteWorkflowsByWorkspace_RemovesTheirSteps|TestMigration_PrunesOrphanWorkflowStepsOnce|TestPostgresDeleteWorkflowRemovesStepsAndPrunesOrphans' -count=1`
  passed (4 tests, `ok ... 1.815s`).
  `TestPostgresDeleteWorkflowRemovesStepsAndPrunesOrphans` was skipped:
  `KANDEV_TEST_POSTGRES_DSN` is unset and no local PostgreSQL is available.
- The original source also updated upgrade fixtures. Those fixture and manifest
  changes are excluded from this port; no new fingerprint checks are added.
- `gofmt -l` on the touched files: no output.
  `bash scripts/lint-go-changed <files>`: 0 issues.
- `python3 scripts/list-docs.py validate` (392 decisions, 1670 specifications)
  and `python3 scripts/lint-spec-files.py --all` passed.

## Upstream port validation

The independent port restored the pre-change delete/migration source through
an explicit disposable overlay. Single delete, workspace bulk delete and orphan
cleanup failed with retained steps (0.868s); the not-found rollback control
passed.

The candidate passed all four SQLite cases under race detection (2.679s).
They cover other-workflow and excluded-workflow preservation, missing-row
rollback, first-boot pruning, live steps and marker readback. The temporary
migration case also inserts a later orphan after the marker and reopens again,
confirming that cleanup does not repeat. PostgreSQL was skipped because
`KANDEV_TEST_POSTGRES_DSN` is unset; no PostgreSQL pass is claimed.

The existing SQL guard passed using its normal whole-internal scan. A first
attempt limited to three changed files was rejected as an unused registered
exemption in another package; its registry and criteria were left unchanged.
The existing documentation coverage evaluator returned covered with no errors.
No upgrade fixture, manifest, standing test or operating database was changed.
Temporary checks are removed after delivery.

## Review follow-up: concurrent PostgreSQL startup

The PostgreSQL read-then-write migration now takes the existing transaction-scoped
advisory-lock pattern before reading its marker. Concurrent starters must observe
the first committed marker before deciding whether cleanup is still needed.
The lock, cleanup and marker share transaction ownership; SQLite keeps its
existing serialization. Production comments describe only this invariant.

The disposable existing SQLite migration case passed with race detection
(1.490s), and the existing whole-internal SQL guard passed. The conditional
PostgreSQL multi-connection holder/marker case was skipped because the existing
DSN is unset; no real PostgreSQL concurrency pass is claimed. The source was
reviewed against the existing dialect and transaction-lock pattern, with the lock
before the first marker read. No standing test, new setting or operating DB
write is added; the temporary cases are removed after delivery.

## CI regression follow-up: v0.93.0 fixture workflow row

`TestPreviousStableUpgrade/{sqlite3,pgx}/v0.93.0` failed after the one-time
prune because the v0.93.0 fixture seeded the `workflow_steps` row
`fixture-v0930-workflow-step` with `workflow_id='fixture-v0930-workflow'` but
never seeded the matching `workflows` row. The prune correctly removed that
orphan, and the fixture's preserved owner-row sentinel then found no step.

The fixture now seeds the missing workflow row in both SQL dumps, and the
manifest checksums are updated to the values from the original integrated
source. The prune behavior is unchanged: a real orphan is still deleted once,
and live workflow steps are still preserved.

Minimal regressions were added to existing test files:

- `TestDeleteWorkflowRemovesOnlyItsStepsAndBindings`
- `TestDeleteWorkflowMissingRollsBackStepAndBindingDeletes`
- `TestDeleteWorkflowsByWorkspaceRemovesOnlySelectedSteps`
- `TestRunMigrationsPrunesWorkflowStepOrphansOnce`
- `TestPostgresWorkflowStepOrphanPruneStartsAndSerializes` (env-gated)

The maintainer's proposed new test file was not applied wholesale. The
PostgreSQL case lives in the existing startup migration test file and reuses
the existing isolated multi-connection helper and DSN gate.

### Verification (2026-10-29)

- On main `173d1ef`: the same storeconformance case passed all sentinel checks
  and failed later in the storage scenario on Windows only
  (`temporary artifact path must be an absolute path`); the scenario seeds
  `/tmp/<id>`, which is not absolute on Windows.
- On PR head `5b82e96` before the fixture fix: the test failed at
  `upgrade_test.go:183` with the reported sentinel error.
- After the fix: the test passed the sentinel and replay checks and again
  reached the same Windows-only storage scenario failure, so the sentinel
  regression is gone while the unrelated platform limitation remains.
- `TestUpgradeFixtureManifest`: pass.
- The five regressions above: four SQLite cases pass with race detection. The
  PostgreSQL case first runs `runMigrations` to prove the startup path reaches
  the prune, then exercises two concurrent helper calls under the advisory
  lock; it skips locally because `KANDEV_TEST_POSTGRES_DSN` is unset. CI runs
  it with a DSN.
- `go run ./cmd/sqlguard ./internal`: pass.
- `scripts/lint-go-changed` on the changed Go files: 0 issues. A shared
  golangci-lint lock from another run required the equivalent
  `--allow-parallel-runners` invocation.
- Normal pre-commit hooks were used for the commit.

## Review follow-up: step creation serializes with workflow deletion

A later review identified a PostgreSQL race: step creation could insert a step
after a delete removed existing steps but before the workflow row was deleted,
leaving a persistent orphan after the one-time marker. The race predates this
PR, but it violates the current delete/prune invariant and the PostgreSQL
read-modify-write guideline.

The fix uses the workflow row as the serialization point:

- `DeleteWorkflow` locks the workflow row with `SELECT ... FOR UPDATE` on
  PostgreSQL before deleting bindings and steps.
- `DeleteWorkflowsByWorkspace` locks the affected workflow rows in id order with
  the same predicate before deleting bindings and steps.
- The step writer locks and validates the owning workflow row before demoting
  start steps or inserting, and rejects a missing workflow
  (`ErrWorkflowNotFound`) or a stale expected version
  (`ErrTaskVersionConflict`).
- SQLite keeps its immediate writer serialization and uses the same existence
  and version checks. No foreign-key rebuild and no new lock framework are
  added.

Regression coverage:

- `TestCreateStepForMissingWorkflowIsRejected` checks the SQLite rejection.
- `TestPostgresWorkflowStepCreateSerializesWithDelete` holds the workflow row,
  blocks `DeleteWorkflow` and `CreateStep` on the same row, releases it, and
  asserts the delete succeeds, the create returns `ErrWorkflowNotFound`, and no
  steps remain. It skips locally without `KANDEV_TEST_POSTGRES_DSN`; CI runs
  it.
- The service test mocks mirror workflow creation and deletion into the real
  test database so they exercise the same invariant as production.

`go run ./cmd/sqlguard ./internal` passed, `scripts/lint-go-changed` reported
0 issues, and the focused SQLite tests passed with race detection. The workflow
repository package passed with the pre-existing
`TestResolveCurrentRunner_FallsBackToLatestTaskRunner` timing flake excluded;
that test fails intermittently on the same `created_at` value and is unrelated
to this change. The workflow service package passed in full.

