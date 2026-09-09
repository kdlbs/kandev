---
id: "01-persist-recipient-contracts"
title: "Persist initial recipient contracts"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.12
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Persist initial recipient contracts

## Summary

Prepare the initial-target contract and its durable provenance/retry storage.
This first slice has no source-step bindings or reference graph.

## In scope

- Nullable `session_target: {kind: initial}`; unknown kinds (including `step`
  in this slice) fail validation. Preserve omitted versus explicit null updates.
- REST and MCP target schema, decoding, forwarding, controller validation,
  response/event projection, and frontend types. Test create/update/clear parity.
- Workflow column and aligned task stub schema, replay migrations, built-in
  null defaults, YAML template loader mapping, import/export, duplication, sync.
- Version 2 for explicit initial targets and version 1 for legacy exports. Modify
  `WorkflowExport.Validate`, the shared import/sync version gate.
- Task metadata `workflow_initial_session` write-once snapshot and bounded
  `workflow_session_route` prepared/committed record. Add conditional,
  transaction-capable repository operations and task-duplication exclusions.
- Reopen, original-session deletion, compare-and-set, rollback, SQLite/PostgreSQL
  behavior, and existing required-store/conformance/upgrade coverage.

## Out of scope

- Source-step target support, binding table, earlier-position validation.
- Runtime wiring, UI controls, and public documentation.

## Acceptance

- REST/MCP preserve target values and partial-update semantics, reject conflicts,
  and emit equivalent events with explicit target test inputs.
- Templates, built-in initialization, portable version 1/2, duplication, and
  sync preserve initial intent and legacy lifecycle defaults.
- Snapshot writes are immutable; session deletion retains initial profile.
  Fresh insertion/prepared route and promotion/committed route have transactional
  repository seams. Stale writes and failures preserve unrelated metadata.

## Verification

Use TDD for new contract/repository behavior. From the repository root:

```bash
rtk make -C apps/backend sqlguard
```

From `apps/backend`, with an isolated `KANDEV_TEST_POSTGRES_DSN` for PostgreSQL:

```bash
rtk go test ./internal/workflow/... ./internal/workflowsync/... ./config/workflows/... -count=1
rtk go test ./internal/mcp/server -run 'Test.*WorkflowStep' -count=1
rtk go test ./internal/mcp/handlers -run 'TestWorkflowStep.*Parity' -count=1
rtk go test -race ./internal/task/repository/sqlite -run 'TestWorkflowSessionTarget|TestPostgresWorkflowSessionTarget|Test.*Workflow.*Default|Test.*Bootstrap' -count=1
rtk go test -race ./internal/persistence/storeconformance -count=1
rtk go test ./internal/backendapp -run '^TestPostgresBootInitializesRepositories$' -count=1
```

New metadata test names use `TestWorkflowSessionTarget` and
`TestPostgresWorkflowSessionTarget`. Include explicit initial-target payloads in
server and parity tests; old payloads are insufficient. Record PostgreSQL skips.

## Files likely touched

- `apps/backend/internal/workflow/models/{models.go,export.go,session_target.go}`
- `apps/backend/internal/workflow/{controller/controller.go,service/service.go,service/sync_apply.go,repository/sqlite.go}`
- `apps/backend/internal/mcp/server/{config_handlers.go,config_handlers_test.go}`
- `apps/backend/internal/mcp/handlers/{config_workflow_handlers.go,workflow_step_parity_test.go}`
- `apps/backend/config/workflows/loader.go` and loader tests
- `apps/backend/internal/task/repository/sqlite/{base_schema.go,base_migrations.go,defaults.go,workspace_bootstrap.go}`
- `apps/backend/internal/task/repository/sqlite/workflow_session_target.go` and tests (new)
- Task models, repository transaction interfaces, session deletion, and task duplication
- `apps/backend/internal/persistence/{requiredstores,storeconformance}/`
- `apps/web/lib/types/http.ts` and related workflow request/response types

## Dependencies

None.

## Risks

Existing tests do not automatically exercise new fields. The original marker
disappears with its session row unless the task snapshot is recorded first.
Unconditional metadata writes do not provide write-once or retry safety.

## Inputs

- Requirement 001.7 and the initial-target portions of requirement 002.
- Design: Explicit recipient contract; Initial provenance and entry routing.
- Existing policy round-trip tests, MCP parity tests, task metadata CAS helpers.
- `builtin_workflow_step_rows.go` only finds IDs; inspect without automatic edits.

## Parallelism

`sequential`

## Results

Implemented the initial target contract across models, REST, MCP, SQL schema,
workflow templates, export/import, duplication, and frontend types. Added the
initial metadata snapshot and bounded route record used by runtime routing.
Workflow, MCP, repository, SQL guard, and store-conformance checks passed.
