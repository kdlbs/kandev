---
status: current
system: auth
requirements:
  - REQ-AUTH-AUTH-001
---

# Credential lookup failures

## Purpose and ownership

Auth owns the distinction between an invalid credential and an unavailable verification dependency.
This design extends [auth requirements](../requirements/auth.md) without changing credential formats or authorization scope.
It covers request authentication, including gateway PAT query parameters.
It does not change Postgres pool policy, password verification, or capability-only proxy authentication.

The implementation before this design collapsed storage errors into `false`.
[Issue #5001](https://github.com/kdlbs/kandev/issues/5001) reports the resulting challenges under concurrent Postgres load.
The design below is implemented and covered by the linked fix package.

## Requirement mapping

- `AC-AUTH-AUTH-001.11` and `.12`: resolver results and transport mapping.
- `AC-AUTH-AUTH-001.13`: candidate ordering and error retention.
- `AC-AUTH-AUTH-001.14`: required reads and optional writes.
- `AC-AUTH-AUTH-001.15`: operator diagnostics.
- `AC-AUTH-AUTH-001.16`: middleware ordering and compatibility.
- `AC-AUTH-AUTH-001.17`: browser continuity and validation.

## Resolver results

Change `Service.ResolveSessionToken` and `Service.ResolveBearer` in `internal/auth/service_credentials.go` to return `(authn.Identity, bool, error)`.
Only success returns a real identity and `true`, with a nil error.
Known invalid credentials return an empty identity, `false`, and a nil error.
Required lookup failures return an empty identity, `false`, and an error.
Introduce `ErrCredentialLookupUnavailable` and preserve the underlying error for `errors.Is` checks.
No caller can use a partial identity from the error result.

Classify `store.ErrNotFound` from credential reads as invalid.
Classify `sql.ErrNoRows` from account reads as invalid.
Keep disabled-account detection separate from arbitrary errors returned by `activeUser`.
Use a private inactive-account sentinel or an equivalent explicit status result.
Do not classify errors by message text.
All other required read errors, including deadline and cancellation errors, remain unavailable.
If the connection can still receive a response, the transport returns 503.

Change `OrgStatusChecker` to return `(bool, error)`.
The callback wired in `internal/backendapp/main.go` maps `org.ErrOrgNotFound` and inactive organizations to `false, nil`.
It propagates other `services.Org.Get` errors.
Change `orgUsable` accordingly, retaining success for a nil checker or an empty organization ID.
Update every caller to compile and retain its fail-closed behavior.
`IdentityForUser` retains its boolean compatibility contract.
`ResolveUserIdentity` retains its existing `ErrIdentityUnavailable` category for undecidable lookups.
This change does not introduce a new error-response contract for password login or other account operations.

## Required reads and optional writes

The required checks are the credential read, expiry check, account read/status, and organization read/status.
Do not accept a credential until all required checks succeed.
`TouchSession`, `TouchAPIToken`, and cleanup of an already expired session remain best effort.
Their errors do not become credential rejection or lookup-unavailable results.
An organization read failure still prevents authentication even if an earlier touch succeeded.
No retry, credential cache, stale identity, schema change, or migration is required.

## HTTP candidate ordering

Change `httpmw.ResolveRequest` to carry the third result.
Try the request-scoped cookie first, then the Authorization bearer if needed.
A valid cookie ends resolution without reading the bearer.
A valid bearer wins over an earlier invalid or unavailable cookie.
Remember a cookie lookup error while attempting the bearer.
If no candidate succeeds, return any remembered required-read error.
Return ordinary invalid only when every attempted candidate is definitively invalid.

`httpmw.Middleware` retains the disabled-mode branch before credential resolution.
After unsuccessful resolution, handle lookup errors before the public and deferred path branches.
Return HTTP 503 with this stable JSON response:

```json
{"error":"authentication temporarily unavailable","code":"persistence_unavailable"}
```

Do not add `WWW-Authenticate`, clear cookies, revoke records, call downstream handlers, or inject a synthetic identity.
Keep the existing 401 and Bearer challenge for definitive failures on protected routes.
Credentialless public/deferred routes and non-PAT bearer deferral retain their current behavior.

This ordering also protects `/api/v1/auth/me` and `/api/v1/app-state` from projecting an outage as an anonymous user.
It prevents `/mcp` and gateway handlers from replacing an already observed storage error with a 401.
A public or self-authenticating request that supplies an unreadable session/PAT also receives 503.
Credentialless health/readiness probes remain available.
This conditional change must be explicit in public reference documentation and middleware tests.

## Gateway query tokens

Change `websocket.AuthPolicy.ResolveToken` in `internal/gateway/websocket/access.go` to return the same three results.
Update `backendapp.gatewayAuthPolicy`, all production callers, fixtures, and function-valued test stubs.
`requireConnectionAuth` returns 503 for an unavailable query-token lookup before upgrade or forwarding.
Definitively invalid query tokens retain the gateway's existing 401 response.

The capability-authenticated branch also calls `ResolveToken` to decide whether to strip a query token.
Handle its error explicitly: return 503 rather than forward a possibly valid gateway credential to the preview application.
Keep successful token stripping and unrelated application-token preservation unchanged.
The existing boolean `AuthPolicy.ActiveUser` gate for capability-only requests remains outside this contract.

## Operator diagnostics

Log at the auth resolver boundary, once per failed required lookup.
Use the service's existing logger, with a nil-safe path for test fixtures.
Use a fixed event such as `auth.credential_lookup_failed` and bounded fields for credential kind, lookup operation, and error category.
Categories distinguish cancellation, deadline, and other store failures.
Do not log `err.Error()`, SQL text, request URLs, request headers, raw tokens, hashes, or DSNs.
The wrapped cause remains an internal return value, never a transport response or generic diagnostic field.
No new metrics or high-cardinality labels are needed.

## Browser continuity and validation

`apps/web/lib/api/client.ts` already calls `onUnauthorized` only for a challenged 401.
A 503 remains an `ApiError` for the calling surface.
Add coverage for `fetchJson`, `fetchBlob`, and `fetchConditionalJson` without changing production UI.
Cover recovery on a later successful request and preserve the challenged-401 positive control.

Use real HTTP middleware integration tests for fault classification, downstream non-execution, and recovery with unchanged credentials.
Use focused gateway tests for pre-upgrade query-token errors and proxy token handling.
Use desktop and phone browser tests to prove continuity in the existing account surface.
Browser interception supplies the exact backend response and proves client behavior only.
The Go tests prove that real lookup failures produce that response.
No new layout, navigation control, copy, or viewport composition is required.

## Related records

- [Opt-in authentication decision](../../../decisions/2026-07-24-opt-in-authentication.md).
- [Integration challenge classification](../../../plans/integration-provider-auth-errors/plan.md), a completed companion record that remains unchanged.
- [Fix package](../../../plans/auth-credential-store-errors/plan.md).

The design and regression matrix preserve the rationale for this local error-classification repair.
No separate ADR is needed because the existing fail-closed trust boundary remains intact.
