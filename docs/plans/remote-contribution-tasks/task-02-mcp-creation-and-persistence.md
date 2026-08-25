---
id: "02-mcp-creation-and-persistence"
title: "MCP creation and persistence"
status: completed
wave: 2
depends_on: ["01-provider-contribution-resolution"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 02: MCP Creation and Persistence

## Acceptance

- `create_task_kandev.repositories[].repository_url` accepts supported PR/MR URLs while its public input
  property set remains byte-for-byte equivalent by name to the pre-feature schema.
- The MCP coordinator resolves the provider change before task persistence, creates one target
  repository attachment with the binding, and idempotently associates the existing PR/MR before launch.
- Association failure compensates the newly created task; `start_agent: false`, inheritance, and ordinary
  repository URLs preserve their current behavior.

## Verification

```bash
cd apps/backend
rtk go test ./internal/task/service ./internal/task/repository/sqlite ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp -run 'Test.*(CreateTask|RemoteContribution|ContributionURL|ExistingPR|ExistingMR|ToolSchema)'
```

## Files likely touched

- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/repository/sqlite/task_repository.go`
- focused task service/repository tests
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers_test.go`
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/backendapp/helpers.go`
- focused backend wiring tests

## Dependencies

Task 01's binding and provider resolvers.

## Parallelism

Sequential. It persists Task 01's contract and establishes the durable input for runtime tasks.

## Inputs

- Spec: **What**, **API surface**, task creation and catalog scenarios.
- ADR: reuse `repository_url`, target attachment ownership, association-before-launch.
- Existing patterns: MCP create-task dependency setters, `ResolveRepositoryRef`,
  `AssociateExistingPRByURLForWorkspace`, and `AssociateExistingMRByURL`.

## Risks

- Resolve the workspace before provider calls and preserve cross-workspace not-found behavior.
- Compensation must remove only the task created by this call and must run before any asynchronous launch.
- Keep `pr_number` compatibility metadata only where required; do not create a second source attachment.

## Output contract

Report behavior, schema property comparison, persistence/association outcome, files changed, exact test
results, blockers/risks, divergence, and task/plan status updates.

## Completion

Implemented server-side contribution resolution behind the existing `repository_url` property, target
repository attachment persistence, association-before-launch, and task compensation when association
fails. The MCP catalog regression test confirms that the create-task property names remain unchanged;
ordinary repository URLs and `start_agent: false` continue through the existing path.

The affected task, MCP, and backend wiring packages passed in the 17-package backend suite: 5,603 tests
passed. Association and provider calls remain injected/coordinated server-side, with hermetic tests and
no provider-authored title/body/diff content entering the trusted binding or task context.
