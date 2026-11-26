---
created: 2026-11-24
status: done
requirements:
  - REQ-AUTH-AUTH-001
system_design:
  - ../../specs/auth/system-design/credential-lookup-failures.md
legacy_specs: []
---

# Office API personal access token fix

## Overview

Scripts can use a verified personal access token (PAT) on user-facing Office routes under the same workspace authorization as a session cookie.
The existing auth requirement covers this path. [Issue #5490](https://github.com/kdlbs/kandev/issues/5490), [REQ-AUTH-AUTH-001](../../specs/auth/requirements/auth.md)

## Root cause and reproduction

`httpmw.Middleware` verifies the PAT and attaches its user identity to the request.
`AgentAuthMiddleware` attempts agent JWT validation on the same bearer and returns `401 {"error":"invalid token"}` before the Office workspace guard runs.

On `8aab9447b35f`, a temporary Go test mounted global auth and the production Office route group with SQLite test data.
The same minted PAT received 200 on a protected control route and 401 on `GET /api/v1/office/workspaces/:wsId/projects` for its own workspace.
A session cookie received 200 on the Office project list.

## Scope and technical approach

In `apps/backend/internal/office/agents/handler.go`, classify PAT bearers with `auth.IsPATFormat` after trimming surrounding whitespace for that check.
Require an identity from `authn.FromGin` for that branch.
When authentication is enabled, pass the verified user identity to Office workspace authorization and return the existing 401 response if no identity exists. When authentication is disabled, global auth supplies a synthetic identity, so treat the PAT-shaped bearer as if no bearer was sent.

The change belongs at the Office boundary because global auth already resolves the cookie and PAT and handles storage errors.
Continue to pass non-PAT bearers to the agent JWT validator with the existing token extraction.
The runtime handlers authenticate Office agents with their own JWT check.
When auth is disabled, credentialless Office requests and PAT-shaped bearers pass through with the synthetic identity.
Add a short clarification to the public auth reference during implementation. [Office bearer authentication design](../../specs/auth/system-design/credential-lookup-failures.md#office-bearer-authentication)

## Tests

The HTTP regression uses the production Office mount, the workspace scope guard, and a real project handler.
This exercises the API from the Authorization header to the project response.

| Criteria | Test and expected result |
| --- | --- |
| `AC-AUTH-AUTH-001.4`, `.7` | `TestOfficePATAuthentication` returns the seeded project for an owning PAT and 404 for a workspace outside the caller’s scope. It also covers the global bearer whitespace rules. |
| `AC-AUTH-AUTH-001.12`, `.13` | The same test returns 401 for an unrecognized PAT on Office metadata and 200 when a valid session cookie accompanies that bearer on the project list. |
| `AC-AUTH-AUTH-001.16` | A user PAT receives 401 from the runtime JWT check. `TestOfficePATAuthenticationDisabledMode` covers credentialless access and confirms a PAT-shaped bearer has the same result in disabled mode. The existing Office JWT tests cover agent authorization. |
| `AC-AUTH-AUTH-001.11`, `.13`, `.16` | The existing `internal/auth/httpmw` tests cover cookie and PAT lookup errors, fallback, and downstream deferral. |

## Work orders

- [x] [Task 01 Accept verified user credentials in Office](task-01-accept-office-pats.md)

## Verification results

The temporary reproduction failed at the Office project list with the reported 401.
The candidate passed the production-route tests using a Go overlay. The owning PAT received 200, the foreign workspace received 404, and the unrecognized PAT received 401.
Cookie precedence, runtime JWT enforcement, disabled mode, and the selected agent JWT and credential-storage tests passed with the candidate.

The specification catalog, specification lint, and PR documentation coverage preflight passed.
The coverage preflight used `internal/office/agents/handler.go` as the proposed code change.

The permanent regression reproduced the 401 before the middleware change and passed afterward. It covers owning and whitespace-padded PATs, foreign-workspace denial, invalid PAT rejection, session-cookie precedence, runtime JWT enforcement, and disabled-mode behavior. During PR fixup, the disabled-mode regression first failed with 401, then passed after the middleware treated the PAT-shaped bearer like no bearer.
The focused Go tests passed in `internal/backendapp`, `internal/office/agents`, and `internal/auth/httpmw`.
The full backend build, changed-file Go lint, public documentation tests and validation, and `git diff --check` passed.

## Risk

The global layer can defer an unresolved bearer to Office.
The PAT prefix classifies the credential. With authentication enabled, Office requires an authenticated identity even on routes without a workspace parameter. With authentication disabled, the synthetic identity makes a PAT-shaped bearer behave like no bearer.
