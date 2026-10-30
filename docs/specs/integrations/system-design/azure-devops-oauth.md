---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
created: 2026-10-22
owners:
  - kandev
---

# Azure DevOps Microsoft Sign-In System Design

## Purpose and boundaries

This design adds Microsoft Entra ID delegated sign-in as a second credential
method for the workspace-scoped Azure DevOps connection described in
[part 1](azure-devops-integration-01.md) and
[part 2](azure-devops-integration-02.md). The integration system owns it
because the credential belongs to the provider connection, not to a Kandev
user session.

The design uses, but does not own:

- The encrypted secret store (`internal/secrets`, wrapped by
  `internal/integrations/secretadapter`).
- The executor's server-side clone path (`internal/orchestrator/executor`) and
  `internal/repoclone` credential helper.
- Workspace authorization (`Service.authorizeWorkspaceAccess`) and the
  integration auth-health poller (`internal/integrations/healthpoll`).
- Typed startup configuration (`internal/common/config/catalog.go`).

Decision:
[ADR-2026-10-22-azure-devops-entra-public-client](../../../decisions/2026-10-22-azure-devops-entra-public-client.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001` | [Sign-in flow](#sign-in-flow), [HTTP API](#http-api), [Frontend](#frontend), [Security](#security) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002` | [Credential source](#credential-source), [Token renewal](#token-renewal), [Clone credentials](#clone-credentials), [Failure and recovery](#failure-and-recovery) |
| `REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003` | [Method transitions](#method-transitions), [Persistence](#persistence), [Security](#security) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `azuredevops.OAuthSettings` | Resolved application identity: client ID, redirect URL, authority host, availability. Built once at startup. |
| `azuredevops.entraClient` | Tenant discovery, authorization URL construction, code exchange, and refresh against the Microsoft identity platform v2.0 endpoints. |
| `azuredevops.oauthFlowStore` | Persists pending sign-in attempts in `azure_devops_oauth_flows`. |
| `azuredevops.Service` (OAuth methods) | `StartOAuth`, `CompleteOAuth`, credential resolution, renewal with per-workspace serialization, method transitions. |
| `azuredevops.credentialSource` | Returns the current REST authorization header value for a workspace, renewing when needed. Replaces the raw PAT argument of `ClientFactory`. |
| `azuredevops.RESTClient` | Sends `Basic` for PAT and `Bearer` for OAuth; retries once after renewal on a 401 for OAuth. |
| Executor `AzureDevOpsCredentialResolver` | New interface, mirroring `GitLabCredentialResolver`, that returns Basic clone credentials for one workspace. |
| Mock authority (`mock_entra.go`, `mock_controller.go`) | In `KANDEV_MOCK_AZURE_DEVOPS=true`, `mockEntra` replaces the Entra client and `GET /api/v1/azure-devops/mock/oauth/authorize` redirects to the relative callback (or with `MockState.oauthError`), so E2E never contacts Microsoft. |
| Web `useAzureDevOpsOAuthConnect` | Starts sign-in, owns the popup, receives the callback message, and drives the paste fallback. |
| Web `AzureDevOpsSettings` | Method selector, PAT fields or Microsoft sign-in block, signed-in identity. |

## Data and contracts

### Configuration

Two typed startup keys are added to `catalog.go` and a new
`AzureDevOpsConfig` section in `config.Config`:

| Key | Environment | Default |
| --- | --- | --- |
| `azureDevOps.oauthClientId` | `KANDEV_AZURE_DEVOPS_OAUTH_CLIENT_ID` | Kandev's published public client ID (`azuredevops.DefaultOAuthClientID`) |
| `azureDevOps.oauthRedirectUrl` | `KANDEV_AZURE_DEVOPS_OAUTH_REDIRECT_URL` | `http://localhost:<server.port>/api/v1/azure-devops/oauth/callback` |

Neither value is sensitive. They are the installation defaults. A workspace can
enter its own client ID in the Microsoft account block. The settings page always
sends `redirectUrl` set to its own origin (`window.location.origin`), with or
without a client ID, so a changed port or host needs no setting; the
configured redirect URL only applies to API callers that omit it. The backend validates them as a
GUID and as an HTTPS or loopback-HTTP URL without query or fragment; Microsoft
still checks the redirect against the application's registration, and the
popup watch completes sign-in when the redirect lands on that origin. The effective pair is stored on the flow row and,
after sign-in, in `oauth_client_id` / `oauth_redirect_url`. Renewal always uses
the stored client ID, because Entra binds refresh tokens to the client that
obtained them.

### Auth method

`AuthMethodOAuth = "oauth"` joins `AuthMethodPAT`. `configFromRequest` accepts
both. The web type becomes `AzureDevOpsAuthMethod = "pat" | "oauth"`.

`Config` gains read-only response fields:

- `oauthAccount` (`db:"oauth_account"`): signed-in display name or UPN from
  `connectionData`, for display only.
- `oauthTenantId` (`db:"oauth_tenant_id"`): tenant used for sign-in and
  renewal.
- `oauthClientId` / `oauthRedirectUrl` (`db:"oauth_client_id"` /
  `db:"oauth_redirect_url"`): the Entra application and redirect the workspace
  signed in with; renewal uses this client ID.
- `tokenExpiresAt` (`db:"token_expires_at"`): access-token expiry.
- `reauthRequired` (`db:"oauth_reauth_required"`): set when renewal returns a
  terminal Entra error; cleared by a successful sign-in.

`HasSecret` reports the credential of the stored method.

### Entra contract

- Authority: `https://login.microsoftonline.com/<tenant>/oauth2/v2.0/{authorize,token}`.
- Tenant: discovered per organization from the `X-VSS-ResourceTenant` header of
  an unauthenticated `HEAD https://dev.azure.com/<org>/_apis/connectionData`.
  A missing or all-zero value falls back to `organizations`.
- Scope: `499b84ac-1321-427f-aa17-267ca6975798/.default offline_access openid profile`.
- Public client: authorization code with PKCE `S256`; the token request sends
  `client_id` and `code_verifier` and no secret. The request is server-side, so
  it carries no browser `Origin`.
- `prompt=select_account`.
- The token response provides `access_token`, `refresh_token`, `expires_in`.
  A refresh response can rotate the refresh token.

### HTTP API

All routes stay under `/api/v1/azure-devops` and scope by `?workspace_id=`.

| Route | Request | Response |
| --- | --- | --- |
| `GET /oauth/availability` | none | `{available, defaultClientId, defaultRedirectUrl}`; `available` means the installation has a default application |
| `POST /oauth/start` | `{organizationUrl, defaultProjectId?, defaultProjectName?, clientId?, redirectUrl?}` | `{authUrl, state}` |
| `GET /oauth/callback` | `code`, `state` or `error`, `error_description` query | HTML page that posts `{type:"azure-devops-oauth-callback", code, state, error}` to `window.opener` with target origin `window.location.origin`, then closes |
| `POST /oauth/complete` | `{code, state}` | `Config` |
| `POST /oauth/sign-out` | none | `Config` (Microsoft account connections only; `ErrInvalidConfig` otherwise) |

Errors carry stable `code` values: `azure_devops_oauth_unavailable` (409),
`azure_devops_oauth_invalid_state` (400), `azure_devops_oauth_failed` (400,
Entra error code only), `azure_devops_oauth_provider_unavailable` (502, tenant
discovery failed), and `azure_devops_reauth_required` (401). The callback page
copy comes from `internal/i18n` (`azureDevOps.oauthCallback.*`).

`POST /config` with `authMethod:"oauth"` updates only the default project and
rejects an organization change with `ErrInvalidConfig` ("sign in again").
`POST /config/test` with no body tests stored credentials of either method; a
body with `authMethod:"oauth"` and no stored OAuth credential returns
`{ok:false}` with a sign-in-required error.

## Sign-in flow

1. The UI calls `POST /oauth/start`. The service authorizes workspace access,
   validates the organization URL, discovers the tenant, generates a 32-byte
   state and PKCE verifier, and stores a flow row with the SHA-256 state hash,
   workspace, Kandev user ID (from the request's `authn` identity, empty when
   per-user auth is off), organization URL, default project, tenant,
   verifier, redirect URL, and a ten-minute expiry.
2. The UI opens `authUrl` in a popup. The user signs in at Microsoft.
3. Entra redirects to `/oauth/callback`. The page forwards `code`/`state` (or
   `error`) to the opener and closes. When the opener is absent or the origin
   differs, the page shows a "copy this URL into Kandev" message instead.
4. The opener calls `POST /oauth/complete` with the code and state. For the
   paste fallback, the UI parses `code` and `state` from the pasted URL and
   calls the same route.
5. `CompleteOAuth` atomically consumes the flow row (`consumed_at IS NULL AND
   expires_at > now`), checks that the caller is the starting user and still
   has workspace access, exchanges the code, probes `connectionData` with the
   new access token for `oauthAccount`, then applies
   [method transitions](#method-transitions).

## Credential source

`ClientFactory` becomes `func(cfg *Config, auth Authorizer) Client`, where
`Authorizer` (`auth.go`) exposes `Header(ctx) (string, error)`,
`Renew(ctx, rejectedHeader string) (bool, error)`, and `Redact(text) string`.

- PAT: `patAuthorizer` returns `Basic base64(":"+pat)`; `Renew` reports
  `false`, so a 401 surfaces unchanged.
- OAuth: `oauthAuthorizer` returns `Bearer <access>` from
  `Service.accessToken(ctx, workspaceID)`; `Renew` calls
  `Service.renewAccessToken` with the rejected token.
- Sign-in: `staticBearerAuthorizer` probes `connectionData` with the freshly
  exchanged token before anything is stored.

`resolveCredentials`, `clientForWorkspace`, mentions, branch listing, the watch
poller, and `TestConnectionForWorkspace` obtain an `Authorizer` instead of a PAT
string. `TestConnectionForWorkspace` with a submitted PAT builds a transient
`patAuthorizer` and persists nothing. The mock factory ignores the authorizer.

`RESTClient.send` asks the authorizer for the header per request. On a 401 it
calls `Renew` once with the header it sent and, only when that renewed the
credential, repeats the request once with the same body. A renewal error such as
`ErrReauthRequired` is returned instead of the 401. Error redaction removes both
the PAT and any bearer value.

## Token renewal

`Service.accessToken` returns the stored access token when `tokenExpiresAt` is
more than five minutes away. Otherwise it takes the per-workspace lock
(`workspaceLocks`), re-checks freshness, and renews only if the token is still
expiring.

`Service.renewAccessToken` (the 401 path) runs under the same lock:

1. If the stored access token already differs from the rejected one, return
   without renewing.
2. Reveal the refresh token, call the token endpoint for the stored tenant.
3. Persist the new refresh token first (keeping the old one when none is
   returned), then the access token, then `token_expires_at`, using a context
   detached from request cancellation with a ten-second limit.

Renewal is shared by REST, the health poller, and clone credentials, so
concurrent consumers produce at most one token request per workspace.
Sign-in completion, sign-out, and credential removal take the same lock, so a
renewal that started for a replaced credential cannot write its tokens back.
Sign-in snapshots the previous Microsoft tokens and restores them if any later
write fails, keeping the existing connection usable.

## Clone credentials

The executor gains `AzureDevOpsCredentialResolver` with
`ResolveAzureDevOpsCloneCredentials(ctx, workspaceID) (username, password string, err error)`
and `SetAzureDevOpsCredentialResolver`, wired from `backendapp/main.go` like
`SetGitLabCredentialResolver(services.GitLab)`.

`ensureClonedWithWorkspaceAuthForSessionAndState` and
`refreshAzureDevOpsRepositoryForSessionWithState` call the resolver instead of
revealing `cloneauth.AzureDevOpsPATKey` directly. The service returns
`("kandev", pat)` for PAT and `("kandev", accessToken)` for OAuth after
renewal. Azure Repos accepts an Entra access token as the Basic password, so
`EnsureWorkspaceClonedWithBasicAuth*` and `RefreshWorkspaceRepositoryWithBasicAuth*`
stay unchanged. A clone that runs longer than the token lifetime is unaffected
because Git authenticates at connection start.

## Method transitions

| Transition | Order |
| --- | --- |
| Sign-in succeeds (new, re-sign-in, or PAT to OAuth) | Store refresh token, access token, then upsert config with `auth_method='oauth'`, organization, project, tenant, account, expiry, `oauth_reauth_required=0`; reset auth health and probe; then delete the PAT key. |
| Save PAT on an OAuth connection | Existing `setConfigWithPAT` path with `auth_method='pat'`; after the config write, delete OAuth secret keys and clear OAuth columns. |
| Delete connection | Delete secrets of both methods, then the config row, with the existing rollback snapshot extended to OAuth keys. |
| Copy OAuth connection | `CopyConfigToWorkspace` writes organization and project with `auth_method='oauth'`, no secrets, and `oauth_reauth_required=1`. |
| Sign out | `Service.SignOutOAuth` takes the renewal lock, deletes both Microsoft secret keys, clears `oauth_account` and `token_expires_at`, sets `oauth_reauth_required=1`, and resets auth health; organization, project, tenant, client ID, watchers, and task links stay. `HasSecret` becomes false, so the UI shows Sign in with Microsoft. |

Readers always follow `auth_method`. A leftover secret from a failed best-effort
deletion is never read and is removed by the next successful transition.

## Failure and recovery

| Condition | Behavior |
| --- | --- |
| Tenant discovery fails | `start` returns 502 with a localized-key error; no flow row. |
| Entra returns `error` to the callback | Opener shows cancellation (`access_denied`) or failure; flow row expires unused. |
| Unknown, expired, consumed, or foreign state | `complete` returns 400; nothing is written. |
| Code exchange fails | 400 with the Entra error code only; nothing is written. |
| Renewal returns `invalid_grant` or `interaction_required` | Set `oauth_reauth_required=1`, return `ErrReauthRequired`; health shows "sign in again". Later operations fail fast without calling Entra until a sign-in succeeds. |
| Renewal network or 5xx failure | Return a transient error; no flag; the next operation retries. |
| Persistence fails after a rotated refresh token | Refresh token is written first, so the retained token remains valid. |
| Backend restarts during sign-in | The persisted flow row survives; completion within ten minutes still works. |

## Persistence

`azure_devops_configs` gains nullable or defaulted columns through
`ensureConfigColumn`: `oauth_account TEXT NOT NULL DEFAULT ''`,
`oauth_tenant_id TEXT NOT NULL DEFAULT ''`, `oauth_client_id`,
`oauth_redirect_url` (text, default empty), `token_expires_at` (timestamp),
`oauth_reauth_required` (boolean, default false). Existing rows keep
`auth_method='pat'`, so PAT connections need no migration.

New table, created for SQLite and Postgres:

```sql
CREATE TABLE IF NOT EXISTS azure_devops_oauth_flows (
  state_hash TEXT PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  user_id TEXT NOT NULL DEFAULT '',
  organization_url TEXT NOT NULL,
  default_project_id TEXT NOT NULL DEFAULT '',
  default_project_name TEXT NOT NULL DEFAULT '',
  tenant_id TEXT NOT NULL,
  client_id TEXT NOT NULL DEFAULT '',
  pkce_verifier TEXT NOT NULL,
  redirect_url TEXT NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  consumed_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL
)
```

Expired and consumed rows are deleted when a new flow starts and when the
workspace's config is deleted.

Secret keys, owned by `azuredevops`:
`azure_devops:<ws>:oauth_refresh_token` and
`azure_devops:<ws>:oauth_access_token`. The PAT key stays in `cloneauth`.

## Frontend

- `lib/types/azure-devops.ts`: widen `AzureDevOpsAuthMethod`; add the response
  fields above.
- `hooks/domains/azure-devops/use-azure-devops-oauth-connect.ts`: modeled on
  `components/jira/use-jira-oauth-connect.ts`; validates `event.origin`,
  `event.source === popup`, and the pending state; shows the inline paste panel
  as soon as the popup opens, and reports a blocked popup. While a sign-in is
  pending it also reads the popup's address once it is back on Kandev's origin,
  so an operator redirect URL without the callback path completes without
  pasting. The first result clears the pending state, so the callback message
  and the address watch never complete twice.
- Progress (opening sign-in, completing, signing out) is announced in one
  translated `role="status"` region; action labels stay stable. Controls use
  `controlSizingClassName("standard")` (28px desktop, 44px on phones and coarse
  pointers). One completion runs at a time, and the redirect watch stops when
  the popup closes while the paste panel stays available.
- `hooks/domains/azure-devops/use-azure-devops-oauth.ts`: installation default
  client ID and sign-out.
- `components/azure-devops/azure-devops-settings-form.ts`: method-aware pure
  form helpers (`requestFromForm`, `canSubmitForm`, `signedInWithMicrosoft`).
- `lib/api/domains/azure-devops-oauth-api.ts`: availability, start, complete.
- `components/azure-devops/azure-devops-settings.tsx`: an Authentication method
  `Select` after the organization URL. PAT shows the existing PAT input and
  help. Microsoft account shows a sign-in block with identity, re-sign-in
  state, the Sign in with Microsoft button, and the Application (client) ID
  field (installation default as placeholder; sign-in disabled with an
  explanation when no client ID is available). Form `authMethod` and
  `oauthClientId` come from config; `startRequestFromForm(form, origin)` always
  adds the page origin as redirect and the client ID only when entered.
- All copy goes through `t()` in the `azuredevops` namespace across seven
  locales; "Microsoft" stays verbatim as a brand noun.
- Phone layout keeps the existing single-column field order; the sign-in button
  and paste dialog actions are full width with touch-sized targets.

## Security

- The state is single-use, hashed at rest, expires after ten minutes, and binds
  the workspace and starting user; completion re-checks workspace access.
- PKCE prevents code interception; the verifier never leaves the server.
- The redirect URL comes from configuration only. The callback page posts only
  to its own origin and contains no token.
- Tokens live only in the encrypted secret store. They are excluded from
  `Config`, logs, error bodies, URLs, task metadata, and agent environments.
  The executor passes them through the existing file-descriptor credential
  helper.
- The client ID is public. Entra admin consent policies apply unchanged; a
  tenant that blocks user consent shows Microsoft's admin-approval page, which
  surfaces as a cancellation in Kandev.

## Observability

- Structured zap logs: `azure devops oauth started|completed|failed` with
  workspace ID and Entra error code; `azure devops oauth renewed|reauth_required`.
  Token values and authorization codes are never logged.
- Auth health continues to record `last_ok`/`last_error` per workspace; the
  re-sign-in condition uses a stable error key the UI maps to localized copy.

## Related decisions

- [ADR-2026-10-22-azure-devops-entra-public-client](../../../decisions/2026-10-22-azure-devops-entra-public-client.md)
- [ADR-2026-07-20-provider-neutral-remote-repositories](../../../decisions/2026-07-20-provider-neutral-remote-repositories.md)
