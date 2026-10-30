---
id: "08-workspace-application"
title: "Workspace Entra application fields"
status: done
wave: 5
depends_on:
  - "05-settings-ui"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.4
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 08: Workspace Entra application fields

## Summary

Let the user enter the Application (client) ID in the Microsoft account block
(the redirect is the page origin, derived automatically), so a workspace can sign in with an Entra application its
organization already approves when the installation has no default.

## In scope

- `StartOAuthRequest.clientId` / `redirectUrl` with validation;
  `resolveOAuthApplication`; flow `client_id`; config `oauth_client_id` /
  `oauth_redirect_url`; renewal with the stored client; copy keeps the
  application; availability returns the installation defaults.
- Settings: one client ID field in the Microsoft block, default as placeholder,
  redirect sent as `window.location.origin` for a workspace client,
  sign-in disabled with an explanation without any client ID; the method option
  is always selectable.

## Out of scope

- Tenant ID field (discovered from the organization).

## Acceptance

- A workspace without an installation default signs in with its own client ID
  and redirect; renewal keeps using that client after the default changes.
- Invalid client IDs and non-loopback HTTP, relative, or query-bearing redirects
  are rejected.

## Verification

From `apps/backend`:

```bash
go test -race ./internal/azuredevops/ -count=1
```

From `apps/web`:

```bash
pnpm exec vitest run components/azure-devops/ lib/api/domains/azure-devops
pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/backend/internal/azuredevops/{oauth_application.go,service_oauth.go,service_oauth_renewal.go,oauth_store.go,store.go,models.go,controller_oauth.go,service_oauth_transitions.go}`
- `apps/web/components/azure-devops/{azure-devops-oauth-fields.tsx,azure-devops-settings.tsx,azure-devops-settings-form.ts}`
- `apps/web/lib/{types/azure-devops.ts,api/domains/azure-devops-oauth-api.ts}`, locales

## Dependencies

Task 05.

## Parallelism

`sequential`

## Results

- Backend: `go test -race ./internal/azuredevops/` pass; sqlguard pass;
  `lint-go-changed` 0 issues.
- Follow-up: the Redirect URL field was removed; the redirect follows the page
  origin so a port change needs no setting.
- Web: 85 Azure DevOps tests pass; ESLint clean; `tsc --noEmit` pass;
  `i18n:check` and `i18n:ratchet` pass (`oauthUnavailable` removed, four keys added).
- E2E: see plan verification results.
