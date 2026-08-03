---
id: "01-step-contract-and-template"
title: "Workflow step persistence contract"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workflow/cancelled-turn-completion/spec.md"
---

# Task 01: Workflow step persistence contract

## Acceptance

- `cancel_triggers_turn_complete` is a persisted non-null workflow-step boolean with a replay-safe `false` database default and complete create/update/read coverage.
- Template loading, all template-instantiation paths, portable import/export, and workflow sync preserve the field; newly instantiated `simple` workflows enable it on `Backlog` and `In Progress` only.
- Existing workflow rows are not backfilled, and focused tests cover fresh SQLite schema, same-database replay, the env-gated Postgres path, custom defaults, and standard-template values.

## Verification

```bash
(cd apps/backend && go test -tags fts5 ./config/workflows ./internal/workflow/models ./internal/workflow/repository ./internal/workflow/service ./internal/task/repository/sqlite -run 'Test.*CancelTriggersTurnComplete' -count=1)
```

## Files Likely Touched

- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/models/export_test.go`
- `apps/backend/config/workflows/loader.go`
- `apps/backend/config/workflows/loader_test.go`
- `apps/backend/config/workflows/kanban.yml`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/repository/sqlite_test.go`
- `apps/backend/internal/task/repository/sqlite/workspace_bootstrap.go`
- `apps/backend/internal/task/repository/sqlite/builtin_workflow_test.go`
- `apps/backend/internal/task/repository/sqlite/postgres_schema_test.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/service/service_test.go`
- `apps/backend/internal/workflow/service/sync_apply.go`
- `apps/backend/internal/workflow/service/sync_apply_test.go`

## Dependencies

None.

## Parallelism

Sequential. This task owns the shared persisted model, migration, template, and portable contract required by every later task.

## Inputs

- Spec sections `What`, `Data Model`, `Persistence Guarantees`, and standard-template scenarios.
- ADR `2026-08-02-explicit-user-cancel-completion`.
- Existing `AutoAdvanceRequiresSignal` model/repository/import/export wiring as the nearest field pattern.
- ADR 0027 and backend schema replay guidance.

## Risks

- Missing any template-instantiation path creates different defaults depending on whether a workflow came from workspace bootstrap, empty-workflow repair, backend template creation, import, or sync.
- Do not use a data backfill or a database default of `true`; the product default belongs only to the embedded `simple` template.

## Output Contract

Report the field contract, migration behavior, exact template steps enabled, files changed, focused test results, blockers, and residual risks. Update this task and `plan.md` status in the same conversation.

## Results

Implemented the persisted `cancel_triggers_turn_complete` boolean with a replay-safe SQLite default of `false`, wired it through the workflow model, loader, standard Kanban template, SQLite repository, workspace/default template bootstraps, portable import/export, and sync reconciliation. The standard template enables it only for Backlog and In Progress; existing rows remain disabled after migration replay.

Verification:

- `rtk go test -tags fts5 ./config/workflows ./internal/workflow/models ./internal/workflow/repository ./internal/workflow/service -run 'Test.*CancelTriggersTurnComplete' -count=1` — 5 tests passed.
- `rtk go test -tags fts5 ./internal/task/repository/sqlite -run 'TestCreateWorkspaceWithKanban_CancelTriggersTurnCompleteDefaults' -count=1` — 1 test passed.
- `rtk go test -tags fts5 ./internal/workflow/service -run 'TestApplySyncedWorkflows_PreservesCancelTriggersTurnComplete' -count=1` — 1 test passed.
- `rtk go test -tags fts5 ./config/workflows ./internal/workflow/models ./internal/workflow/repository ./internal/workflow/service ./internal/task/repository/sqlite` — 392 tests passed across 5 packages.
- `rtk git diff --check` — passed.
