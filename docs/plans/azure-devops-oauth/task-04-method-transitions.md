---
id: "04-method-transitions"
title: "Method transitions, copy, and delete"
status: done
wave: 3
depends_on:
  - "02-entra-sign-in"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.5
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 04: Method transitions, copy, and delete

## Summary

Guarantee exactly one usable credential per workspace across every change of
method, organization, copy, and delete.

## In scope

- `CompleteOAuth` removes the PAT after the config write.
- `SetConfigForWorkspace` accepts `oauth` for project-only updates, rejects an
  organization change on OAuth, and on a PAT save over OAuth clears OAuth
  secrets and columns after the config write.
- `DeleteConfigForWorkspace` snapshot and rollback cover OAuth keys; flow rows
  for the workspace are purged.
- `CopyConfigToWorkspace` copies OAuth settings without secrets and sets
  `oauth_reauth_required`.

## Out of scope

- UI.

## Acceptance

- Each transition in the design's Method transitions table has a test proving
  the old credential is used until the new one is stored and removed after.
- A failed write in any step leaves the previous method usable.

## Verification

From `apps/backend`:

```bash
go test ./internal/azuredevops/... -run 'Transition|Copy|Delete|SetConfig' -count=1
go test ./internal/azuredevops/... -count=1
```

## Files likely touched

- `apps/backend/internal/azuredevops/{service.go,service_oauth.go,copy.go,store.go}`
- `apps/backend/internal/azuredevops/service_oauth_transitions_test.go` (new)

## Dependencies

Task 02. Can run in parallel with Task 03.

## Risks

- `service.go` and `service_oauth.go` are shared with Task 03; land one first
  if run in parallel.

## Parallelism

`parallel-safe with Task 03 after coordination on service_oauth.go`

## Results

- `SetConfigForWorkspace` with `authMethod: "oauth"` saves only the default
  project of an existing sign-in for the same organization; any other case
  (new workspace, organization change, PAT connection) is rejected with
  `ErrInvalidConfig` ("sign in with Microsoft for this organization").
- Saving a PAT clears Microsoft tokens and columns after the config write; a
  failed PAT save keeps the Microsoft connection usable.
- `CompleteOAuth` removes the PAT after the config write (Task 02).
- Delete removes the PAT, Microsoft tokens, and pending sign-ins.
- Copy of a Microsoft account connection writes settings only, removes any
  target credential, and sets `oauth_reauth_required` on the target.
- Test connection for `authMethod: "oauth"` without a matching sign-in returns
  `{ok:false}`.
- `go test -race ./internal/azuredevops/`: pass. `scripts/lint-go-changed`: 0 issues.
