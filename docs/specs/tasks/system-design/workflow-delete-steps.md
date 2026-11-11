---
status: current
system: tasks
requirements:
  - REQ-TASKS-WORKFLOW-DELETE-STEPS-001
---

# Workflow delete removes steps system design

## Purpose and boundaries

The task SQLite repository deletes workflows, so it also deletes their steps.
The fix does not change which repository creates `workflow_steps` or when; it
makes the delete correct for the table that actually exists.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-WORKFLOW-DELETE-STEPS-001` | [Root cause](#root-cause), [Control flow](#control-flow), [Migration](#migration) |

## Root cause

The backend opens the task repository before the workflow repository.
`ensureRunnerProjectionTables` in `task/repository/sqlite/base_schema.go`
creates a `workflow_steps` stub without a foreign key so task queries that
project the step runner work in task-only stores. The workflow repository's
DDL in `workflow/repository/sqlite.go` declares
`FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE`, but it
uses `CREATE TABLE IF NOT EXISTS`, so it never runs on a database where the stub
already exists. `DeleteWorkflow` and `DeleteWorkflowsByWorkspace` assumed the
cascade. Workspace deletion was not affected because it deletes steps
explicitly.

## Components and responsibilities

- `Repository.DeleteWorkflow` deletes session bindings, then the workflow's
  steps, then the workflow row, in one transaction.
- `Repository.DeleteWorkflowsByWorkspace` deletes the steps of the selected
  workflows with the same predicate it uses for session bindings.
- `Repository.pruneOrphanWorkflowStepsOnce` runs from `runMigrations` after the
  projection table exists.

## Control flow

The step delete is a plain `DELETE ... WHERE workflow_id = ?` rebound for the
driver. Where the foreign key exists, the cascade then has nothing left to
remove. If the workflow row is missing, the transaction rolls back and the step
delete does not land.

## Migration

- Statement: `DELETE FROM workflow_steps WHERE workflow_id NOT IN (SELECT id FROM workflows)`.
- Marker: `kandev_meta` key `workflow_steps_orphans_pruned_at`, written with
  `ON CONFLICT(key) DO NOTHING` in the same transaction as the delete. A boot
  that finds the marker skips the delete.
- The delete is idempotent, so an interrupted boot repeats it safely.
- Both statements are portable between SQLite and PostgreSQL.

## Failure and recovery

A migration error fails repository startup like any other required migration.

## Residual risks

- Other tables created first by the task repository's projection stubs also
  lack their declared foreign keys. The broader fix (initialization order or a
  table rebuild) is left to a separate change.
- `workflow_step_participants` rows of a deleted step are not removed. The same
  stub ordering removes their cascade, and workspace deletion does not delete
  them either.
