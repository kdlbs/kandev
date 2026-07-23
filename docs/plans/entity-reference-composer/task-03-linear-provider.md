---
id: "03-linear-provider"
title: "Linear mention provider"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 03: Linear Mention Provider

## Acceptance

- Linear adapter searches issues with structured `SearchFilter.Query` and explicit workspace scope.
- Results retain immutable issue ID, identifier, title, URL, and non-secret workspace/org scope.
- Missing config and upstream failures map to normalized statuses with cancellation preserved.
- Provider authorizer validates workspace organization scope and canonical Linear destinations.

## Verification

```bash
cd apps/backend && go test ./internal/linear/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/linear/service_mentions.go`
- `apps/backend/internal/linear/service_mentions_test.go`
- `apps/backend/internal/mentions/provider_linear.go`
- `apps/backend/internal/mentions/provider_linear_test.go`

## Dependencies

Task 01.

## Inputs

Spec provider contract; existing `SearchIssuesForWorkspace`, `LinearIssue.ID`, and GraphQL query tests.

## Output contract

Report mapping/scope, files changed, exact tests, blockers, risks, and mark task/plan done.
