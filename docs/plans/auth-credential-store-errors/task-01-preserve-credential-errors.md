---
id: "01-preserve-credential-errors"
title: "Preserve credential lookup errors end to end"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AUTH-AUTH-001
acceptance_criteria:
  - AC-AUTH-AUTH-001.1
  - AC-AUTH-AUTH-001.3
  - AC-AUTH-AUTH-001.4
  - AC-AUTH-AUTH-001.11
  - AC-AUTH-AUTH-001.12
  - AC-AUTH-AUTH-001.13
  - AC-AUTH-AUTH-001.14
  - AC-AUTH-AUTH-001.15
  - AC-AUTH-AUTH-001.16
system_design:
  - ../../specs/auth/system-design/credential-lookup-failures.md
---

# Task 01: Preserve credential lookup errors end to end

## Summary

Preserve unavailable results from required auth reads through HTTP and gateway authentication.
Deliver resolver signatures, organization wiring, transport handling, diagnostics, and regression coverage together so the work order compiles independently.

## In scope

- Implement the [resolver and transport contracts](../../specs/auth/system-design/credential-lookup-failures.md).
- Change session/PAT resolvers and `ResolveRequest` to three-result contracts, with no partial identity on failure.
- Distinguish credential-store missing rows, account missing rows, disabled users, and missing/inactive organizations from infrastructure errors.
- Propagate organization read errors through `OrgStatusChecker`, `orgUsable`, and backend wiring.
- Extract the organization callback into a small private adapter in `backendapp/auth.go` if needed to test its real error mapping.
- Migrate other organization-gate callers while retaining their established fail-closed behavior.
- Preserve cookie precedence and successful bearer fallback. Retain an earlier read error if the fallback fails.
- Return the specified 503 body before allowlist/deferred processing after an undecidable lookup. Preserve genuine 401 behavior.
- Migrate gateway `AuthPolicy.ResolveToken`, including the capability branch's token-stripping lookup.
- Log each required-read failure with fixed operation/category fields. Exclude unsanitized error text and secret material.
- Retain best-effort session/PAT activity writes and expired-session cleanup.
- Update all existing service, SSO, and gateway test call sites for the new signatures.

## Out of scope

Postgres pool policy, credential caches, request retries, password-login classification, capability-only account checks, browser production code, and database migrations.

## Acceptance

1. Required read failures produce no identity and the specified 503 before protected handlers, upgrades, or proxy forwarding. Invalid credentials retain their existing rejection.
2. The complete resolver, fallback, optional-write, organization, diagnostics, route, concurrency, and recovery matrix in the plan passes.
3. Every caller compiles. Disabled auth, credentialless public paths, independent non-PAT validation, and successful proxy token stripping retain their existing behavior.

## TDD and regression matrix

Start with `TestCredentialStoreFailureReturns503` in `internal/auth/httpmw/credential_errors_test.go`.
Create real session/PAT credentials against the SQL stores and prove a 200 baseline.
Make the credential reader unavailable, then assert 503 with `code: persistence_unavailable` and no `WWW-Authenticate` or cookie deletion.
Assert that the protected handler did not run.
Before correction, both cases must fail with the observed 401 challenge.

Use isolated readers or controlled request contexts for required-read faults.
Use an account-repository wrapper for account failures and the organization checker for organization failures.
Exercise the backend's actual organization adapter to prove missing organization and read error remain distinct.
Use the existing concrete auth store rather than replacing its verification path with a success stub.
Do not add a production test-only fault endpoint.

Add the named tests from the plan, including:

- Missing, revoked, malformed, and expired credentials, missing/disabled accounts, missing/suspended orgs, and non-PAT bearers.
- Valid cookie plus bad bearer without a bearer read, invalid cookie plus valid PAT, failed cookie plus valid PAT, and failed cookie plus invalid PAT.
- Empty/no credentials, nil organization checker, empty organization ID, and nil logger.
- Credentialless public routes, auth/me and app-state with unreadable cookies, deferred MCP/gateway routes, and auth disabled.
- Optional write/cleanup failures with valid and expired credentials.
- Context cancellation/deadline and arbitrary required-read errors, without an auth challenge.
- Interleaved valid and failed requests under parallel load. Only requests with a failed required read return 503, never 401.
- A reversible fault followed by success with the same cookie/PAT, without login, reissue, or credential deletion.
- Gateway errors before upgrade/forwarding and successful PAT stripping beside unrelated application tokens.
- Diagnostic capture containing operation/category but no injected secret markers, hashes, DSN, headers, URL, or raw database message.

## Verification

