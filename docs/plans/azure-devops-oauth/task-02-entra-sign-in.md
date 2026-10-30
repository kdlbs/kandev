---
id: "02-entra-sign-in"
title: "Entra sign-in flow"
status: done
wave: 2
depends_on:
  - "01-credential-seam"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.6
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.7
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.8
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.6
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 02: Entra sign-in flow

## Summary

Let a workspace connect through Microsoft sign-in via the API: availability,
start, callback page, completion, persisted flow state, and a mock authority
for tests.

## In scope

- Startup keys `azureDevOps.oauthClientId` / `azureDevOps.oauthRedirectUrl`
  (`catalog.go`, `config.go`), `OAuthSettings`, `DefaultOAuthClientID`.
- `entra.go`: tenant discovery, authorize URL (PKCE S256, `prompt=select_account`,
  Azure DevOps `.default` + `offline_access openid profile`), code exchange.
- `azure_devops_oauth_flows` table for SQLite and Postgres; hashed state,
  ten-minute expiry, atomic single-use consume, user and workspace binding.
- `StartOAuth`, `CompleteOAuth` (tokens to secret store, refresh token first;
  config upsert with `auth_method='oauth'`, tenant, account, expiry; health
  reset and probe).
- Routes `GET /oauth/availability`, `POST /oauth/start`, `GET /oauth/callback`,
  `POST /oauth/complete`.
- Mock authorize and token routes in `mock_controller.go`; mock client ID.

## Out of scope

- Renewal and clone with OAuth (Task 03). Removing the PAT and other method
  transitions (Task 04). UI (Task 05).

## Acceptance

- With the mock authority, start then complete creates an OAuth connection whose
  `GET /config` reports `authMethod:"oauth"`, `oauthAccount`, `hasSecret:true`,
  and no token value.
- Reused, expired, unknown, foreign-user, and foreign-workspace states return
  400 and write nothing; an Entra `error` callback writes nothing.
- Availability is false with an empty client ID and true in the e2e profile.

## Verification

From `apps/backend`:

```bash
go test ./internal/azuredevops/... -run 'OAuth|Entra|Flow' -count=1
go test ./internal/common/config/... -count=1
go test ./internal/azuredevops/... -count=1
```

Include a Postgres flow-store case alongside `store_postgres_test.go`.

## Files likely touched

- `apps/backend/internal/common/config/{catalog.go,config.go}` and tests
- `apps/backend/internal/azuredevops/{entra.go,oauth_settings.go,oauth_flow_store.go,service_oauth.go,models.go,store.go,controller.go,mock_controller.go,provider.go}` and tests

## Dependencies

Task 01.

## Risks

- Tenant discovery must not follow redirects or send credentials.
- The callback HTML must contain no token and post only to its own origin.

## Parallelism

`sequential`

## Results

- Startup keys `azureDevOps.oauthClientId` / `azureDevOps.oauthRedirectUrl`
  (`KANDEV_AZURE_DEVOPS_OAUTH_CLIENT_ID` / `_REDIRECT_URL`) with redirect
  validation; `ResolveOAuthSettings` applies `DefaultOAuthClientID` (still empty
  until Task 07) and the local callback.
- `entra.go`: tenant discovery from `X-VSS-ResourceTenant` (zero, missing, or
  malformed falls back to `organizations`), PKCE authorize URL, code exchange and
  refresh; `EntraError` keeps only the OAuth error code.
- `azure_devops_oauth_flows` (hashed state, ten-minute expiry, single-use
  consume) plus OAuth columns on `azure_devops_configs`, using dialect tokens;
  same-DB replay test; env-gated Postgres test (skipped locally, no DSN).
- `StartOAuth` / `CompleteOAuth` with workspace and user binding; completion
  probes `connectionData` with the new token before persisting, stores the
  refresh token first, and removes the replaced PAT (pulled forward from Task 04).
- Routes `GET /oauth/availability`, `POST /oauth/start`, `GET /oauth/callback`
  (localized via `internal/i18n`, posts only to its own origin, `no-store`,
  `no-referrer`), `POST /oauth/complete`; stable error codes
  `azure_devops_oauth_*` and `azure_devops_reauth_required`.
- Mock mode: `mockEntra` and `GET /api/v1/azure-devops/mock/oauth/authorize`
  (relative redirect; `MockState.oauthError` simulates denial).
- `go test ./internal/azuredevops/ ./internal/common/config/ ./internal/i18n/`: pass.
- `go run ./cmd/sqlguard ./internal`: pass. `storeconformance -race`: pass.
- `scripts/lint-go-changed`: 0 issues.
