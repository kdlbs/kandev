---
id: "01-workspace-configuration"
title: "Workspace configuration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 01: Workspace Configuration

## Acceptance

- Azure configuration and PATs are isolated by workspace and survive service reconstruction.
- Only canonical Azure DevOps Services organization URLs are accepted.
- Config test/save/copy/delete and auth-health persistence are covered by failing-first tests.

## Verification

- `rtk go test ./internal/azuredevops/... -run 'Test(Config|Store|OrganizationURL|Copy)'` from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/models.go`
- `apps/backend/internal/azuredevops/store.go`
- `apps/backend/internal/azuredevops/service.go`
- `apps/backend/internal/azuredevops/provider.go`
- Corresponding `*_test.go` files.

## Inputs

- Spec: Data Model, Permissions, and Failure Modes.
- Patterns: `internal/jira/{models,store,service,provider}.go` and shared integration secret/health utilities.

## Output Contract

Report behavior added, files changed, RED/GREEN commands, blockers, residual security risks, and set this task plus its plan checkbox to done.
