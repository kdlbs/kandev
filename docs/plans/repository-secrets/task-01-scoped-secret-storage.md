---
id: "01-scoped-secret-storage"
title: "Add scoped secret storage"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 01: Add Scoped Secret Storage

## Acceptance

- Existing secret rows migrate to Global without changing encrypted data or ownership.
- Global and Workspace CRUD/list/reveal enforce the spec's user and workspace boundaries.
- Scope is immutable, omitted scope defaults Global, and workspace deletion removes Workspace
  secrets.
- Agent and executor profile save/runtime paths accept Global secret references only.
- Internal integration credentials remain hidden and functional.
- SQLite and PostgreSQL migration replay succeeds twice.

## Files likely touched

- `apps/backend/internal/secrets/models.go`
- `apps/backend/internal/secrets/store.go`
- `apps/backend/internal/secrets/sqlite_store.go`
- `apps/backend/internal/secrets/service.go`
- `apps/backend/internal/secrets/handlers.go`
- `apps/backend/internal/secrets/user_visible_store.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/backendapp/main.go`
- Focused secret, profile, workspace-delete, and PostgreSQL tests

## Inputs

- Spec sections `Scope and permissions`, `Data model`, and `API surface`.
- ADR sections `Two user-visible secret scopes` and `Scope-specific resolution APIs`.
- Existing authentication ownership semantics in ADR-2026-07-24.

## Dependencies

None.

## TDD sequence

1. Add failing store/service tests for migration defaults, workspace authorization, list modes,
   immutable scope, cascade deletion, and consumption policies.
2. Add failing agent/executor profile tests for Workspace refs and stale runtime resolution.
3. Implement the schema, service boundary, handlers, and backend wiring.
4. Refactor shared env validation only after behavior is green; rerun internal integration adapter
   tests to prove no credential regression.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
```

When PostgreSQL test infrastructure is available, also run the repository's env-gated PostgreSQL
schema replay target documented in `apps/backend/AGENTS.md`.

## Risks

- Do not reinterpret backend-owned deterministic secret IDs as user Workspace secrets.
- Do not authorize a Workspace secret using only its `user_id`; validate the workspace boundary too.
- Runtime profile resolvers must enforce Global scope even for legacy rows that bypass save-time
  validation.

## Output contract

Report the schema/migration shape, scope list contract, workspace authorization adapter, global-only
consumer enforcement, files changed, exact tests run, and residual risks. Update this task and the
plan status only after the focused and broad backend checks pass.

## Result

Implemented Global and Workspace metadata, replay-safe schema upgrades, user/workspace authorization,
scope-specific list/reveal operations, workspace-secret cleanup, and Global-only agent/executor
profile enforcement. Existing secret IDs and encrypted payloads remain unchanged during migration.

Focused secret, repository, service, profile, lifecycle, executor, and backend tests passed; the
full `make -C apps/backend test` and `make -C apps/backend lint` checks also passed. PostgreSQL
schema replay coverage is included in the env-gated tests; this workspace did not provide a
`KANDEV_TEST_POSTGRES_DSN`, so those tests were compile-checked and skipped locally.
