---
id: "01-add-portable-workflow-policy"
title: "Add portable workflow policy"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Add Portable Workflow Policy

## Summary

Define and persist the normalized workflow profile-session policy. Carry it
through workflow CRUD, cached workflow metadata, portable export/import, and
sync without changing existing workflow behavior.

## In scope

- Domain enum and normalization.
- Replayable SQLite/PostgreSQL workflow column migration.
- Repository create/read/list/update mappings.
- Controller/service/adapter and portable workflow mappings.
- Focused persistence, metadata, import/export, and sync tests.

## Out of scope

- Session switching behavior.
- Workflow settings UI.

## Acceptance

- Empty and unknown policy values read and write as `complete`.
- All workflow API and portable paths preserve each canonical value.
- Existing workflow fixtures and imports remain compatible.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/repository/sqlite ./internal/workflow/models ./internal/workflow/service ./internal/workflow/handlers ./internal/workflowsync -run 'ProfileSessionPolicy|WorkflowMeta' -count=1
```

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/workflow.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/backendapp/orchestrator.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/workflowsync/`

## Dependencies

None.

## Risks

- The task repository owns workflow data, and the workflow package owns
  portable conversion. Both paths must normalize identically.

## Parallelism

`sequential`

## Inputs

- Requirement AC 001.1 and 001.6.
- System-design Data and contracts and Persistence sections.
- Existing `agent_profile_id` and `prompt` workflow mappings.

## Results

- Added the normalized workflow profile-session policy to task persistence,
  workflow metadata, API/boot projections, portable export/import, and sync.
- Verification passed: `go test -tags fts5 ./internal/task/repository/sqlite ./internal/workflow/models ./internal/workflow/service ./internal/workflow/handlers ./internal/workflowsync -run 'ProfileSessionPolicy|WorkflowMeta' -count=1`.
