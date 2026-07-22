---
id: "12-registration-persistence"
title: "Registration persistence"
status: done
wave: 1
depends_on: ["11-reconcile-main"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 12: Registration Persistence

## Acceptance

1. The unpublished singleton tables are replaced by the plural registration, workspace-bound flow,
   registration FK, and composite delivery schema from the spec without changing released
   `legacy_shared` migration behavior.
2. Managed/imported registrations use independent encrypted generation bundles; the unpublished App
   environment configuration, bindings, source resolver, and compatibility behavior are removed.
3. Delete/reference constraints, compensation, restart reload metadata, workspace copy/delete, and
   personal generation invalidation have focused real-database tests.

## Verification

```bash
rtk go test ./internal/github -run 'Test.*(Registration|WorkspaceConnection|PersonalConnection|Legacy)'
test -z "$(rg -l 'KANDEV_GITHUB_APP' internal/common/config internal/github || true)"
```

Run from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/store.go`
- `apps/backend/internal/github/deployment_app_store.go` (rename to registration-oriented name)
- `apps/backend/internal/github/deployment_app_store_test.go` (rename with production file)
- `apps/backend/internal/github/store_connections.go`
- `apps/backend/internal/github/store_connections_test.go`
- `apps/backend/internal/github/personal_connection_repository.go`
- `apps/backend/internal/github/personal_connection_repository_test.go`
- `apps/backend/internal/github/deployment_app_config.go` (remove/replace with catalog-only resolver)
- `apps/backend/internal/github/deployment_app_config_test.go` (remove/replace)
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/config_test.go`

## Dependencies

Task 11.

## Inputs

- Spec: **Data Model**, **Persistence Guarantees**, and removal of unpublished App configuration in
  **What**.
- ADR: `docs/decisions/2026-07-21-workspace-selectable-github-app-registrations.md`.
- Preserve existing encrypted secret-store compensation patterns and released legacy seeding.

## Output Contract

Report schema/repository contract, migration assumptions, tests run, files touched, blockers/risks,
and update this task plus `plan.md` to done.
