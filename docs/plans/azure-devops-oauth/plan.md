---
created: 2026-10-22
status: building
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
legacy_specs: []
---

# Implementation Plan: Azure DevOps Microsoft Sign-In

## Overview

Add Microsoft account (Entra ID OAuth, authorization code + PKCE) as an
alternative to the PAT for a workspace's Azure DevOps connection. The user picks
**Microsoft account** in settings, clicks **Sign in with Microsoft**, signs in in
a popup, and returns connected. REST calls, the health poller, and server-side
Git clone/refresh then use the signed-in identity, renewing tokens
automatically.

Order and reason:

1. Task 01 removes the raw-PAT assumption from every consumer behind a
   credential abstraction, with PAT behavior unchanged. Everything after it plugs
   a second credential into one seam.
2. Tasks 02-04 add the backend sign-in, renewal, and method transitions. After
   Task 03 the backend is fully usable through the API with the mock authority.
3. Task 05 adds the settings UI. Task 06 proves desktop and phone flows end to
   end and updates public docs.
4. Task 07 is the external prerequisite for production availability: publish the
   Kandev Entra application and ship its client ID. It does not block Tasks
   01-06, which run against the mock authority and operator configuration.

Requirements: [azure-devops-oauth.md](../../specs/integrations/requirements/azure-devops-oauth.md).
Design: [azure-devops-oauth.md](../../specs/integrations/system-design/azure-devops-oauth.md).
Decision: [ADR-2026-10-22-azure-devops-entra-public-client](../../decisions/2026-10-22-azure-devops-entra-public-client.md).

## Scope

### In scope

- `authMethod: "oauth"` for Azure DevOps workspace connections.
- Entra tenant discovery, sign-in start/callback/complete, persisted flow state.
- Access-token renewal shared by REST, health, and Git clone/refresh.
- Method switching, organization change, copy, and delete rules.
- Settings UI on desktop and phone, seven-locale copy, E2E, public docs.
- Startup keys `azureDevOps.oauthClientId` and `azureDevOps.oauthRedirectUrl`.

### Out of scope

- Microsoft personal accounts, service principals, managed identities,
  device-code flow, Azure DevOps Server.
- Per-workspace client ID fields in the UI.
- `az` CLI credentials for agent pull-request creation.
- Per-Kandev-user Azure identities within one workspace.

## Technical approach

### Credential seam (Task 01)

- `internal/azuredevops/auth.go` (new): `Authorizer` interface with
  `Header(ctx) (string, error)` and `Renew(ctx, stale string) error`;
  `patAuthorizer`.
- `client.go`: `ClientFactory func(cfg *Config, auth Authorizer) Client`;
  `DefaultClientFactory` and `provider.go` mock factory updated.
- `rest_client.go`: `RESTClient` holds an `Authorizer`; `do` sets
  `Authorization` from it; redaction covers the header secret.
- `service.go` `resolveCredentials` returns `(*Config, Authorizer, error)`;
  callers in `service_reads.go`, `service_mentions.go`, `poller.go`,
  `watch_service.go`, `watch_cleanup.go`, `work_item_mutations.go` follow.
- Executor: `AzureDevOpsCredentialResolver` +
  `SetAzureDevOpsCredentialResolver` in `orchestrator/executor/executor.go`;
  `ensureClonedWithWorkspaceAuthForSessionAndState` and
  `refreshAzureDevOpsRepositoryForSessionWithState` use it.
  `Service.ResolveAzureDevOpsCloneCredentials` returns `("kandev", pat)`.
  Wiring in `backendapp/main.go` beside `SetGitLabCredentialResolver`.

### Sign-in (Task 02)

- `internal/common/config`: `AzureDevOpsConfig{OAuthClientID, OAuthRedirectURL}`
  plus two `catalog.go` entries; `azuredevops.OAuthSettings` (`oauth_settings.go`) resolved in
  `provider.go`; `DefaultOAuthClientID` constant (empty until Task 07).
- `entra.go` (new): tenant discovery via `X-VSS-ResourceTenant`, authorize URL,
  code exchange, refresh; base authority URL injectable for tests and mock.
- `oauth_flow_store.go` (new): `azure_devops_oauth_flows` table (SQLite and
  Postgres), insert, atomic consume, purge.
- `service_oauth.go` (new): `StartOAuth`, `CompleteOAuth`, secret keys
  `OAuthRefreshTokenKey`/`OAuthAccessTokenKey`, OAuth columns via
  `ensureConfigColumn`.
