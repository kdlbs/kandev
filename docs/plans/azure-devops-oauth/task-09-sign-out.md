---
id: "09-sign-out"
title: "Microsoft account sign-out"
status: done
wave: 6
depends_on:
  - "08-workspace-application"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.8
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 09: Microsoft account sign-out

## Summary

Add a Sign out action to a signed-in Microsoft account connection that removes
the stored Microsoft tokens while keeping the connection's settings, so Kandev
stops acting as the user until a new sign-in.

## In scope

- `Service.SignOutOAuth` and `POST /api/v1/azure-devops/oauth/sign-out`.
- Store update that clears the account and expiry and requires sign-in while
  keeping tenant and client ID.
- Settings: Sign out button in the Microsoft block, success and failure toasts,
  seven-locale copy.

## Out of scope

- Ending the Microsoft browser session or revoking tokens at Microsoft.

## Acceptance

- After sign-out, no Microsoft secret remains, `hasSecret` is false, the
  organization, project, client ID, and watchers are unchanged, and clone
  credentials fail with `ErrReauthRequired` until a new sign-in.
- Sign-out on a PAT connection or an unconfigured workspace is rejected.
- The UI shows Sign in with Microsoft after sign-out on desktop and phone.

## Verification

From `apps/backend`:

```bash
go test -race ./internal/azuredevops/ -count=1
```

From `apps/web`:

```bash
pnpm exec vitest run components/azure-devops/ lib/api/domains/azure-devops
pnpm e2e:run --host --shards 1 --project chromium tests/integrations/azure-devops-oauth.spec.ts
```

## Files likely touched

- `apps/backend/internal/azuredevops/{service_oauth_transitions.go,oauth_store.go,controller_oauth.go}` and tests
- `apps/web/components/azure-devops/{azure-devops-oauth-fields.tsx,azure-devops-settings.tsx}`, `lib/api/domains/azure-devops-oauth-api.ts`, locales, E2E spec

## Dependencies

Task 08.

## Parallelism

`sequential`

## Results

- `Service.SignOutOAuth` (renewal lock held) deletes both Microsoft secret keys,
  `Store.MarkOAuthSignedOut` clears account and expiry and requires sign-in,
  auth health is reset; organization, project, tenant, and client ID stay.
  `POST /api/v1/azure-devops/oauth/sign-out` returns the config; PAT or missing
  connections return 400.
- Settings: Sign out button next to Sign in again while signed in (touch-sized on
  phone), success and failure toasts, four new keys in seven locales.
- Go: `go test -race ./internal/azuredevops/` pass; `lint-go-changed` 0 issues;
  sqlguard pass.
- Web: 92 Azure DevOps Vitest tests pass; ESLint clean; `tsc --noEmit` pass;
  `i18n:check` and `i18n:ratchet` pass.
- E2E: `azure-devops-oauth.spec.ts` (chromium) 4 passed including the new
  sign-out flow; `mobile-azure-devops-oauth.spec.ts` (mobile-chrome) 1 passed
  with the touch-size check.
