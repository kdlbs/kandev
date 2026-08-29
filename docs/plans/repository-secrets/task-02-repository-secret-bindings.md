---
id: "02-repository-secret-bindings"
title: "Persist repository secret bindings"
status: done
wave: 2
depends_on: ["01-scoped-secret-storage"]
plan: "plan.md"
spec: "../../specs/workspaces/requirements/repository-secrets.md"
---

# Task 02: Persist Repository Secret Bindings

## Acceptance

- Repository create/get/list/update/event DTOs carry secret-only key-to-ID bindings and no values.
- Explicit replacement is atomic with repository mutation; omission preserves and empty clears.
- Keys and binding counts follow shared profile env rules.
- A repository accepts visible Global or same-workspace secrets only.
- Secret deletion leaves a broken binding; repository/workspace deletion removes bindings.
- List paths batch-load bindings and migration replay works on SQLite and PostgreSQL.

## Files likely touched

- `apps/backend/internal/task/models/models.go`
- `apps/backend/internal/task/repository/interface.go`
- `apps/backend/internal/task/repository/sqlite/base_schema.go`
- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/repository_entity.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/handlers/repository_handlers.go`
- `apps/backend/internal/task/dto/dto.go`
- `apps/web/lib/types/http.ts`
- Repository persistence, service, handler, DTO, migration, and event tests

## Inputs

- Completed Task 01 scope-specific metadata and validation operations.
- Spec `Repository secret binding`, `API surface`, and `Persistence guarantees`.
- Existing repository manual-save and find-or-create/backfill semantics.

## Dependencies

Task 01.

## TDD sequence

1. Add failing schema/repository tests for create, batch list, replacement, dangling secret IDs, and
   cascades.
2. Add failing service/handler tests for scope validation, key validation, omission versus clear,
   atomic rollback, and event projection.
3. Implement normalized persistence and transactional repository mutations.
4. Extend TypeScript transport types only; leave UI behavior to Tasks 05 and 06.

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps/web && pnpm run typecheck
```

## Risks

- Do not add a secret foreign key: hard deletion must preserve the broken reference.
- Automated repository backfills must not clear bindings by serializing an absent field as `[]`.
- Repository event consumers must receive IDs only and never trigger a reveal.

## Output contract

Report the normalized schema, transactional replacement behavior, omission/clear semantics, scope
validation, event/DTO shape, files changed, tests run, and residual risks.

## Result

Implemented the normalized `repository_secret_bindings` table, repository DTO/event projection,
atomic create/update replacement, omission-versus-empty semantics, batched reads, scope-aware
reference validation, and cascade behavior that preserves dangling references after secret deletion.
SQLite and env-gated PostgreSQL replay/round-trip coverage are included. Backend tests, lint, and
frontend typecheck passed.