- `controller.go`: `GET /oauth/availability`, `POST /oauth/start`,
  `GET /oauth/callback`, `POST /oauth/complete`.
- `mock_controller.go`: mock authorize (redirects to callback with a code) and
  token endpoints; mock client ID so availability is true in the e2e profile.

### Renewal and clone (Task 03)

- `oauthAuthorizer`, `Service.accessToken`, per-workspace renewal lock,
  stale-token skip, refresh-token-first persistence, `oauth_reauth_required`.
- REST 401 retry once after `Renew`.
- `ResolveAzureDevOpsCloneCredentials` returns `("kandev", accessToken)` for
  OAuth.

### Transitions (Task 04)

- `SetConfigForWorkspace`: accept `oauth` (project-only updates; reject
  organization change); PAT save on an OAuth connection clears OAuth secrets and
  columns.
- `CompleteOAuth` deletes the PAT after config write.
- `DeleteConfigForWorkspace` and rollback snapshot cover OAuth keys and flows.
- `copy.go` `CopyConfigToWorkspace`: OAuth copies settings only and sets
  `oauth_reauth_required`.

### Frontend (Task 05)

- `apps/web/lib/types/azure-devops.ts`, `lib/api/domains/azure-devops-api.ts`.
- `components/azure-devops/use-azure-devops-oauth-connect.ts` (new) after
  `components/jira/use-jira-oauth-connect.ts`.
- `components/azure-devops/azure-devops-oauth-fields.tsx` (new) for the
  Microsoft block and paste dialog; `azure-devops-settings.tsx` gets the method
  `Select` and drops the hardcoded `authMethod: "pat"`.
- Locale keys in `apps/web/src/locales/*/azuredevops.json` for all seven locales.

## ASCII UI preview

Structural requirements: method selector placed directly after Organization URL;
PAT input only for the PAT method; Microsoft block only for the Microsoft method;
identity and re-sign-in state inside the Microsoft block; the existing Test,
Save/Update, Copy, and Delete actions keep their positions. Spacing and icons
are illustrative.

### UI-01: Settings > Integrations > Azure DevOps, PAT method (desktop)

```text
+--------------------------------------------------------------------+
| Organization URL                                                   |
| [https://dev.azure.com/acme                                     ]  |
| Authentication method                                              |
| [Personal access token                                        v]  |
| Default project               Personal access token (i)            |
| [Platform               v]    [**************************     ]   |
|                                                                    |
| [Test connection]                          [Delete] [Update]       |
+--------------------------------------------------------------------+
```

### UI-02: Microsoft method, not signed in (desktop)

```text
+--------------------------------------------------------------------+
| Organization URL                                                   |
| [https://dev.azure.com/acme                                     ]  |
| Authentication method                                              |
| [Microsoft account                                            v]  |
| +----------------------------------------------------------------+ |
| | Sign in with your work or school account. Kandev never sees    | |
| | your password.                                                 | |
| | [ Sign in with Microsoft ]                                     | |
| +----------------------------------------------------------------+ |
| Default project  (available after sign-in)                         |
| [Optional               v]  (disabled)                             |
+--------------------------------------------------------------------+
```

Disabled state (AC-001.3): the `Microsoft account` option is disabled with the
hint "Microsoft sign-in is not configured for this installation."

### UI-03: Microsoft method, signed in / sign-in required (desktop)

```text
| +----------------------------------------------------------------+ |
| | (check) Signed in as Ana Silva            [Sign in again]      | |
| +----------------------------------------------------------------+ |

| +----------------------------------------------------------------+ |
| | (!) Microsoft sign-in expired. Sign in again to reconnect.     | |
| | [ Sign in with Microsoft ]                                     | |
| +----------------------------------------------------------------+ |
```

### UI-04: Paste fallback panel (inline in the Microsoft block)

```text
+--------------------------------------------------+
| Finish Microsoft sign-in                         |
| If the sign-in window did not return to Kandev,  |
| copy the address from its address bar and paste  |
| it here.                                         |
| [http://localhost/api/v1/azure-devops/oauth/...] |
|                          [Cancel] [Complete]     |
+--------------------------------------------------+
```

### UI-05: Phone, Microsoft method

```text
+------------------------------+
| Organization URL             |
| [https://dev.azure.com/acme] |
| Authentication method        |
| [Microsoft account       v]  |
| Signed in as Ana Silva       |
| [   Sign in again          ] |  <- full width, 44px target
| Default project              |
| [Platform                v]  |
| [   Test connection        ] |
| [   Update                 ] |
+------------------------------+
```

