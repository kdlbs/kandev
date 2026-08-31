---
id: "01-add-portable-workflow-policy"
title: "Move policy to workflow steps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.10
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 01: Move Policy to Workflow Steps

## Summary

Make the destination step the only owner of the normalized profile-session
policy. Remove the unshipped workflow-level contract and carry the field through
step CRUD, templates, portable export/import, and workflow sync.

## In scope

- Add the enum field to `WorkflowStep` and `StepDefinition`.
- Add a replayable SQLite/PostgreSQL `workflow_steps` column migration.
- Include the field in step create, read, list, update, request, and response
  mappings.
- Move the portable field from `WorkflowPortable` to `StepPortable`.
- Preserve the field through templates, duplication, import, and sync equality
  and update paths.
- Remove workflow-level persistence, DTO, metadata, portable, boot, and frontend
  type mappings introduced by the unshipped implementation.
- Add focused normalization and mixed-step round-trip tests.

## Out of scope

- Session switching behavior.
- Combined selector behavior.

## Acceptance

- Empty and unknown step policy values normalize to `complete`.
- Two steps in one workflow can persist and round-trip different canonical
  values.
- Export, import, templates, and sync keep the policy attached to the same step.
- No workflow-level policy field or precedence path remains.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/workflow/repository ./internal/workflow/models ./internal/workflow/service ./internal/workflow/handlers ./internal/workflowsync -run 'ProfileSessionPolicy|WorkflowStep' -count=1
```

## Files likely touched

- `apps/backend/internal/workflow/models/models.go`
- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/repository/sqlite.go`
- `apps/backend/internal/workflow/controller/controller.go`
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/service/sync_apply.go`
- `apps/backend/internal/workflowsync/`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/workflow.go`

## Dependencies

None.

## Risks

- The current branch already contains an unshipped workflow-level field. Leaving
  it in one serialization or sync path creates two authorities.
- Template, import, and sync paths use separate step mappings and equality
  checks. Each must preserve the enum.

## Parallelism

`sequential`

## Inputs

- Requirement AC 001.1, 001.6, and 001.10.
- System-design Data and contracts and Persistence sections.
- Existing step `agent_profile_id` mappings.

## Results

Moved the normalized policy to `WorkflowStep` and `StepDefinition`, removed the
unshipped workflow-level contract, and carried the field through step CRUD,
templates, duplication, portable export/import, and workflow sync. Added
normalization and mixed-step round-trip coverage. The affected backend package
suite passed with 7,209 tests across 27 packages.
