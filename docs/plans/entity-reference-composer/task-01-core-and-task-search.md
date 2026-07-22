---
id: "01-core-and-task-search"
title: "Core external mention search"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 01: Core External Mention Search

## Acceptance

- Normalized provider service validates plain queries, runs bounded providers concurrently, and returns deterministic partial groups with safe statuses.
- Registry uses provider descriptors, owns provider identity/canonical ref construction, and accepts an arbitrary namespaced fake provider without native-provider switches, proving a future plugin bridge can use the same seam.
- Registry binds a unique `(provider, kind)` reference authorizer so search destinations and later submissions use the same provider-owned scope checks.
- HTTP handler implements the spec contract without central route registration yet.
- Kandev tasks are not registered in this provider registry because existing task discovery remains owned by `@`.

## Verification

```bash
cd apps/backend && go test ./internal/mentions/...
```

## Files likely touched

- `apps/backend/pkg/api/v1/mentions.go`
- `apps/backend/internal/mentions/types.go`
- `apps/backend/internal/mentions/service.go`
- `apps/backend/internal/mentions/handler.go`
- `apps/backend/internal/mentions/service_test.go`
- `apps/backend/internal/mentions/handler_test.go`

## Dependencies

None.

## Inputs

Spec API/failure/plugin-compatibility sections; ADR provider boundary; ADR 0043 stable DTO rules and RPC direction.

## Output contract

Report contract/type choices, ranking/cap/timeout behavior, files changed, exact test result, blockers, risks, and set this task plus plan checkbox to done.
