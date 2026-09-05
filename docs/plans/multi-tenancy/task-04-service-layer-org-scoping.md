---
id: "04-service-layer-org-scoping"
title: "Service-layer org scoping"
status: todo
wave: 2
depends_on: ["02-org-entity-and-identity"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 04: Service-Layer Org Scoping

## Acceptance

- `workspaces.org_id` is populated and every `authorize*` helper in
  `internal/task/service/service_access.go` checks the org before the owner.
  A foreign-org workspace, task, session, environment, repository, or
  attachment returns the existing `*NotFound` sentinel.
- Descendant tables listed in the tenancy registry as needing a denormalized
  `org_id` receive one, with the registry recording the production query that
  justified it.
- `secrets.org_id` is populated; secret reads are org-scoped ahead of the
  existing user/workspace scope.
- A repository-level test runs every list and get method as a foreign org and
  asserts an empty result or a sentinel — the second net beyond the table
  registry.
- Visibility and scopes inside an org are unchanged: this task adds no new
  visibility rule, it bounds the existing ones. Every team-access reach-matrix
  and scope test still passes unmodified.
- `org` visibility resolves to the workspace's own org: a member of another org
  gets 404 on an org-visible workspace.
- Org scopes are held in one org only: `org.members.manage` and
  `org.config.manage` grant nothing outside the holder's org.
- Workspace membership is confined to one org: adding a member from another org
  is refused with 404, and the tenancy migration drops any membership row whose
  member and workspace would land in different orgs, logging each one without
  aborting the migration.

## Verification

- `go test ./internal/task/... ./internal/secrets/...` from `apps/backend`
- `go test ./internal/task/repository/... -run TestForeignOrgIsolation`
- `KANDEV_TEST_POSTGRES_DSN=... go test ./internal/task/repository/sqlite/...`

## Files Likely Touched

- `apps/backend/internal/task/service/service_access.go`
- `apps/backend/internal/task/repository/sqlite/{base_schema,base_migrations,workspace,attachment}.go`
- `apps/backend/internal/task/models/models.go`, `internal/task/dto/`
- `apps/backend/internal/secrets/sqlite_store.go`
- `apps/backend/internal/tenancy/registry.go`

## Inputs

- Spec: Data model (denormalization rule), Permissions, Scenarios (foreign-org
  404, operator has no org visibility).
- Patterns: the existing `authorize*` helpers and their `*NotFound` no-leak
  convention; the "new user-facing service entry points must apply scoping"
  rule in `apps/backend/AGENTS.md`.

## Output Contract

Report which descendant tables gained an `org_id` and the query that justified
each, the foreign-org isolation test's coverage count against the method count,
RED/GREEN commands, and set this task plus its plan checkbox to done.
