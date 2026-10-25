---
id: "05-persist-source-step-targets"
title: "Persist source-step targets"
status: complete
wave: 5
depends_on:
  - "04-prove-session-targeting"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.7
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.8
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.9
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.10
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-002.12
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 05: Persist source-step targets

## Summary

Extend the working initial-target contract with direct source-step references
and durable source bindings. Initial provenance and entry retry storage remain
unchanged.

## In scope

- Add `kind: step` to backend/frontend/MCP target unions. Validate same workflow,
  earlier position, direct profile override, no indirect target, and conditional
  rule exclusion. Include incoming-reference validation for edit/reorder/delete.
- Portable `step_position`, template and duplication ID remapping, sync equality,
  unresolved profile/source errors, and closed-kind rejection in earlier readers.
- Task-owned source-only `task_workflow_session_bindings` table. Add transactional
  conditional upsert/load/cleanup, nullable session pointer, and logical profile.
- Aligned full/stub schema and template/bootstrap mappings where needed. Test
  null legacy defaults without blindly changing ID-only helpers.
- Task/workflow/session deletion cleanup, stale-operation rejection, SQL guard,
  required-store descriptor and fixed task adapter, replay and upgrade tests.

## Out of scope

- Replacing initial metadata with table rows.
- Runtime source selection and UI source choices.

## Acceptance

- REST/MCP create/update/clear preserve source references and reject invalid
  sources without changing the workflow; initial contracts still pass.
- Export/import, template instantiation, duplication and sync remap references
  correctly; readers cannot silently ignore unsupported target kinds.
- Bindings survive reopen, reject stale writes, retain profile after session
  deletion, and clean up under task/workflow deletion on both databases.

## Verification

Use TDD; from the repository root:

```bash
rtk make -C apps/backend sqlguard
```

From `apps/backend`, use an isolated `KANDEV_TEST_POSTGRES_DSN`:

```bash
rtk go test ./internal/workflow/... ./internal/workflowsync/... ./config/workflows/... -count=1
rtk go test ./internal/mcp/server -run 'Test.*WorkflowStep' -count=1
rtk go test ./internal/mcp/handlers -run 'TestWorkflowStep.*Parity' -count=1
rtk go test -race ./internal/task/repository/sqlite -run 'TestWorkflowSessionBinding|TestPostgresWorkflowSessionBinding|TestWorkflowSessionTarget|TestPostgresWorkflowSessionTarget|Test.*DeleteWorkflow' -count=1
rtk go test -race ./internal/persistence/storeconformance -count=1
rtk go test ./internal/backendapp -run '^TestPostgresBootInitializesRepositories$' -count=1
```

Add explicit step-target payloads to schema/forwarding/event-parity tests.
Report PostgreSQL skips as missing evidence.

## Files likely touched

- Task 01's workflow model/controller/repository, MCP server/handler, and loader files
- `apps/backend/internal/task/repository/sqlite/workflow_session_bindings.go` and tests (new)
- `apps/backend/internal/task/repository/sqlite/{base_schema.go,base_migrations.go,workflow.go}`
- Task models/repository interfaces and session/task deletion paths
- `apps/backend/internal/persistence/{requiredstores,storeconformance}/`
- `apps/backend/AGENTS.md` for binding ownership
- `apps/web/lib/types/http.ts` and related workflow contracts

## Dependencies

Task 04 completes the initial slice. Task 01's metadata and version contract are
reused; do not recreate them.

## Risks

A duplicate/import can retain foreign source IDs. A reorder or deletion can
invalidate dependents. Profile removal and sync must validate the whole result.
Table cleanup must cover workspace reset as well as direct deletion.

## Inputs

- Design: Explicit recipient contract; Source-step bindings.
- Existing `RemapStepID`, `pull_from_step_position`, MCP parity tests.
- `workflow.go` is relevant for deletion; `builtin_workflow_step_rows.go`
  is an ID lookup, not a target-value mapping.

## Parallelism

`sequential`

## Results

Implemented the source-step target variant, binding table, migrations, required
store inventory, cleanup, portable step-position remapping, and stale-operation
guards. Binding repository tests and task repository race tests passed; full
store conformance passed 290 tests.
