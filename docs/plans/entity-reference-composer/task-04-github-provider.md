---
id: "04-github-provider"
title: "GitHub mention provider"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 04: GitHub Mention Provider

## Acceptance

- Issue and PR search projections preserve immutable REST/node identity through PAT and `gh` paths.
- Plain text becomes backend-generated title-only GitHub search and respects workspace repository/org scope.
- Issue and PR groups remain separate, cached/scoped safely, bounded, cancellable, and safely classified.
- Provider authorizers validate the workspace-owned repository/organization scope and canonical GitHub host.

## Verification

```bash
cd apps/backend && go test ./internal/github/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/github/models.go`
- `apps/backend/internal/github/client_helpers.go`
- `apps/backend/internal/github/pat_client.go`
- `apps/backend/internal/github/gh_client.go`
- `apps/backend/internal/github/service_mentions.go`
- `apps/backend/internal/github/service_mentions_test.go`
- `apps/backend/internal/mentions/provider_github.go`
- `apps/backend/internal/mentions/provider_github_test.go`

## Dependencies

Task 01.

## Inputs

ADR identity rule; workspace settings search wrappers; existing search-cache and workspace-scope tests.

## Output contract

Report identity propagation/query generation/cache scope, files changed, exact tests, blockers, risks, and mark task/plan done.
