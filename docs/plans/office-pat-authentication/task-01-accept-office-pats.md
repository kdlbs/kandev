---
id: "01-accept-office-pats"
title: "Accept verified user credentials in Office"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-AUTH-001
acceptance_criteria:
  - AC-AUTH-AUTH-001.4
  - AC-AUTH-AUTH-001.7
  - AC-AUTH-AUTH-001.11
  - AC-AUTH-AUTH-001.12
  - AC-AUTH-AUTH-001.13
  - AC-AUTH-AUTH-001.16
system_design:
  - ../../specs/auth/system-design/credential-lookup-failures.md
---

# Task 01 Accept verified user credentials in Office

## Summary

A verified personal access token (PAT) reaches the user-facing Office API and uses the caller's workspace permissions.
Office continues to validate agent JWTs in its own middleware and runtime handlers.

## Scope

- In `AgentAuthMiddleware`, detect PAT bearers with `auth.IsPATFormat(strings.TrimSpace(token))`.
- Read the identity with `authn.FromGin` and pass the request to the workspace guard.
- When authentication is enabled, an unresolved PAT has no identity and must receive the existing Office `401 {"error":"invalid token"}` response. When authentication is disabled, the synthetic identity means a PAT-shaped bearer is treated as if no bearer was sent.
- Keep the raw token for the non-PAT agent JWT path.
- Add a sentence in the existing personal access token section of the public auth reference. Explain that user-facing Office routes use the same bearer header and runtime routes require an agent JWT.

Global auth owns credential resolution, lookup errors, cookie precedence, and disabled mode.
The Office workspace guard owns workspace authorization. Runtime callbacks verify the agent JWT independently.

## Acceptance

1. An owning PAT returns the project's data with HTTP 200. A valid PAT without access to the workspace returns 404.
2. An unrecognized PAT without a valid cookie returns 401. A valid cookie takes precedence over a PAT on the same request. Required credential lookup errors return 503.
3. The agent JWT tests pass, and a user PAT receives 401 on an agent runtime route. With auth disabled, a credentialless Office request and a PAT-shaped bearer both succeed on the metadata route.

## Regression test

Add `TestOfficePATAuthentication` in `internal/backendapp/office_pat_auth_test.go`.
Use `newEnabledAuthService` for the auth fixture and call `MintToken` and `Login` to create the credentials.
Use `newRunSubscriptionCheckHarness` to seed an owned workspace and a second workspace outside the user's scope.
Configure `officeTestServices` with a real `AgentService` and `ProjectService`.
Mount `httpmw.Middleware` before `mountOfficeRoutes`.

In `owning_PAT`, send the PAT to `GET /api/v1/office/workspaces/:wsId/projects` and assert HTTP 200 with a seeded project.
Before the correction, this case must fail with `401 {"error":"invalid token"}`.
Cover a PAT with surrounding whitespace, which global auth already normalizes.
Use the same PAT on the other workspace's project list to assert 404.
Use an unregistered PAT on `/api/v1/office/meta` to assert 401 without relying on workspace authorization.
Send a valid session and the unregistered PAT to the owned project list to assert cookie precedence.
Send the valid PAT to `/api/v1/office/runtime/projects` to assert `401 {"error":"invalid runtime token"}`.

In `TestOfficePATAuthenticationDisabledMode`, use `newDisabledAuthService` on the same route mount.
Assert 200 on Office metadata both without a bearer and with a PAT-shaped bearer.

## Verification

Run the regression before the production edit and verify that it fails at the expected status assertion.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp -run '^TestOfficePATAuthentication$/^owning_PAT$' -count=1)
```

Run these commands from the repository root after the correction.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/backendapp ./internal/office/agents ./internal/auth/httpmw \
  -run '^Test(OfficePATAuthentication(DisabledMode)?|OfficeRouteGroupMountsScopeGuard|OfficeScope(AgentTokenNotBrowserIdentityDecides|ConfinesAgentJWTToItsOwnWorkspace|AllowsAgentJWTInsideItsOwnWorkspace)|AgentAuthMiddleware_RejectsCrossWorkspaceToken|EnabledModePATAuthenticates|Credential(StoreFailureReturns503|FallbackKeepsLookupErrors|FailureRouteCompatibility))$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/office/agents/handler.go apps/backend/internal/backendapp/office_pat_auth_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
(cd apps/backend && go build -trimpath -tags fts5 ./...)
```

## Files likely touched

- `apps/backend/internal/office/agents/handler.go`
- `apps/backend/internal/backendapp/office_pat_auth_test.go`
- `docs/public/authentication.md`
- `docs/plans/office-pat-authentication/plan.md`
- `docs/plans/office-pat-authentication/task-01-accept-office-pats.md`

## Dependencies

This work order runs independently.

## Risk

A PAT-shaped bearer can reach Office before authentication succeeds. With authentication enabled, reject it before handler dispatch unless a verified user identity exists. With authentication disabled, global auth supplies the synthetic identity and Office treats the PAT-shaped bearer like no bearer.

## Parallelism

`sequential`

## Inputs

- [Authentication and workspace segregation requirements](../../specs/auth/requirements/auth.md#req-auth-auth-001-opt-in-authentication--multi-user-segregation)
- [Office bearer authentication design](../../specs/auth/system-design/credential-lookup-failures.md#office-bearer-authentication)
- `apps/backend/internal/auth/httpmw/middleware.go` for credential resolution and Office deferral.
- `apps/backend/internal/backendapp/office_scope.go` for the production route mount and workspace guard.
- `apps/backend/internal/backendapp/office_scope_test.go` and `auth_test.go` for route and auth fixtures.

## Results

Completed on 2026-11-24.

The owning-PAT regression failed before the middleware change with `401 {"error":"invalid token"}` and passed after it. The focused Go test command above passed all Office PAT, Office agent JWT, and credential-store error cases.

`go build -trimpath -tags fts5 ./...` passed from `apps/backend`. Changed-file Go lint, public documentation tests and validation, and `git diff --check` also passed.
