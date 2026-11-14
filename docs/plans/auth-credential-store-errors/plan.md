---
created: 2026-11-02
status: done
requirements:
  - REQ-AUTH-AUTH-001
system_design:
  - ../../specs/auth/system-design/credential-lookup-failures.md
legacy_specs: []
---

# Implementation Plan: Authentication credential-store errors

## Overview

Fix [issue #5001](https://github.com/kdlbs/kandev/issues/5001) by preserving unavailable verification results through request authentication.
The service, middleware, organization wiring, gateway handling, browser continuity coverage, and public error contract are delivered.
Both work orders are complete and were executed sequentially in the primary conversation.

## Evidence and confirmed root cause

Investigation used the canonical issue body and current repository source.
The issue has no comments or image attachments as of 2026-11-02.
The authenticated GitHub account is `carlosflorencio`; assignment to that account was applied and verified.

- `auth.Service.ResolveSessionToken` and `ResolveBearer` return `false` for every credential-store error.
- They also discard errors from `activeUser`; `orgUsable` uses a boolean callback that discards organization-store errors during backend wiring.
- `httpmw.ResolveRequest` cannot carry an error, so `Middleware` sends a 401 with `WWW-Authenticate: Bearer`.
- `websocket.AuthPolicy.ResolveToken` has the same boolean limitation for `?token=PAT` clients.
- The shared browser API client reacts to a challenged 401 by calling its unauthorized handler.

A temporary `TestIssue5001StoreFailureIsNotUnauthorized` exercised the real middleware, service, and SQL stores.
It created one valid session and PAT, proved both returned 200, closed the backing database, and repeated the requests.
Both returned 401, `WWW-Authenticate: Bearer`, and `{"error":"authentication required"}` instead of the expected 503.
The diagnostic command exited 1 with two expected failing subtests:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/auth/httpmw -run '^TestIssue5001StoreFailureIsNotUnauthorized$' -count=1 -v)
```

The temporary test was removed after evidence collection.
There are no production or permanent test changes at this checkpoint.
This proves the classification defect independently of database driver.
It does not reproduce the reporter's Postgres connection-limit incident or establish its exact database error.

## Requirement conformance and assumptions

Auth owns this repair because it owns credential verification and session challenges.
The existing session/revocation contract remains authoritative: `AC-AUTH-AUTH-001.1`, `.3`, and `.4`.
The active requirement lacked explicit storage-outage behavior.
New criteria `.11` through `.17` extend the same requirement instead of creating a repair-only specification.
The new [system design](../../specs/auth/system-design/credential-lookup-failures.md) defines the error path.

The assumption check found no material product question that blocks this package.
The issue explicitly distinguishes unavailable storage from invalid credentials.
A successful independent fallback remains sufficient to authenticate.
The implementation must not use outages to admit unverified identities or remove genuine invalid-credential challenges.

## Scope

### In scope

- Required credential, account, and organization read-error classification.
- Tri-state propagation through HTTP and gateway query-token authentication.
- Candidate precedence, failure diagnostics, and optional-write compatibility.
- Deterministic regression tests, including mixed success/error requests and unchanged-credential recovery.
- Client tests and desktop/phone browser continuity evidence.
- Public reference text in `docs/public/authentication.md` during implementation.

### Out of scope

- Postgres pool sizing, idle retention, configuration defaults, and performance tuning.
- Reproducing infrastructure-specific connection limits without the reporter's diagnostic evidence.
- Password-login error classification, new retry loops, credential caching, and schema changes.
- Capability-only proxy account checks, provider credentials, and new UI or localization strings.

## Technical approach

Follow the [system design](../../specs/auth/system-design/credential-lookup-failures.md) for exact contracts and ordering.
Change resolver and organization-checker signatures, then migrate every caller in the same compiling work order.
Preserve `store.ErrNotFound`, `sql.ErrNoRows`, disabled-account, and missing/suspended-organization outcomes as invalid.
All other required lookup errors produce an unavailable result with no identity.
Use the service logger for bounded diagnostics without raw error text.

HTTP error handling precedes allowlist/deferred handling after an unsuccessful credential resolution.
This prevents anonymous boot projections and downstream 401 rewrites.
Public requests without credentials retain their allowlist behavior.
Public requests carrying an unreadable session/PAT now receive the same 503 contract as protected requests.
The gateway must also handle errors in its capability branch's PAT-stripping lookup.

### Compatibility matrix

| Credential and transport | Required outcome | Evidence |
| --- | --- | --- |
| Session cookie, HTTP or gateway upgrade | Valid identity, definitive rejection, or 503 | Middleware and resolver tests |
| PAT bearer, HTTP and external MCP | Same classification without downstream 401 rewrites | Middleware route-matrix tests |
| PAT query, WebSocket and port proxy | 503 before upgrade/forward on lookup failure | Gateway tests |
| Valid proxy capability plus PAT query | Lookup failure stops forwarding; successful PAT is stripped | Proxy regression tests |
| Non-PAT bearer, independent downstream validator | Existing deferral when no session lookup failed | Middleware compatibility tests |
| Auth disabled or public route without credentials | Existing behavior with no required credential lookup | Middleware compatibility tests |

SQLite and Postgres share these auth stores and result contracts.
Deterministic injected failures prove classification without requiring a live Postgres service.
No Postgres latency or pool-churn claim is part of acceptance.

## Tests

Each name below is a planned permanent regression, not a claim that the test already exists.

- `internal/auth/service_credentials_errors_test.go`: `TestCredentialLookupFailures` covers required reads, missing rows, expiry, disabled accounts, and organization outcomes (`.11`, `.12`).
- The same file: `TestCredentialOptionalWritesDoNotReject` covers touch and expired-session cleanup failures (`.14`).
- The same file: `TestCredentialLookupDiagnostics` checks bounded fields, one event per failed read, nil logger safety, and sensitive-value exclusion (`.15`).
- `internal/auth/httpmw/credential_errors_test.go`: `TestCredentialStoreFailureReturns503` is the RED gate (`.11`).
- The same file: `TestCredentialFallbackKeepsLookupErrors` covers error+invalid and error+valid candidate combinations (`.13`).
- The same file: `TestCredentialFailureRouteCompatibility` covers protected routes, auth/me, app-state, MCP/deferred routes, public routes, and disabled mode (`.12`, `.16`).
- The same file: `TestCredentialStoreRecoveryWithSameCredential` and `TestConcurrentCredentialLookupOutcomes` cover recovery and interleaved successful/failed requests (`.11`).
- `internal/gateway/websocket/access_auth_errors_test.go`: `TestQueryTokenStoreFailureBeforeUpgrade` and `TestProxyTokenLookupFailureDoesNotForward` cover query-token and stripping paths (`.11`, `.12`, `.16`).
- `internal/backendapp/auth_credential_errors_test.go`: `TestOrganizationCredentialLookupWiring` covers the real missing/inactive/error adapter distinction (`.11`, `.12`).
- `apps/web/lib/api/client.test.ts`: add 503 and recovery cases for all three fetch helpers, with challenged-401 positive controls (`.17`).

Keep new backend cases in separate files because nearby test files are already large.
Use controllable read failures, not timing-dependent Postgres saturation.
Each concurrent request must carry its own fault context or isolated reader dependency.
Do not race a shared mock failure toggle or close a shared database underneath unrelated tests.

## E2E tests

`apps/web/e2e/tests/auth/credential-store-errors.spec.ts` runs in project `auth`.
Its phone companion `mobile-credential-store-errors.spec.ts` runs in `mobile-chrome` and shares a sibling helper.
Use the existing auth-enabled backend fixture and a disposable database, restoring baseline startup state afterward.

Start on the existing account surface with a valid session.
Intercept a UI-triggered account-data read with the exact 503 response and no challenge header.
Observe that request and response, then assert the same route, signed-in identity, and session cookie.
Remove interception, repeat the UI action, and prove successful data loading without another login.
Keep a challenged-401 control that still reaches `/login`.
These scenarios map to `.17`; Go integration tests remain the backend-causality proof for `.11`.
No rendered composition changes or ASCII UI preview are required.

## Work orders

- [x] [Task 01: Preserve credential lookup errors end to end](task-01-preserve-credential-errors.md)
- [x] [Task 02: Prove browser continuity and document 503](task-02-prove-browser-continuity.md)

Task 02 depends on Task 01. Both were sequential.
Exact commands and owned files appear in each work order.

## Verification results

- Investigation: two expected RED subtests confirmed the defect, as recorded above.
- Temporary reproduction: removed.
- Task 01 focused regression: `TestCredentialStoreFailureReturns503` passed for session and PAT after the implementation.
- Task 01 backend suites: `go test -trimpath -tags fts5 ./internal/auth/... ./internal/gateway/websocket ./internal/backendapp -count=1` passed.
- Task 01 race matrix: `go test -trimpath -race -tags fts5 ./internal/auth ./internal/auth/httpmw ./internal/gateway/websocket -run 'Test(Credential|ConcurrentCredential|QueryTokenStore|ProxyTokenLookup)' -count=1` passed.
- Task 01 command compile: `go test -trimpath -tags fts5 ./cmd/kandev -run '^$' -count=1` passed.
- Task 01 changed-file lint: `bash scripts/lint-go-changed` reported 0 issues.
- Review remediation: `go test -trimpath -tags fts5 ./internal/auth -run 'Test(CredentialLookupFailures|ResolveUserIdentity)' -count=1` passed, including disabled accounts with retained credentials and all three `ResolveUserIdentity` organization outcomes.
- PR fixup: centralized the 503 response in `authn`, tightened diagnostic tuple assertions and invalid-credential error checks, clarified the public fallback wording, and added causal waits for both browser responses. The affected Go suites, Vitest, and both browser scenarios passed again.
- Task 02 client tests: `pnpm exec vitest run lib/api/client.test.ts` passed 17 tests.
- Task 02 changed-file lint: `pnpm exec eslint` reported 0 errors and 0 warnings.
- Task 02 browser continuity: the managed `auth` desktop spec and `mobile-chrome` phone spec each passed their scenario.
- Public docs: `node --test scripts/validate-public-docs.test.mjs` passed 63 tests, and `node scripts/validate-public-docs.mjs` validated 54 pages.
- Package validation: `python3 scripts/list-docs.py validate` validated 425 decisions and 1886 specifications; both spec lint commands passed.
- PR-documentation coverage preflight: passed with status `covered`, both work orders accepted, and no errors.
- `git diff --check` passed.
- All production, test, browser, and documentation changes are delivered; both work orders are complete.

## Risks

- Missing-row and disabled-account errors must not be mistaken for storage outages.
- A forgotten boolean caller or organization adapter can silently restore 401 misclassification.
- Public requests carrying unreadable credentials now receive 503, including boot/auth-state reads.
- Query-token handling must preserve the proxy's token-stripping boundary.
- Raw database errors can contain secrets. Diagnostic fields must remain bounded and sanitized.
- The package corrects failure classification. It does not guarantee elimination of Postgres connection failures.

## Design-package validation

Run from the repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/auth docs/plans/auth-credential-store-errors
git status --short -- docs/specs/auth docs/plans/auth-credential-store-errors
```

Use `.github/scripts/pr-docs.cjs` `validateCoverage` with the linked requirement, design, plan, and both work orders.
Include `apps/backend/internal/auth/service_credentials.go` as a synthetic triggering path during design preflight.
That exercises the complete coverage chain without requiring production edits or staging the package.
The exact design preflight is:

```bash
node <<'NODE'
const fs = require('node:fs');
const assert = require('node:assert/strict');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/specs/auth/requirements/auth.md',
  'docs/specs/auth/system-design/credential-lookup-failures.md',
  'docs/plans/auth-credential-store-errors/plan.md',
  'docs/plans/auth-credential-store-errors/task-01-preserve-credential-errors.md',
  'docs/plans/auth-credential-store-errors/task-02-prove-browser-continuity.md',
];
const result = validateCoverage({
  changedFiles: [
    ...paths.map(filename => ({ filename, status: 'modified' })),
    { filename: 'apps/backend/internal/auth/service_credentials.go', status: 'modified' },
  ],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({
  ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors,
}, null, 2));
assert.equal(result.ok, true);
assert.equal(result.workOrders.length, 2);
NODE
```
