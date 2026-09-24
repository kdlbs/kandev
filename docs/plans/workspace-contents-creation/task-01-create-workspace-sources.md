---
id: "01-create-workspace-sources"
title: "Create and launch with workspace sources"
status: complete
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-004
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-004.1
  - AC-TASKS-MIXED-REPOSITORIES-004.5
  - AC-TASKS-MIXED-REPOSITORIES-004.6
  - AC-TASKS-MIXED-REPOSITORIES-004.8
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 01: Create and launch with workspace sources

## Summary

Create APIs accept ordered folders and repositories and prepare all sources before first launch. Preserve old input and inherited-subtask behavior while distinguishing explicit empty scratch. This provides the runtime foundation consumed by Task 02.

## In scope

- Implement presence-aware `workspace_sources` through DTO, HTTP/WS, service preparation, response hydration, and rollback.
- Normalize legacy input; reject conflicting new/old fields and forbidden inherited-workspace edits.
- Persist through existing folder/source-batch storage with cross-kind positions; avoid create-then-live-attach.
- Wire folders into initial launch, folder-only managed roots, resume and cleanup. Reuse Local/Worktree source handling, branch policy and server provider inspection.
- Test folder validation, duplicate paths/names, write failure, no premature event/launch, first-turn file visibility, and preservation of live files on cleanup.

## Out of scope

No commits, publishing, new executors, or live-task attachment redesign. UI and last-used persistence are later work orders.

## Acceptance

- Valid mixed and folder-only requests round-trip in source order and expose all files before the agent starts.
- Invalid batches do not publish a partially sourced task; startup failures retain the existing recoverable task lifecycle and never delete user folders.
- Legacy HTTP/WS inputs and omitted subtask inheritance retain behavior; explicit empty input is scratch only where editable.

## Verification

Run from the repository root. The listed tests are the implementation evidence for
this work order. Install workspace dependencies once if missing.

```bash
(cd apps/backend && go test -tags fts5 ./internal/task/...)
(cd apps/backend && go test -tags fts5 ./internal/agent/runtime/lifecycle -run 'WorkspaceSources|WorkspaceFolders|FolderOnly|CreateWorkspaceSources')
(cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Workspace|Launch')
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -race -tags fts5 ./internal/persistence/storeconformance -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/task/dto/`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/task/handlers/task_ws_handlers.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/service_workspace_sources.go`
- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/repository/sqlite/workspace_folder.go`
- `apps/backend/internal/orchestrator/dynamic_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_execution.go`
- `apps/backend/internal/agent/runtime/lifecycle/workspace_sources_reconcile.go`
- `apps/backend/pkg/api/v1/`

## Dependencies

None.

## Risks

Repository-less early exits and failure cleanup are critical. Use the existing PostgreSQL test harness for changed transaction methods; set KANDEV_TEST_POSTGRES_DSN for those cases and record any unavailable infrastructure, never a false pass.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md), listed IDs.
- [Design](../../specs/tasks/system-design/mixed-repository-selection.md), creation extension.
- Existing mixed selection and attachment tests; plan Tests/E2E tables map scenarios.

## Results

Implemented presence-aware workspace source creation, ordered mixed source
persistence, folder runtime preparation, launch/resume reconciliation, rollback,
and legacy/inherited compatibility.

Verification passed on 2026-09-14:

- `go test -tags fts5 ./internal/task/...`
- Focused lifecycle workspace-source tests.
- Focused orchestrator workspace/launch tests and dynamic repository summary tests.
- `go run ./cmd/sqlguard ./internal`.
- `go test -race -tags fts5 ./internal/persistence/storeconformance -count=1`.
- `git diff --check`.
