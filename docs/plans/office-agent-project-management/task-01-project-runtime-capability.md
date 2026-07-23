---
id: "01-project-runtime-capability"
title: "Add the run-scoped project capability"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 01: Add the run-scoped project capability

## Acceptance

- A valid Office run can list projects in its token workspace; it cannot select another workspace.
- `create_project` succeeds only when the run capability derived from `can_create_projects` is present and emits a runtime audit event.
- CEO defaults grant `can_create_projects`; all other role defaults deny it unless explicitly overridden.

## Verification

```bash
cd apps/backend && rtk go test ./internal/office/runtime ./internal/office/shared ./internal/office/agents ./internal/office/service
```

## Files Likely Touched

- `apps/backend/internal/office/shared/permissions.go`
- `apps/backend/internal/office/shared/permissions_test.go`
- `apps/backend/internal/office/runtime/capabilities.go`
- `apps/backend/internal/office/runtime/actions.go`
- `apps/backend/internal/office/runtime/actions_test.go`
- `apps/backend/internal/office/runtime/handler.go`
- `apps/backend/internal/office/runtime/handler_test.go`
- `apps/backend/internal/office/routes.go`
- `apps/backend/internal/office/agents/service.go`
- `apps/backend/internal/office/service/agents.go`

## Inputs

- Spec: Permissions, Runtime capabilities, CLI and MCP scenarios.
- Pattern: existing `create_agent` runtime capability, action, handler, audit, and denial tests.
- Reuse `projects.ProjectService`; do not duplicate project validation or persistence.

## Output Contract

Report the capability contract, files changed, targeted tests, permission defaults, security risks, blockers, and task status. Use TDD and do not edit `plan.md`.

## Completion Evidence

- Added `list_projects` for every Office run and `create_project` derived from `can_create_projects`; CEO defaults enable project creation and every other role default disables it while allowing explicit overrides.
- Added authenticated `GET` and `POST /api/v1/office/runtime/projects` routes backed by the existing project service. Both operations force the validated run token workspace; request JSON cannot select another workspace.
- Added success and denial coverage for project creation audit events (`runtime.action` and `runtime.denied`) and verified denied calls never reach the project dependency.
- TDD cycles passed for permissions, capability serialization/derivation, workspace-scoped actions, and HTTP routes (11 focused regression tests).
- Assigned combined suite with `GOCACHE=/tmp/kandev-go-cache`: 126 tests passed. The remaining pre-existing `internal/office/service` test `TestRelayComment_AgentComment_Relayed` could not run because the restricted sandbox denied its `httptest` loopback socket (`listen tcp6 [::1]:0: operation not permitted`). Two elevated reruns were interrupted before execution.

## Corrective Evidence

- Added fail-closed workspace validation to both project actions before the project dependency can run. Empty or whitespace-only workspace scope now returns `ErrWorkspaceOutOfScope` for list and create operations.
- Added regression tests proving neither `ListProjects` nor `CreateProject` reaches the project manager without workspace scope.
- Added `can_create_projects` to Office permission metadata and a drift regression test that requires metadata keys to exactly match `shared.AllPermissionKeys()` in display order.
- `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/office/runtime -count=1` passed: 34 tests.
- `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/office/dashboard -count=1` passed: 131 tests.
- Added a distinct `create_task` run capability derived from `can_create_tasks` and a signed-token `POST /api/v1/office/runtime/tasks` action. The action takes workspace and caller identity only from `RunContext`.
- Project, parent task, and assignee references are validated against the token workspace before the Office task creator runs. Parent-plus-project requests must match the parent's persisted project, and missing/cross-workspace relations fail closed with `runtime.denied` and no creator call.
- The existing Office creator remains the persistence boundary: roots use `CreateOfficeTaskAsAgent`, children use `CreateOfficeSubtaskAsAgent`, and both carry `assignee_agent_profile_id` into the workflow runner projection.
- Focused runtime security suite passed: 16 tests covering capability derivation, root/child routing, workspace spoofing, project/parent/assignee denial, denied audit behavior, and unsupported input.
- Hardened project lead assignment: nonempty lead references are resolved through
  the Office agent service, must exactly match the resolved canonical profile ID,
  and must belong to the signed run workspace. Names, aliases, missing IDs, and
  cross-workspace IDs fail before the project manager is invoked; empty leads
  remain valid and surrounding whitespace on canonical IDs is normalized.
- `GOCACHE=/tmp/kandev-go-cache rtk go test ./internal/office/runtime -count=1`
  passed: 72 tests, including HTTP denial audit coverage for missing,
  cross-workspace, and same-workspace aliased lead and assignee references.
- Parent-bearing requests to the general runtime task-create endpoint now require
  `create_subtask` and explicit mutation scope for the parent before any parent
  workspace/project lookup or creator call. Root requests continue to require
  `create_task`.
- Focused action and handler regressions cover missing subtask capability,
  same-workspace but out-of-scope parents, zero relationship reads on denial,
  and successful scoped child creation. The full runtime package passed with 78
  tests.
