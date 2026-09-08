---
spec: docs/specs/office/requirements/agents.md
created: 2026-07-22
status: implemented
---

# Implementation Plan: Office Agent Project Management

## Overview

Make the default Office setup task executable by giving authorized Office agents a run-scoped project list/create surface, exposing it through `$KANDEV_CLI`, and allowing created follow-up tasks to carry `project_id`. Separately replace the generic Kanban tool inventory injected into Office turns with an exact Office capability context. This plan does not add workspace creation or expose Kanban completion tools to Office.

Corrective follow-up: an Office-only prompt and CLI must never be launched
without the scheduler-created signed runtime environment. Generic task/session
start paths fail closed for Office-owned tasks when that context is absent, and
the CLI/public docs explain that its API URL and key are automatic Office run
credentials rather than user configuration.

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

### Office launch-context invariant

Files:
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/task_operations_test.go`

Changes:
- Validate the minimum, task-bound Office runtime environment before any
  `LaunchPreparedSession` call selects `ModeOffice`.
- Fail before starting the agent when a generic/manual/workflow path reaches an
  Office-owned task without scheduler-provided `KANDEV_CLI`,
  `KANDEV_API_URL`, `KANDEV_API_KEY`, agent/workspace identity, and run ID.
- Reject generic Office resume and prepared-workspace relaunch paths; Office
  recovery must return to the scheduler so it can mint fresh runtime context.
- Preserve scheduler launches through `StartTaskWithEnv` and keep regular
  task-mode launches unchanged.

### CLI context diagnostics

Files:
- `apps/backend/cmd/agentctl/kandev_client.go`
- `apps/backend/cmd/agentctl/kandev_test.go`

Changes:
- Replace the bare missing-variable error with an actionable explanation that
  the values are injected automatically for Office runs.
- Direct regular task sessions to their Kandev MCP tools without suggesting
  users manufacture or persist an Office JWT.

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
- **What:** Office-owned tasks cannot start in `ModeOffice` without a complete
  scheduler-provided runtime environment, while scheduler and regular
  task-mode launches retain their existing behavior.
  **File:** `apps/backend/internal/orchestrator/task_operations_test.go`
  **How:** focused orchestrator launch tests with captured executor requests.
- **What:** invoking the CLI outside an Office run returns actionable,
  non-secret-bearing guidance.
  **File:** `apps/backend/cmd/agentctl/kandev_test.go`
  **How:** run the command with the two required variables unset and assert the
  structured error and zero HTTP dispatch.

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

Wave 4 (parallel):
- [x] [task-06-office-launch-context-guard](task-06-office-launch-context-guard.md)
- [x] [task-07-cli-context-diagnostic](task-07-cli-context-diagnostic.md)

Wave 5:
- [x] [task-08-public-runtime-credentials-docs](task-08-public-runtime-credentials-docs.md)

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

None. The approved corrective contract fails generic manual/workflow Office
launches before process start instead of minting a second, unaudited credential
class or silently granting the broader task-mode MCP surface.
