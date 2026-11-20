---
id: "01-scope-model-and-migration"
title: "Canonical scope and safe migration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.2
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.4
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.9
system_design:
  - ../../specs/integrations/system-design/github-repository-scope.md
---

# Task 01: Canonical scope and safe migration

## Outcome and dependencies

Added `selected` and preserved legacy allowsets through migration. The rebased
base already contained #5283. The design's Data, Shared Evaluation, and
Migration sections define the boundaries for this work order.

## Owned scope

- `apps/backend/internal/github/models.go`, `store.go`, and
  `workspace_settings_service.go`: constant, canonical read/write, validation,
  list normalization, and atomic partial updates.
- `service_routing.go` and its tests: shared membership rule for later consumers.
- New `store_repo_scope_migration.go` and its test: invoke after
  `backfillGitHubWorkspaceOwnership()` and before `seedLegacyWorkspaceConnections()`.
- New `workspace_settings_union_test.go`, affected `workspace_settings_test.go`,
  `controller_write_endpoints_test.go`, and `store_postgres_test.go`.
- `apps/backend/internal/backendapp/settings_integration_operations.go`:
  audit `updateGitHubSettings` forwarding and cover it in
  `settings_operations_test.go`. Production forwarding changes require a failing test.

Search consumers and rendered UI belong to later orders. Do not change credentials.

## Acceptance

1. `TestWorkspaceScopeUnionRoundTrip` fails first, then proves both grants
   survive save/read and an unrelated repository remains denied.
2. `TestWorkspaceScopeUnionMigration` seeds raw legacy modes, stale inactive
   lists, empty restricted rows, and already-selected rows. Reopen twice and
   prove exact allowset preservation. A transaction failure leaves original rows
   intact. `TestPostgresWorkspaceScopeUnion` proves migration/write behavior on PostgreSQL.
3. `TestWorkspaceScopeUnionPatchCompatibility` covers legacy inputs, canonical
   GET values, omissions, null, `[]`, unknown modes, and all-mode conflicts.
   Preserve unrelated fields. `TestGitHubSettingsOperationsScopeUnion` sends a
   mixed `selected` patch through catalog/agent operations and checks canonical readback.

## Verification

Run the first named regression before implementation, then run this block from
repo root. Each subshell restores the working directory.

```bash
(cd apps/backend && go test -race ./internal/github -run '^(TestWorkspaceScopeUnion(RoundTrip|PatchCompatibility|Migration)|TestRepositoryInWorkspaceScope|TestStore_GitHubWorkspaceSettingsRoundTrip|TestService_UpdateWorkspaceSettings_.*)$' -count=1)
(cd apps/backend && go test -race ./internal/backendapp -run '^TestGitHubSettingsOperationsScopeUnion$' -count=1)
(cd apps/backend && go test -race ./internal/github -run '^TestPostgresWorkspaceScopeUnion$' -count=1 -v)
```

Set `KANDEV_TEST_POSTGRES_DSN` to an isolated test database. Use
`testutil.OpenIsolatedPostgres`. Record a missing DSN as skipped, not passed.
No other work order repeats this environment gate.

## Risks and results

Seed stale records with raw SQL. Ordinary upserts remove that evidence.
All canonical writes and reads agree on the new mode. `TestWorkspaceScopeUnionRoundTrip`,
`TestWorkspaceScopeUnionPatchCompatibility`, and `TestWorkspaceScopeUnionMigration`
failed before implementation and pass with `-race`. Migration replay uses two
`NewStore` reopens; a SQLite trigger proves transaction rollback. The settings
HTTP endpoint and settings-operations adapter preserve a mixed `selected` patch.
`TestPostgresWorkspaceScopeUnion` passed against a disposable PostgreSQL 16
container. `TestPRWatchRetirement_MigrationReplay` also passes against its
pre-scope table shape; migration skips schemas without scope columns because
they have no saved scope values to convert. The full backend test suite and
build pass.
