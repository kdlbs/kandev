---
id: "04-task-pr-wiring"
title: "Task PR persistence and backend wiring"
status: done
wave: 2
depends_on: ["03-read-services"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 04: Task PR Persistence And Backend Wiring

## Acceptance

- Azure PR associations validate task repository ownership, persist across restart, list by workspace, and refresh from Azure.
- The Azure service and health/PR refresh lifecycle are wired non-fatally into backend startup and route registration.
- Azure repositories use the persisted `azure_devops` provider without changing GitHub or GitLab behavior.

## Verification

- `rtk go test ./internal/azuredevops/... ./internal/backendapp/... -run 'Test.*Azure'` from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/store_task_pr.go`
- `apps/backend/internal/azuredevops/service_task_pr.go`
- `apps/backend/internal/azuredevops/poller.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/main.go`
- Relevant provider parsing and cleanup files found during implementation.

## Inputs

- Completed Task 03.
- Patterns: `internal/gitlab/store.go`, task-MR services, and backendapp GitLab wiring.

## Output Contract

Report persistence/wiring changes, files changed, RED/GREEN commands, lifecycle behavior, blockers, and set this task plus its plan checkbox to done.
