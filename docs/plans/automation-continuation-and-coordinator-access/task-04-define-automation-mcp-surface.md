---
id: "04-define-automation-mcp-surface"
title: "Define automation MCP surface"
status: completed
wave: 2
depends_on: ["01-persist-continuation-policy"]
plan: "plan.md"
spec: "../../specs/office/requirements/automation-continuity.md"
---

# Task 04: Define Automation MCP Surface

## Acceptance

- `SurfaceAutomation` exposes exactly the fixed catalog while all existing base surfaces retain
  their current tools; monolithic registration is decomposed so exclusions are structural.
- Lifecycle dispatch resolves one trusted automation principal, and a shared authorizer applies its
  workspace/self boundary before any included read, create, or mutation.
- Self, foreign, malformed, and forged targets fail closed; cross-task spawning uses the target
  task's normal profile and permission audit derives `automation_mcp` identity.

## TDD scenarios

1. RED: Snapshot every base-surface catalog and prove automation excludes deletion, configuration,
   task-local authoring, diagnostics, provider, and plugin groups.
2. RED: Prove principal identity comes from execution task/session and is absent from tool schemas.
3. RED: Exercise same-workspace, foreign-workspace, and self-target matrices for inventory,
   creation/coordination, blocker discovery/resolution, messaging, stopping, and spawning.
4. RED: Prove a spawned cross-task session resolves the target task profile while same-automation-
   task spawning is denied.
5. RED: Prove audit source/actor cannot be forged.
6. GREEN: Split registrations, add typed surface/principal, wire lifecycle scope, and centralize
   authorization.
7. REFACTOR: Remove handler-local scope derivation and retain external PAT authorization unchanged.

## Verification

- `cd apps/backend && go test -tags fts5 ./internal/mcp/profile ./internal/mcp/scope ./internal/mcp/server ./internal/mcp/handlers ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor`
- `git diff --check`

## Files likely touched

- `apps/backend/internal/mcp/profile/profile.go`
- `apps/backend/internal/mcp/profile/profile_test.go`
- `apps/backend/internal/mcp/scope/scope.go`
- `apps/backend/internal/mcp/scope/scope_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/question_handlers.go`
- `apps/backend/internal/mcp/server/agent_permissions_test.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/question_handlers.go`
- `apps/backend/internal/mcp/handlers/agent_permissions.go`
- `apps/backend/internal/mcp/handlers/spawn_session.go`
- `apps/backend/internal/mcp/handlers/spawn_session_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_identity.go`
- `apps/backend/internal/agent/runtime/lifecycle/mcp_identity_test.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/orchestrator/executor/executor_mcp_mode_test.go`
- `apps/backend/internal/backendapp/helpers.go`

## Dependencies

Task 01 supplies the durable automation-run origin/binding contract used to resolve the principal.

## Inputs

- Automation MCP surface and Permissions sections in the automation settings spec.
- Surface-placement rules in the external question and permission specs.
- Existing lifecycle `taskScopedMCPHandler`, `internal/mcp/scope`, and profile registry.

## Parallelism

Parallel-safe with Tasks 02 and 06 after Task 01. Ownership is limited to MCP, lifecycle identity,
executor profile resolution, and backend wiring; Task 02 must not edit those files.

## Output contract

Report exact catalogs, split groups, principal contents, self/foreign matrices, spawned profiles,
audit evidence, files changed, and exact tests.

## Risks

- Falling back to owner identity or missing one action in the central policy can cross workspace or
  self-approval boundaries.

## Results

Implemented the fixed `SurfaceAutomation` catalog, execution-time allowlist, trusted principal derivation, workspace/self guards, automation permission attribution, and target-profile spawning. Added workspace-list and principal regression coverage. MCP verification passed with 870 tests.
