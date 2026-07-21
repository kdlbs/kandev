---
id: "02-rest-client"
title: "REST client"
status: done
wave: 1
depends_on: ["01-workspace-configuration"]
plan: "plan.md"
spec: "../../specs/azure-devops-integration/spec.md"
---

# Task 02: REST Client

## Acceptance

- A PAT-authenticated HTTP client reads projects, repositories, WIQL results, work-item batches, PRs, reviewers, threads, linked work items, and policy evaluations.
- Requests are context-aware and bounded; credentials and upstream response bodies are redacted from user-visible errors.
- WIQL hydration preserves result order, batches at 200, and tolerates omitted work items.

## Verification

- `rtk go test ./internal/azuredevops/... -run 'TestRESTClient'` from `apps/backend`.

## Files Likely Touched

- `apps/backend/internal/azuredevops/client.go`
- `apps/backend/internal/azuredevops/rest_client.go`
- `apps/backend/internal/azuredevops/client_models.go`
- Corresponding `*_test.go` files.

## Inputs

- Spec: API Surface and Failure Modes.
- Patterns: `internal/gitlab/pat_client.go` and its `httptest` coverage.

## Output Contract

Report endpoints implemented, files changed, RED/GREEN commands, API-version exceptions, blockers, and set this task plus its plan checkbox to done.