Run each command from the repository root. Do not overlap these checks.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/auth/httpmw -run '^TestCredentialStoreFailureReturns503$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/auth/... ./internal/gateway/websocket ./internal/backendapp -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/auth ./internal/auth/httpmw ./internal/gateway/websocket -run 'Test(Credential|ConcurrentCredential|QueryTokenStore|ProxyTokenLookup)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/kandev -run '^$')
git ls-files -m -o --exclude-standard -z -- 'apps/backend/*.go' | xargs -0 -r bash scripts/lint-go-changed
git diff --check
```

Record the historical RED evidence from the temporary diagnostic test and the permanent regression's GREEN result after implementation.
The backendapp suite covers startup wiring; the command-package compile catches broader typed call-site breakage.

## Files likely touched

- `apps/backend/internal/auth/service_credentials.go`
- `apps/backend/internal/auth/service.go`
- `apps/backend/internal/auth/service_test.go`
- `apps/backend/internal/auth/service_sso_test.go`
- `apps/backend/internal/auth/service_credentials_errors_test.go` (new)
- `apps/backend/internal/auth/httpmw/middleware.go`
- `apps/backend/internal/auth/httpmw/credential_errors_test.go` (new)
- `apps/backend/internal/backendapp/main.go`
- `apps/backend/internal/backendapp/auth.go`
- `apps/backend/internal/backendapp/auth_credential_errors_test.go` (new)
- `apps/backend/internal/gateway/websocket/access.go`
- `apps/backend/internal/gateway/websocket/access_auth_errors_test.go` (new)
- `apps/backend/internal/gateway/websocket/port_proxy_auth_test.go`
- Other test stubs found by `rg -n 'ResolveBearer|ResolveSessionToken|ResolveRequest|ResolveToken|orgUsable|OrgStatusChecker' apps/backend`.

## Dependencies

None. Read the root and backend `AGENTS.md` before implementation.

## Risks

Keep auth-store `ErrNotFound`, account `sql.ErrNoRows`, and organization `ErrOrgNotFound` distinct.
Do not return success after an unresolved required read or forward a possibly valid PAT to the preview application.
The general 503 branch must precede deferral so external MCP cannot rewrite the failure to 401.

## Parallelism

`sequential`

## Inputs

- [Auth requirements](../../specs/auth/requirements/auth.md), the listed acceptance criteria.
- [Credential lookup design](../../specs/auth/system-design/credential-lookup-failures.md), all sections.
- [Parent plan](plan.md), evidence and test matrix.
- Existing `service_test.go`, `httpmw/middleware_test.go`, `backendapp/auth_test.go`, and `websocket/port_proxy_auth_test.go` fixture patterns.

## Results

Complete. The historical RED evidence remains the temporary diagnostic test recorded in the parent plan; this work order added the permanent regression gates after the design checkpoint.

Changed production and test surfaces:

- `apps/backend/internal/auth/service.go`, `service_credentials.go`: tri-state resolvers, `ErrCredentialLookupUnavailable`, bounded diagnostics, optional-write isolation, and the `(bool, error)` organization checker.
- `apps/backend/internal/auth/httpmw/middleware.go`: cookie/bearer error retention, 503 before public/deferred handling, and the stable `persistence_unavailable` contract.
- `apps/backend/internal/gateway/websocket/access.go`: tri-state query-token handling for upgrades and the capability branch.
- `apps/backend/internal/backendapp/main.go`, `auth.go`: organization read-error propagation through the extracted status adapter.
- New regression files: `service_credentials_errors_test.go`, `httpmw/credential_errors_test.go`, `backendapp/auth_credential_errors_test.go`, and `gateway/websocket/access_auth_errors_test.go`.

Verification:

- `TestCredentialStoreFailureReturns503`: passed for session cookie and PAT bearer, including no challenge header, no cookie deletion, and no downstream handler execution.
- `go test -trimpath -tags fts5 ./internal/auth/... ./internal/gateway/websocket ./internal/backendapp -count=1`: passed.
- `go test -trimpath -race -tags fts5 ./internal/auth ./internal/auth/httpmw ./internal/gateway/websocket -run 'Test(Credential|ConcurrentCredential|QueryTokenStore|ProxyTokenLookup)' -count=1`: passed.
- `go test -trimpath -tags fts5 ./cmd/kandev -run '^$' -count=1`: passed.
- `bash scripts/lint-go-changed` for changed Go files: 0 issues.
- The new matrix covers missing/revoked/expired credentials, missing/disabled accounts, missing/suspended organizations, credential/account/organization read errors, cancellation, optional-write failures, cookie/bearer combinations, route compatibility, recovery with the same credential, and interleaved concurrent outcomes.
- Disabled-account coverage keeps the normal revocation scenario and separately retains valid session and PAT rows while disabling the account, proving both resolvers reach the `errAccountInactive` classification instead of stopping at credential-not-found.
- `TestResolveUserIdentityOrganizationOutcomes` covers the non-request identity contract with a usable organization (real identity), an unusable organization (invalid with nil error), and an unreadable organization (empty identity plus `ErrIdentityUnavailable` and the injected cause).

Remaining risk: an unreadable credential on a public or deferred path now returns 503 instead of allowing an anonymous projection. That is the specified contract and is covered by the route matrix.