The paste panel renders inline with full-width, touch-sized actions and
a wrapping input; no horizontal scroll (AC-001.9).

## Tests

| AC | Evidence |
| --- | --- |
| 003.7 | `rest_client_test.go` `TestRESTClientPATAuthorizationHeader`; executor `TestEnsureClonedAzureUsesResolver` (Task 01) |
| 001.4, 001.8 | `service_oauth_test.go` `TestStartOAuthStoresHashedFlow`, `TestCompleteOAuthRejectsReusedForeignExpiredState` (Task 02) |
| 001.5, 003.1 | `service_oauth_test.go` `TestCompleteOAuthPersistsTokensThenRemovesPAT` (Tasks 02/04) |
| 002.1, 002.4 | `service_oauth_renewal_test.go` `TestAccessTokenRenewsNearExpiry`, `TestConcurrentRenewalSingleRequest`; `rest_client_test.go` `TestRESTClientOAuthRetriesOnceAfter401` (Task 03) |
| 002.2 | `service_oauth_renewal_test.go` `TestCloneCredentialsUseFreshAccessToken` (Task 03) |
| 002.5 | `TestRenewalInvalidGrantSetsReauthRequired` (Task 03) |
| 003.2-003.5 | `service_oauth_transitions_test.go` (Task 04) |
| 003.6 | `TestOAuthTokensNeverInConfigOrErrors` (Tasks 02/03) |
| 001.1-001.3, 001.6, 001.7 | `azure-devops-settings.test.tsx`, `use-azure-devops-oauth-connect.test.ts` (Task 05) |

## E2E tests

| Flow | AC | File / project |
| --- | --- | --- |
| Switch to Microsoft account, sign in via mock popup, see identity, browse work items | 001.1, 001.2, 001.4, 001.5, 002.1 | `e2e/tests/integrations/azure-devops-oauth.spec.ts` / `chromium` |
| Paste-URL completion and cancelled sign-in keep the PAT connection | 001.6, 001.7, 003.1 | same file |
| Phone: method select, sign-in, paste dialog without overflow | 001.9 | `e2e/tests/integrations/mobile-azure-devops-oauth.spec.ts` / `mobile-chrome` |

## Work orders

- [x] [Task 01: Credential seam for REST and clone](task-01-credential-seam.md)
- [x] [Task 02: Entra sign-in flow](task-02-entra-sign-in.md)
- [x] [Task 03: Token renewal and OAuth clone credentials](task-03-token-renewal.md)
- [x] [Task 04: Method transitions, copy, and delete](task-04-method-transitions.md)
- [x] [Task 05: Settings UI](task-05-settings-ui.md)
- [x] [Task 06: End-to-end proof and public docs](task-06-e2e-docs.md)
- [ ] [Task 07: Publish the Kandev Entra application](task-07-entra-app-registration.md)
- [x] [Task 08: Workspace Entra application fields](task-08-workspace-application.md)
- [x] [Task 09: Microsoft account sign-out](task-09-sign-out.md)

## Verification results

- Go: `go test -race ./internal/azuredevops/` pass; config, i18n, executor
  (Azure), orchestrator and backendapp (Azure) pass; `cmd/sqlguard` pass;
  `storeconformance -race` pass; `scripts/lint-go-changed` 0 issues.
- Web: Vitest `components/azure-devops/` + `lib/api/domains/azure-devops*` 79
  passed; `tsc --noEmit` pass (with a 12 GB heap in this environment);
  ESLint on changed files clean; `i18n:check` and `i18n:ratchet` pass.
- E2E: desktop 4 passed, phone 3 passed (new and existing Azure specs).
- Open: Task 07 and the manual sign-in against a real Azure DevOps organization.

## Risks

- **Consent policy:** many corporate tenants block user consent to unverified
  multi-tenant apps. Mitigation: publisher verification (Task 07), admin-consent
  guidance in public docs, operator client-ID override.
- **Entra token as Git Basic password:** expected to work for Azure Repos; Task
  03 must confirm against a real organization before Task 07 closes.
- **Guest users:** tokens must come from the organization's resource tenant;
  tenant discovery covers this, and a wrong-tenant token surfaces as a 401 test
  failure.
- **Remote Kandev:** the localhost redirect cannot reach a remote backend; the
  paste fallback or an operator-registered redirect URL is required.
- **Dev mode origin:** with a separate Vite port, `postMessage` cannot reach the
  opener; the paste fallback covers it (same as Jira).
