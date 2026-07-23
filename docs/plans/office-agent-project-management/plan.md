---
spec: docs/specs/office/agents.md
created: 2026-07-22
status: implemented
---

# Implementation Plan: Office Agent Project Management

## Overview

Make the default Office setup task executable by giving authorized Office agents a run-scoped project list/create surface, exposing it through `$KANDEV_CLI`, and allowing created follow-up tasks to carry `project_id`. Separately replace the generic Kanban tool inventory injected into Office turns with an exact Office capability context. This plan does not add workspace creation or expose Kanban completion tools to Office.

## Backend

### Run-scoped project capability

Files:
- `apps/backend/internal/office/shared/permissions.go`
- `apps/backend/internal/office/runtime/capabilities.go`
- `apps/backend/internal/office/runtime/actions.go`
- `apps/backend/internal/office/runtime/handler.go`
- `apps/backend/internal/office/routes.go`
- `apps/backend/internal/office/agents/service.go`
- `apps/backend/internal/office/service/agents.go`

Changes:
- Add `can_create_projects`, defaulting to true only for the CEO role.
- Add stable `list_projects` and `create_project` runtime capability keys; listing is available to Office roles, while creation derives from `can_create_projects`.
- Add `GET /api/v1/office/runtime/projects` and `POST /api/v1/office/runtime/projects` through `runtime.Handler`.
- Add `create_task` plus `POST /api/v1/office/runtime/tasks`, derived from `can_create_tasks`; force caller/workspace from the run token and validate project, parent, and assignee ownership before persistence.
- Reuse `projects.ProjectService` through a narrow runtime dependency interface. Force `workspace_id` from the validated run context; never accept it from the request body.
- Emit the existing `runtime.action` / `runtime.denied` audit events.

### Agent CLI

Files:
- `apps/backend/cmd/agentctl/kandev.go`
- `apps/backend/cmd/agentctl/kandev_projects.go`
- `apps/backend/cmd/agentctl/kandev_task.go`
- `apps/backend/cmd/agentctl/kandev_test.go`

Changes:
- Add `projects list` and `projects create` command dispatch.
- `projects create` requires `--name`; supports repeatable `--repository` plus optional description, lead profile, color, budget, and executor JSON; sends the run token to the runtime endpoint.
- Add `--description` and `--project` to `task create`, serialize them through the authenticated Office runtime endpoint, and preserve parent/assignee assignment. Unsupported priority, blocker, and workspace-policy fields fail explicitly.
- Keep workspace creation absent: every mutation is scoped by the signed run token; `KANDEV_WORKSPACE_ID` is context only and is never trusted for authorization.

### Office capability context and skill

Files:
- `apps/backend/config/prompts/office-context.md`
- `apps/backend/internal/sysprompt/sysprompt.go`
- `apps/backend/internal/sysprompt/sysprompt_test.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
- `apps/backend/internal/office/configloader/skills/kandev-projects/SKILL.md`
- `apps/backend/internal/office/skills/system_sync_test.go`

Changes:
- Add an Office-specific first-turn context containing exactly the nine `ModeOffice` tools and directing Office mutations to `$KANDEV_CLI`.
- Select that context before launching an Office MCP session; keep the existing task context for `ModeTask`.
- Add a `kandev-projects` system skill, default for CEOs, documenting list/create and task-project assignment.
- Cross-check advertised Office tool names against the registered `ModeOffice` inventory. Assert that `step_complete_kandev` is absent.

## Frontend

No UI behavior changes. The existing onboarding wizard, project pages, and permission metadata renderer consume the backend contracts. The new permission must appear automatically through the existing permission metadata response.

## Tests

- **What:** CEO runtime context can list/create projects; a worker is denied creation; request workspace input cannot escape the token workspace.
  **File:** `apps/backend/internal/office/runtime/actions_test.go`, `apps/backend/internal/office/runtime/handler_test.go`
  **How:** table-driven action tests and HTTP handler tests with signed runtime JWTs.
- **What:** role defaults and overrides include `can_create_projects` without changing existing permissions.
  **File:** `apps/backend/internal/office/shared/permissions_test.go`
  **How:** table-driven permission resolution tests.
- **What:** CLI commands use the runtime endpoints, serialize repeatable repositories, reject missing names, and include `project_id` and the established Office assignee identifier on task creation without trusting caller-selected workspace scope.
  **File:** `apps/backend/cmd/agentctl/kandev_test.go`
  **How:** in-memory request capture plus signed runtime/SQLite persistence tests for relation ownership and runner assignment.
- **What:** Office prompt inventory equals `ModeOffice` registration and excludes all Kanban/config tools, especially `step_complete_kandev`.
  **File:** `apps/backend/internal/mcp/server/sysprompt_sync_test.go`
  **How:** extract `_kandev` references and compare exact sets.
- **What:** the default CEO receives the project skill after system-skill synchronization.
  **File:** `apps/backend/internal/office/skills/system_sync_test.go`
  **How:** sync embedded skills into SQLite and assert role defaults/content.
- **What:** onboarding's default setup brief is executable through the advertised Office CLI/tool surface.
  **File:** `apps/backend/internal/office/onboarding/service_test.go`
  **How:** contract test against required setup mutations and capability catalog.

## E2E Tests

No new browser interaction is introduced. The runtime handler integration and CLI request-capture tests exercise the user-visible agent path without adding a brittle model-driven E2E.

## Implementation Waves

Wave 1 (parallel):
- [x] [task-01-project-runtime-capability](task-01-project-runtime-capability.md)
- [x] [task-02-project-cli](task-02-project-cli.md)

Wave 2:
- [x] [task-03-office-capability-context](task-03-office-capability-context.md)

Wave 3:
- [x] [task-04-office-integration-verification](task-04-office-integration-verification.md)
- [x] [task-05-public-docs](task-05-public-docs.md)

## Verification Commands

```bash
rtk make -C apps/backend fmt
cd apps/backend && rtk go test ./internal/office/runtime ./internal/office/shared ./internal/office/skills ./internal/office/onboarding
cd apps/backend && rtk go test ./cmd/agentctl ./internal/mcp/server ./internal/sysprompt ./internal/orchestrator
rtk make -C apps/backend lint
rtk make -C apps/backend test
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Open Questions

None. Project creation is CEO-only by default but remains individually grantable through `can_create_projects`. Additional workspace creation remains human-owned and out of scope.
