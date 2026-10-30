---
id: "01-credential-seam"
title: "Credential seam for REST and clone"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.7
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 01: Credential seam for REST and clone

## Summary

Replace every raw-PAT consumer with one credential abstraction, keeping PAT
behavior byte-for-byte identical. This is a behavior-preserving refactor that
gives later tasks a single seam for the OAuth credential.

## In scope

- `Authorizer` interface and `patAuthorizer` (design: Credential source).
- `ClientFactory` signature change; `DefaultClientFactory`, mock factory, and
  every `clientFn(cfg, pat)` call site.
- `RESTClient` header from the authorizer; error-body redaction of the header
  secret.
- Executor `AzureDevOpsCredentialResolver` + setter; clone and pre-worktree
  refresh use it; `Service.ResolveAzureDevOpsCloneCredentials` returns
  `("kandev", pat)`; wiring in `backendapp/main.go`.

## Out of scope

- Any OAuth type, route, column, or UI.

## Acceptance

- All existing `internal/azuredevops`, executor Azure, and backendapp wiring
  tests pass unchanged in assertion.
- New tests prove the PAT `Basic` header, redaction, and that the executor
  obtains Azure clone credentials only through the resolver (no direct
  `cloneauth.AzureDevOpsPATKey` reveal remains in `orchestrator/executor`).

## Verification

From `apps/backend`:

```bash
go test ./internal/azuredevops/... -count=1
go test ./internal/orchestrator/executor/... -run 'Azure' -count=1
go test ./internal/backendapp/... -run 'AzureDevOps' -count=1
bash ../../scripts/lint-go-changed $(git diff --name-only -- '*.go' | sed 's|^apps/backend/||')
```

## Files likely touched

- `apps/backend/internal/azuredevops/{auth.go,client.go,rest_client.go,provider.go,service.go,service_reads.go,service_mentions.go,poller.go,watch_service.go,watch_cleanup.go,work_item_mutations.go}`
- `apps/backend/internal/azuredevops/*_test.go` fakes using `ClientFactory`
- `apps/backend/internal/orchestrator/executor/{executor.go,executor_resume.go}` and Azure tests
- `apps/backend/internal/backendapp/main.go`

## Dependencies

None.

## Risks

- `ClientFactory` is used by many test fakes; update them mechanically without
  changing assertions.
- The executor must keep failing closed when no resolver is wired.

## Parallelism

`sequential`

## Results

- `Authorizer` (`auth.go`) replaces the raw PAT in `ClientFactory`,
  `NewRESTClient`, and `resolveCredentials`; `patAuthorizer` keeps the exact
  `Basic base64(":"+pat)` header and PAT redaction.
- The executor resolves Azure Repos clone and refresh credentials only through
  `AzureDevOpsCredentialResolver`; `azuredevops.Service` implements it and
  `backendapp/main.go` wires it through the orchestrator. No executor code reads
  `cloneauth.AzureDevOpsPATKey` anymore.
- `go test ./internal/azuredevops/ -count=1`: pass.
- `go test ./internal/orchestrator/executor/... -run 'Azure|EnsureCloned|ResolveTaskRepoInfo'`: pass.
- `go test ./internal/backendapp/ -run AzureDevOps`: pass.
- `scripts/lint-go-changed`: 0 issues.
