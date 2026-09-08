---
id: "01-branch-policy-persistence-api"
title: "Add branch-policy persistence and APIs"
status: complete
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-BRANCH-POLICIES-001
  - REQ-WORKSPACES-BRANCH-POLICIES-002
  - REQ-WORKSPACES-BRANCH-POLICIES-005
acceptance_criteria:
  - AC-WORKSPACES-BRANCH-POLICIES-001.4
  - AC-WORKSPACES-BRANCH-POLICIES-001.5
  - AC-WORKSPACES-BRANCH-POLICIES-002.1
  - AC-WORKSPACES-BRANCH-POLICIES-002.2
  - AC-WORKSPACES-BRANCH-POLICIES-002.3
  - AC-WORKSPACES-BRANCH-POLICIES-002.4
  - AC-WORKSPACES-BRANCH-POLICIES-005.4
system_design: "../../specs/workspaces/system-design/branch-policies.md"
---

# Task 01: Add branch-policy persistence and APIs

## Objective

Introduce the authoritative repository branch-policy model, replayable storage,
service authorization, REST/WebSocket operations, boot hydration, and atomic
Gitflow starter.

## TDD contract

Write failing repository and service tests first. They must prove normalized
name uniqueness, CRUD, repository cascade, cross-workspace denial, migration
replay, and all-or-nothing Gitflow seeding before production code is added.

## Implementation scope

- Add `RepositoryBranchPolicy` DTO/model and repository interface methods.
- Add `repository_branch_policies` to the SQLite base schema and replayable
  SQLite/Postgres migrations with the case-insensitive unique index.
- Implement repository storage and service validation with the existing safe
  Git-ref and worktree-template renderer.
- Add list/create/update/delete and atomic Gitflow-starter HTTP and WebSocket
  handlers through the shared service.
- Include policies in active-workspace boot state and publish semantic policy
  mutation events.
- Log mutations and starter outcomes without logging descriptions.

## Implementation acceptance

- Policy CRUD and the Gitflow starter use one authorized service contract across
  REST and WebSocket transports.
- Schema creation and replay produce the same constraints in SQLite and
  Postgres.
- Concurrent or invalid starter requests leave no partial policy set.

## Exclusions

- Task policy snapshots and runtime consumption.
- Repository-settings UI or task-create UI.
- Policy ordering and repository-wide default policies.

## Files likely touched

- `apps/backend/internal/task/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/`
- `apps/backend/internal/task/repository/postgres/`
- `apps/backend/internal/task/service/`
- `apps/backend/internal/task/handlers/`
- `apps/backend/internal/http/`
- `apps/backend/internal/ws/`
- `apps/backend/internal/boot/`

## Verification

- `cd apps/backend && go test ./internal/task/repository/sqlite ./internal/task/repository/postgres -run 'Test.*BranchPolic(y|ies)'`
- `cd apps/backend && go test ./internal/task/service ./internal/task/handlers -run 'Test.*BranchPolic(y|ies)|Test.*Gitflow'`

Run Postgres integration coverage under the package's existing environment gate.
The focused tests must show a RED result before the implementation.

## Dependencies and parallelism

No dependencies. Sequential: this task establishes contracts consumed by every
later work package.

## Output contract

Report the RED tests, schema and API contracts, authorization behavior, exact
verification results, changed files, and remaining risks. Mark this task and
the plan checkbox complete together.

## Results

Implemented the repository-owned policy model, SQLite schema and migrations,
case-insensitive uniqueness, authorized CRUD, atomic Gitflow seeding, REST and
WebSocket handlers, boot hydration, and semantic mutation events. Gitflow
creation now validates that both requested branches exist before inserting any
policy.

Verification:

- `go test ./internal/task/service -run 'TestRepositoryBranchPolicyService' -count=1` passed.
- `go test ./internal/task/... ./internal/backendapp/... ./internal/orchestrator/...` passed: 6,680 tests in 23 packages.
- `make -C apps/backend build` passed.
