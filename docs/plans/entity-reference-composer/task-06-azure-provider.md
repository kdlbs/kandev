---
id: "06-azure-provider"
title: "Azure DevOps mention provider"
status: completed
wave: 2
depends_on: ["01-core-and-task-search"]
plan: "plan.md"
spec: "../../specs/ui/entity-reference-composer.md"
---

# Task 06: Azure DevOps Mention Provider

## Acceptance

- Work-item search generates escaped WIQL internally and returns organization-scoped immutable IDs.
- PR search adds a project-level active-PR read path, filters titles server-side, and avoids browser or unbounded repository fan-out.
- Adapter respects workspace config/default project plus bounded project discovery, cancellation, caps, and safe errors.
- Provider authorizers validate organization URL/path and project scope, not only the shared host origin.

## Verification

```bash
cd apps/backend && go test ./internal/azuredevops/... ./internal/mentions/...
```

## Files likely touched

- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/client_models.go`
- `apps/backend/internal/azuredevops/service_mentions.go`
- `apps/backend/internal/azuredevops/service_mentions_test.go`
- `apps/backend/internal/azuredevops/rest_client_test.go`
- `apps/backend/internal/mentions/provider_azure.go`
- `apps/backend/internal/mentions/provider_azure_test.go`

## Dependencies

Task 01.

## Inputs

Spec all-source contract; existing workspace credential resolver, WIQL path, project/repository listing, and PR models.

## Output contract

Report search/fan-out bounds and identity, files changed, exact tests, blockers, risks, and mark task/plan done.
