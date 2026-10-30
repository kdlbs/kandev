---
id: "03-token-renewal"
title: "Token renewal and OAuth clone credentials"
status: done
wave: 3
depends_on:
  - "02-entra-sign-in"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.6
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 03: Token renewal and OAuth clone credentials

## Summary

Make an OAuth connection behave like a PAT connection for every consumer:
bearer REST calls, proactive and 401-driven renewal, re-sign-in detection, and
fresh access tokens for server-side Git clone and refresh.

## In scope

- `oauthAuthorizer`; `Service.accessToken` with a five-minute renewal margin.
- Per-workspace renewal lock, stale-token skip, refresh-token-first persistence
  with a detached ten-second context, `token_expires_at` update.
- `RESTClient` single retry after `Renew` on 401 for OAuth.
- `invalid_grant` / `interaction_required` set `oauth_reauth_required` and a
  stable health error key; later operations fail fast until sign-in.
- `ResolveAzureDevOpsCloneCredentials` returns `("kandev", accessToken)` for OAuth.

## Out of scope

- UI rendering of the re-sign-in state (Task 05).

## Acceptance

- Concurrent callers near expiry cause exactly one token request; a 401 renews
  once and retries once; a second 401 surfaces as unauthorized.
- Clone credentials for an expired OAuth connection trigger renewal and return
  the new token; no token appears in errors or logs.
- Terminal renewal errors flag the connection and stop further token requests
  until a successful `CompleteOAuth` clears the flag.

## Verification

From `apps/backend`:

```bash
go test -race ./internal/azuredevops/... -run 'Renew|AccessToken|OAuth|RESTClient' -count=1
go test ./internal/orchestrator/executor/... -run 'Azure' -count=1
```

Manual check before closing: with an operator-supplied client ID, sign in to a
real organization and confirm clone, refresh, and work-item reads succeed.
Record the result here.

## Files likely touched

- `apps/backend/internal/azuredevops/{auth.go,service_oauth.go,service_oauth_renewal.go,rest_client.go,store.go,service.go}` and tests

## Dependencies

Task 02.

## Risks

- Holding the renewal lock across a slow token call blocks REST callers for
  that workspace only; keep the token HTTP timeout at 15 seconds.

## Parallelism

`sequential`

## Results

- `oauthAuthorizer` sends `Bearer` from `Service.accessToken`, which renews
  inside a five-minute margin; the freshness check repeats under the
  per-workspace lock so concurrent callers issue one token request.
- `renewAccessToken` (401 path) skips a rotation that a concurrent caller
  already performed; `RESTClient.send` retries exactly once after a successful
  renewal and replays the request body.
- Terminal Entra errors (`invalid_grant`, `interaction_required`,
  `consent_required`, `login_required`) or a missing refresh token set
  `oauth_reauth_required`; later calls fail fast with `ErrReauthRequired`
  until a sign-in clears it. Transient errors do not set the flag.
- `ResolveAzureDevOpsCloneCredentials` returns `("kandev", accessToken)` for
  Microsoft account connections.
- `go test -race ./internal/azuredevops/`: pass. Executor Azure tests: pass.
  `scripts/lint-go-changed`: 0 issues.
- Manual check (2026-10-28, dev instance, organization `ptbcp`, workspace client
  ID `04b07795-…` entered in the Microsoft account block): sign-in succeeded;
  with `token_expires_at` forced into the past, a project read renewed the token
  without user interaction (new expiry about 80 minutes ahead, no re-sign-in
  flag); creating a task on `IT.DIT/BCP.ATMOffer.Portal.Experience.Backend`
  cloned the repository through the backend with the Microsoft access token as
  the Basic password (the workspace had no PAT; clone runs with global and
  system Git config disabled, so ambient Git Credential Manager was not used).
  The clone's `.git/config` holds no credential, header, or helper.
