---
id: "02-portable-contracts"
title: "Carry portable completion settings"
status: done
wave: 2
depends_on: 
  - 01-persistence
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-COMPLETION-001.4
  - AC-TASKS-COMPLETION-001.7
  - AC-TASKS-COMPLETION-001.8
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 02: Carry portable completion settings

## Summary

Expose the persisted setting through all workflow creation and transport boundaries. Make version-1 compatibility conversion explicit while current exports use version 2.

## In scope

- Carry the field through models/StepDefinition, loader, task DTO, workflow controller, REST/WS handlers, stepevents, and MCP schemas/handlers. Omitted PATCH preserves; explicit false disables; null is rejected.
- Normalize version-1 omitted fields at whole-workflow import/sync boundaries using the legacy terminal predicate. Version 2 requires explicit booleans; current exports include false and do not omit it.
- Update template conversion and each built-in YAML with explicit values. Update sync equality, invalid-file handling, embedded config-context instructions, and payload parity tests.

## Out of scope

Do not change runtime completion authority or frontend editing yet.

## Acceptance

- Create/update/list/boot/event/MCP projections agree, including omitted update and explicit false.
- Both portable versions import correctly; export produces version 2. Sync create/update/no-op preserves explicit false and reports invalid version-2 input without destructive reconciliation.
- Templates and new workspace bootstrap preserve each built-in workflow's previous completion intent.

## TDD entry

Add TestCompletionPortableVersions and TestCompletionSyncPreservesFalse in workflow model/service completion_portable_test.go files. Start with a version-1 omitted field on a final Done and a current export containing explicit false; assert the resulting behavior, not just structure.

## Verification

Run from `apps/backend/`. PostgreSQL-dependent cases use an isolated `KANDEV_TEST_POSTGRES_DSN`; never point tests at the reported installation.

```bash
rtk go test -race ./config/workflows ./internal/workflow/models ./internal/workflow/controller ./internal/workflow/handlers ./internal/workflow/service ./internal/workflow/stepevents ./internal/task/dto ./internal/workflowsync -count=1
rtk go test -race ./internal/mcp/handlers ./internal/mcp/server -run 'Test.*(Workflow|Step|Config)' -count=1
rtk go test -race ./internal/task/repository/sqlite -run 'Test.*(Builtin|Workflow|Bootstrap)' -count=1
```


## Files likely touched

- `apps/backend/config/workflows/loader.go`
- `apps/backend/config/workflows/*.yml`
- `apps/backend/config/prompts/config-context.md`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/models/completion_portable_test.go (new)`
- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/service/sync_apply.go`
- `apps/backend/internal/workflow/service/completion_portable_test.go (new)`
- `apps/backend/internal/workflow/stepevents/stepevents.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/backend/internal/task/dto/converters.go`
- `apps/backend/internal/mcp/handlers/config_workflow_handlers.go`
- `apps/backend/internal/mcp/server/config_handlers.go`
- `apps/backend/internal/workflowsync (portable decode callers)`

## Dependencies

Complete 01-persistence first.

## Risks

A bool alone cannot distinguish absent from explicit false/null at compatibility boundaries. Final position establishes eligibility only; it must never default the setting to true. Retain inactive non-final values across round trips. Refresh affected test fixtures using ExportVersion so new required fields do not become false-positive parsing failures.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), criteria in frontmatter.
- [System design](../../specs/tasks/system-design/task-completion.md).
- [Plan](plan.md), confirmed source trace and existing reproduction tests.
- Nearby Go repository/handler tests supply fixture conventions.
- Follow `/tdd` in the primary session.

## Results

Done. The persisted setting now crosses the backend model, loader, service,
controller, MCP, DTO, step-event, frontend API, and workflow-draft boundaries.
Version 2 exports explicit booleans; version 1 imports normalize omitted values
at the boundary. Sync preserves explicit false values and invalid version 2
input is rejected before reconciliation.

Verification:

- `rtk go test -race ./config/workflows ./internal/workflow/models ./internal/workflow/controller ./internal/workflow/service ./internal/workflow/stepevents ./internal/task/dto ./internal/workflowsync -count=1` (426 passed)
- `rtk go test -race ./internal/mcp/handlers ./internal/mcp/server -run 'Test.*(Workflow|Step|Config)' -count=1` (172 passed)
- `rtk go test -race ./internal/task/repository/sqlite -run 'Test.*(Builtin|Workflow|Bootstrap)' -count=1` (65 passed)
- Targeted web contract and workflow-draft tests (31 passed), plus web typecheck.
