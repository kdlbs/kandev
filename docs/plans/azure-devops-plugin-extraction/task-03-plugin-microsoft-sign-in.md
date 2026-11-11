---
id: "03-plugin-microsoft-sign-in"
title: "Microsoft sign-in"
status: in_progress
wave: 3
depends_on: 
  - "02-plugin-connection"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.10
  - AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003.1
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
---

# Task 03: Microsoft sign-in

## Summary

Port Entra sign-in from branch `feature/azure-devops-oauth-a-9e6` (`7226134a2`): tenant discovery, PKCE flows in workspace state, popup and paste completion, renewal under the workspace lock, sign-out, application in use, and the installation default `default_oauth_client_id`.

## In scope

- `internal/auth` Entra provider, flows, renewal locks, 401 retry; mock authority.
- `oauth.*` actions and the Microsoft account block with `useOAuthCompletion` and `usePopupRedirectWatch` (UI-01/02).
- Ported concurrency tests from `service_oauth_concurrency_test.go`.
- Manual validation against `https://dev.azure.com/contoso`: sign-in, forced renewal, clone with the token, sign-out; recorded in Results.

## Out of scope

- Publishing an Entra application.

## Acceptance

- Sign-in, forced renewal, and sign-out pass against the mock authority and against the real organization.
- Concurrent renewal and sign-in tests pass.

## ASCII UI preview

See UI-01 and UI-02 in the [plan](plan.md#ascii-ui-preview). This work order owns the rendered result of those views.

## Verification

Commands run from the Kandev repository root; the plugin worktree is its sibling.

```bash
(cd ../JnManso-kandev-plugin-azure-devops-main && go test -race ./internal/auth/... ./internal/plugin/... -count=1)
(cd ../JnManso-kandev-plugin-azure-devops-main && npm test)
```

## Files likely touched

- `../JnManso-kandev-plugin-azure-devops-main/internal/auth/`
- `../JnManso-kandev-plugin-azure-devops-main/ui/src/settings/`

## Dependencies

Task 02-plugin-connection

## Risks

- The Entra redirect to the page origin must be revalidated with the plugin route.

## Parallelism

`parallel-safe`

## Inputs

- [System design](../../specs/integrations/system-design/azure-devops-plugin.md)
- `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`, `docs/public/plugins-authoring.md`, and the `/create-kandev-plugin`, `/e2e`, and `/mobile-parity` skills where relevant

## Results

- 2026-11-05: implemented on plugin branch `feat/bootstrap` in commits `78b9612` (backend) and `f47ed04` (settings UI).
  - `internal/auth`: `entra.go` and its tests ported unchanged apart from exported names (tenant discovery from `X-VSS-ResourceTenant`, authorize URL with PKCE and the Azure DevOps scope, code exchange, refresh, `EntraError.RequiresSignIn`).
  - `internal/azure`: the branch's `Authorizer` seam (`auth.go`, `rest_client.go` `send`/`sendOnce` with one retry after a renewed 401, redaction) and its tests; `NewBearerAuthorizer` replaces the service-bound static authorizer; `ErrReauthRequired`.
  - `internal/store`:
    - The connection gains `oauthClientId`, `oauthTenantId`, `oauthAccount`, and `tokenExpiresAt`.
    - The secrets are `ws.<id>.oauth.refresh` and `ws.<id>.oauth.access`.
    - `Flows` stores `oauth.flow.<sha256(state)>` in workspace state. A claim is single-use (serialized in process) and rejects expired flows; expired flows are purged on start, and all flows on delete.
  - `internal/plugin`:
    - Actions `oauth.availability`, `oauth.start`, `oauth.complete`, and `oauth.sign_out`.
    - Completion requires the starting actor and workspace.
    - Every credential write holds a per-workspace lock (`locks.go`), so a sign-in wins over an in-flight renewal, as in `replaceOAuthConnection`.
    - Renewal uses the application that signed in.
    - `invalid_grant`-class errors set `reauthRequired`; transient errors do not.
    - Switching to a PAT removes the Microsoft tokens only after the save. Sign-out keeps the settings and bumps the generation. Copy of a Microsoft connection copies the client ID and requires sign-in.
    - Git credentials use the renewed access token, with expiry capped at the token's expiry.
    - Operator setting `default_oauth_client_id` (GUID, `config_schema`).
  - UI:
    - An authentication method selector, plus the Microsoft block: status, application in use, pending-application notice, sign in or sign in again, sign out, the client ID field, and the paste panel.
    - `useMicrosoftSignIn` ports `useOAuthCompletion` and `usePopupRedirectWatch`; the redirect URL is `window.location.origin`.
    - 27 strings taken from the branch's translations for all locales.
  - Action key renamed from `oauth.signOut` to `oauth.sign_out`. Kandev requires `^[a-z0-9][a-z0-9._-]*$`, `plugin-pack` does not check it, and install rejected the package. `server/manifest_test.go` now enforces the grammar.
- Verification:
  - Branch scenarios ported as tests: start validation, completion and rejection of foreign, reused, and expired attempts, a failed exchange keeping the PAT, renewal near expiry with rotation and the signing client, one retry after a rejected bearer, concurrent renewal issuing one refresh, a skipped renewal when already rotated, `invalid_grant` requiring sign-in, a transient failure not requiring it, a renewal in flight losing to a new sign-in, sign-out, method transitions, copy, delete, and Git credentials until sign-out.
  - `go test -race ./...` passes, stable over 3 runs. `make test` passes (Vitest 58), as do `verify-package` and `npm audit`.
  - Smoke on the disposable Kandev from the removal branch: the package installed, `oauth.availability` answered, and `oauth.start` for `https://dev.azure.com/contoso` discovered the real resource tenant and returned a PKCE authorize URL. Settings screenshots for desktop and phone show the Microsoft block.
- Open for this task's acceptance (owner, on `contoso`): clone with the token and sign out.
- 2026-11-09, owner test on `contoso` with the Azure CLI public client ID, through an SSH tunnel to `localhost`: real sign-in succeeded, and a forced expiry renewed the token and `connection.test` passed.
- 2026-11-09, fixes from that test: the client ID field now shows the application that signed in and sits above the sign-in actions. An empty field no longer reports a missing client ID or disables **Sign in again** while a signed-in application exists, matching the `oauth.start` resolution order.
