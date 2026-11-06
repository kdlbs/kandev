---
id: "01-configuration-graph"
title: "Copy the workspace configuration graph"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-CLONE-001
  - REQ-WORKSPACES-CLONE-002
acceptance_criteria:
  - AC-WORKSPACES-CLONE-001.1
  - AC-WORKSPACES-CLONE-001.2
  - AC-WORKSPACES-CLONE-001.3
  - AC-WORKSPACES-CLONE-001.5
  - AC-WORKSPACES-CLONE-001.6
  - AC-WORKSPACES-CLONE-002.2
  - AC-WORKSPACES-CLONE-002.4
system_design:
  - ../../specs/workspaces/system-design/clone-workspace.md
---

# Task 01: Copy the workspace configuration graph

## Summary

Implement and test the configuration mapping and domain-owned transaction
participants. They produce an independent graph without publishing it or
opening a second writer transaction. This is an internal verification boundary
used by the complete creation coordinator in Task 02.

## In scope

- Add clone graph types and mapping under task service/models where appropriate;
  allocate IDs globally before rewriting workflow/set/policy/script references.
- Copy general settings, live repository definitions, inline/named scripts,
  policies, ordered sets, visible workflows, and every live step field.
- Preserve available install-wide references and local source paths; clear
  managed provider checkout paths, sync ownership, timestamps and runtime state.
- Validate and reject unknown/excluded step references and explicit task/session
  references; reuse the typed workflow remappers and preserve the `this` sentinel.
- Add transaction-scoped task/workflow storage methods with existing serializers
  and SQL binding conventions. Add Kanban fallback only for an empty eligible set.
- Use `/tdd`; read scoped guidance and the design before writing permanent tests.

## Out of scope

GitHub/secret copying, public route, UI, events, commit/push, and broad refactoring
of existing storage interfaces or portable YAML import/export.

## Acceptance

1. `TestWorkspaceCloneGraph` seeds nondefault values for every copied field and
   proves fresh row IDs, remapped references, ordered sets/scripts and deep-copy
   independence after edits. Deleted/hidden rows and history are absent.
2. Invalid included references fail before a usable graph is written; valid
   empty-workflow sources receive exactly one template-derived Kanban workflow.
3. A caller-owned transaction rolls every graph participant back on failure or
   cancellation, with no events or source changes. Tests exercise the real
   SQLite factory and PostgreSQL when configured, not mocked SQL alone.

## Verification

Run from the repository root; PostgreSQL cases must report explicit skips when
`KANDEV_TEST_POSTGRES_DSN` is unavailable.

```bash
(cd apps/backend && go test ./internal/task/service ./internal/task/repository/sqlite ./internal/workflow/models ./internal/workflow/repository -run 'TestWorkspaceClone|TestService_CreateWorkspace|TestCreateWorkspaceWithKanban|TestRemap' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/models/workspace_clone.go` (new).
- `apps/backend/internal/task/service/workspace_clone_graph.go` and `_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/workspace_clone.go` and `_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/workspace_clone_postgres_test.go` (new).
- `apps/backend/internal/task/repository/sqlite/workspace_bootstrap.go`.
- `apps/backend/internal/task/repository/sqlite/repository_entity.go`.
- `apps/backend/internal/workflow/repository/workspace_clone.go` and `_test.go` (new).
- `apps/backend/internal/workflow/models/models.go` and focused remapping tests
  only if a typed reference currently lacks a safe copy path.

## Dependencies

None. Introduce internal clone participants without exposing incomplete creation.

## Risks

Unknown references survive `RemapStepID`; JSON action maps can alias source
memory; portable export omits live fields. PostgreSQL must use final-handle
rebinding, while all writer reads use the supplied transaction.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/clone-workspace.md), 001 and 002.4.
- [Design](../../specs/workspaces/system-design/clone-workspace.md), configuration mapping and persistence.
- `workspace_bootstrap.go`, repository/set/script tests, and typed workflow remapping tests.

## Results

Graph and typed-remapping tests passed, including repository settings/scripts/policies/sets, hidden-workflow exclusion, fresh IDs, owner-only membership, secret-binding exclusion, provider-path reset and rollback. Task-defined backend command passed across task service/repository and workflow model/repository packages.
