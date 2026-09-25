---
id: "01-project-identity"
title: "Project identity and flag"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PROJECTS-AGENT-PROJECTS-001
  - REQ-PROJECTS-AGENT-PROJECTS-004
  - REQ-PROJECTS-AGENT-PROJECTS-005
acceptance_criteria:
  - AC-PROJECTS-AGENT-PROJECTS-001.1
  - AC-PROJECTS-AGENT-PROJECTS-001.2
  - AC-PROJECTS-AGENT-PROJECTS-001.3
  - AC-PROJECTS-AGENT-PROJECTS-001.4
  - AC-PROJECTS-AGENT-PROJECTS-001.5
  - AC-PROJECTS-AGENT-PROJECTS-004.2
  - AC-PROJECTS-AGENT-PROJECTS-004.3
  - AC-PROJECTS-AGENT-PROJECTS-004.4
  - AC-PROJECTS-AGENT-PROJECTS-005.3
  - AC-PROJECTS-AGENT-PROJECTS-005.4
system_design:
  - ../../specs/projects/system-design/agent-projects.md
---

# Task 01: Project identity and flag

## Summary

Create the Agent Project domain and gated, workspace-authorized project
create/read/edit lifecycle. Give project tasks a separate identity and a
narrow workflow-free creation path, preserving Office project semantics.

## In scope

- Typed flag registration, all-off profiles, and frontend default contract.
- Project table, task membership migration, service and HTTP DTOs, creation
  idempotency, editing revision, stored default executor, dependency
  validation, and coordinator task.
- Task launch/board/archive admission, generic-child rejection, project-level
  archive/restore/delete service and HTTP routes, and disabled-path rejection.

## Out of scope

- Context materialization, project MCP tools, rendered UI.

## Acceptance

- Creating one project creates exactly one durable workflow-free coordinator
  task with `agent_project_id` and no Office `project_id`; retries do not
  duplicate it.
- Disabled direct API/task launch requests have no side effects, while
  ordinary and Office tasks retain their current path.
- Stale edits and missing repository/profile/executor dependencies return
  typed errors; existing sessions and workers keep their pinned profiles.
- Project archive/restore/delete is atomic or resumable, confirms all worker
  effects, and cannot leave an orphaned workflow-free child.

## Verification

```bash
(cd apps/backend && go test ./internal/projects/... ./internal/task/service/... ./internal/runtimeflags ./internal/common/config ./internal/profiles)
(cd apps && pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts)
```

## Files likely touched

- `profiles.yaml`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/runtimeflags/config.go`
- `apps/backend/internal/projects/`
- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/web/lib/state/slices/features/types.ts`
- `apps/backend/AGENTS.md`
- `apps/backend/internal/projects/AGENTS.md`
- `AGENTS.md`

## Dependencies

None.

## Risks

- Existing persistent-task paths assume a workflow; test create, launch,
  archive, delete, and board reads before expanding project routes.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/projects/requirements/agent-projects.md)
- [System design](../../specs/projects/system-design/agent-projects.md)
- [Identity decision](../../decisions/2026-09-24-agent-project-identity-and-context.md)

## Results

Passed:

- `go test ./internal/task/service ./internal/projects/... -count=1`
- `go test -race ./internal/task/service -run '^TestUnarchiveAgentProjectTreeRetriesWhileArchiveCleanupRuns$' -count=1`
- `go test ./internal/runtimeflags ./internal/profiles`
- `env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE go test ./internal/common/config -count=1`
- `make lint` and the managed E2E backend build.

The restore regression proves project-level restore waits through a running
archive cleanup; ordinary task unarchive still rejects that race.
