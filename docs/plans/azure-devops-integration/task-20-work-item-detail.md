---
id: "20-work-item-detail"
title: "Work-item detail contracts"
status: done
wave: 11
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/azure-devops-integration.md"
---

# Task 20: Work-Item Detail Contracts

## Acceptance

- Azure client/service/controller return current PAT identity, hydrated
  work-item detail, sanitized description inputs, and the normalized allowlist
  of available planning fields.
- Discussion reads non-deleted comments newest-first in bounded pages and passes
  Azure's opaque continuation token without interpretation.
- Detail and discussion errors are independently typed so the frontend can keep
  core fields visible while retrying only the failed section.

## Verification

- `make -C apps/backend test`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/client_models.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/rest_client_test.go`
- `apps/backend/internal/azuredevops/service_reads.go`
- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/controller_test.go`
- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/mock_controller.go`
- `apps/backend/internal/azuredevops/mock_client_test.go`

## Dependencies

None.

## Parallelism

Sequential. Client, service, controller, and mock share the new response types.

## Inputs

- Spec: detail API, planning-field allowlist, discussion failure behavior.
- Microsoft Work Item Get:
  `https://learn.microsoft.com/en-us/rest/api/azure/devops/wit/work-items/get-work-item?view=azure-devops-rest-7.1`.
- Microsoft Comments Get:
  `https://learn.microsoft.com/en-us/rest/api/azure/devops/wit/comments/get-comments?view=azure-devops-rest-7.1`.
- Microsoft Profile Get:
  `https://learn.microsoft.com/en-us/rest/api/azure/devops/profile/profiles/get?view=azure-devops-rest-7.1`.

## Risks

- Comments use `7.1-preview.4`; keep this version local to the comments client
  and retain response-size bounds and context cancellation.

## Output Contract

Report normalized detail fields, paging contract, RED/GREEN commands, files
changed, fixtures, risks, and update task/plan status.
