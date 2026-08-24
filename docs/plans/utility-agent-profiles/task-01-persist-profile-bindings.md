---
id: "01-persist-profile-bindings"
title: "Persist utility profile bindings"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/agents/requirements/utility-agent-profiles.md"
---

# Task 01: Persist utility profile bindings

## Intent

Make profile IDs and explicit binding states the durable utility-agent/default/call-history contract,
and migrate only legacy agent/model choices that identify one eligible profile.

## Acceptance

- Utility-agent CRUD and portable user settings read/write profile IDs; custom agents require an
  eligible profile while built-ins may inherit the default or remain explicitly unconfigured.
- The idempotent legacy migration selects exactly one eligible global profile and writes `explicit`,
  or writes `unconfigured` on zero/multiple matches without overwriting a saved binding.
- Legacy agent/model values remain available as read-only migration inputs until the new binding is
  repaired or a later cleanup migration removes them.
- Call history records the effective profile ID, and pre-migration call rows remain readable with an
  empty value.

## Files likely touched

- `apps/backend/internal/utility/models/models.go`
- `apps/backend/internal/utility/dto/dto.go`
- `apps/backend/internal/utility/store/interface.go`
- `apps/backend/internal/utility/store/sqlite.go`
- `apps/backend/internal/utility/store/sqlite_migration_test.go`
- `apps/backend/internal/utility/service/service.go`
- `apps/backend/internal/utility/service/service_test.go`
- `apps/backend/internal/utility/controller/controller.go`
- `apps/backend/internal/utility/profilebinding/resolver.go`
- `apps/backend/internal/utility/profilebinding/resolver_test.go`
- `apps/backend/internal/user/models/models.go`
- `apps/backend/internal/user/dto/dto.go`
- `apps/backend/internal/user/service/service.go`
- `apps/backend/internal/user/store/sqlite.go`
- `apps/backend/internal/backendapp/services.go`

## Dependencies

None.

## Parallelism

Sequential. This establishes shared persistence and API contracts consumed by every later task.

## Inputs

- Spec: `Data model`, `API surface`, `Failure modes`, and `Persistence guarantees`.
- Plan: `Canonical profile binding`, `Durable migration`, and `Utility CRUD and execution
preparation`.
- Existing patterns: utility store idempotent column migrations, backend-owned portable user
  settings from ADR 0041, agent settings `GetAgentProfile`/`ListAgentProfiles`, and soft-delete
  filtering in `StoreProfileResolver`.

## Verification

```bash
cd apps/backend && go test ./internal/utility/... ./internal/user/...
cd apps/backend && go test ./internal/backendapp/... -run 'Test.*Utility'
```

## Output contract

Report the new wire/storage fields, migration match rules and counts, files changed, exact tests
run, compatibility behavior, blockers, risks, and synchronized task/plan status. Do not change
one-shot process launch behavior or frontend controls in this task.

## Results

Implemented profile ID/state persistence, user default profile storage, eligibility-aware legacy migration, and effective profile IDs in call history. Verified with `go test ./internal/utility/... ./internal/user/...` and backend integration compilation.
