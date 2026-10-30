---
id: "07-entra-app-registration"
title: "Publish the Kandev Entra application"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.4
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 07: Publish the Kandev Entra application

## Summary

External maintainer action. Register the multi-tenant public client that makes
Microsoft sign-in available by default, then ship its client ID.

## In scope

- In a Kandev-owned Entra tenant: register a multi-tenant application
  ("Accounts in any organizational directory"), platform "Mobile and desktop
  applications", redirect URIs `http://localhost` and `http://127.0.0.1` (the
  settings page origin; Entra ignores the port for loopback), "Allow public client
  flows" enabled, delegated API permission Azure DevOps `user_impersonation`.
- Complete publisher verification.
- Set `azuredevops.DefaultOAuthClientID` and its config-catalog default; add a
  test that the default is non-empty.

## Out of scope

- Any other code change.

## Acceptance

- Signing in to a real organization with no operator configuration succeeds
  in a local Kandev build.
- The consent screen shows a verified publisher.

## Verification

From `apps/backend`:

```bash
go test ./internal/azuredevops/... ./internal/common/config/... -count=1
```

Plus the manual sign-in above, recorded here.

## Files likely touched

- `apps/backend/internal/azuredevops/oauth_settings.go`
- `apps/backend/internal/common/config/catalog.go`

## Dependencies

None for registration. Shipping the ID after Task 02.

## Risks

- Publisher verification needs a Microsoft Partner Network account.

## Parallelism

`parallel-safe`

## Results

Pending.
