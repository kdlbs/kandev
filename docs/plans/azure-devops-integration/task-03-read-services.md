---
id: "03-read-services"
title: "Work-item and PR read services"
status: done
wave: 2
depends_on: ["01-workspace-configuration", "02-rest-client"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 03: Work-Item And PR Read Services

## Acceptance

- Workspace-scoped HTTP routes expose project/repository discovery, WIQL search, work-item detail, PR lists, PR detail, and feedback.
- Missing, invalid, forbidden, rate-limited, and unavailable responses have stable HTTP behavior.
- A mock client/controller can deterministically seed all browse and feedback states for E2E.

## Verification

- `rtk go test ./internal/azuredevops/... -run 'Test(Service|Controller|Mock)'` from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/controller.go`
- `apps/backend/internal/azuredevops/handlers.go`
- `apps/backend/internal/azuredevops/service_reads.go`
- `apps/backend/internal/azuredevops/mock_client.go`
- `apps/backend/internal/azuredevops/mock_controller.go`
- Corresponding `*_test.go` files.

## Inputs

- Completed Tasks 01-02.
- Patterns: Jira HTTP workspace scoping and GitLab browse/feedback controllers.

## Output Contract

Report route contracts, files changed, RED/GREEN commands, mock controls, blockers, and set this task plus its plan checkbox to done.
