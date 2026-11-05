---
created: 2026-10-28
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
legacy_specs: []
---

# Implementation Plan: GitHub installation access verification

## Overview

Correct the provider request behind [issue #4867](https://github.com/kdlbs/kandev/issues/4867).
One work order added the regression, corrected pagination, and proved callback and refresh behavior.
Implementation and local verification are complete.

The integration system owns this repair because it owns GitHub credentials and verified provider identity.
The repair restores the association boundary in [ADR 0047](../../decisions/0047-github-authentication-ownership.md).
It retains [registration isolation](../../decisions/2026-07-21-workspace-selectable-github-app-registrations.md).

## Evidence and root cause

Investigation used commit `67c25d4a136f2e80a50f8b3753ff7bb2471da6bb`.
`GitHubOAuthClient.UserCanAccessInstallation` requests `/user/installations/{id}`.
It interprets HTTP 404 as `(false, nil)` and accepts other successful responses without inspecting an installation ID.
GitHub documents [the paginated list endpoint](https://docs.github.com/en/rest/apps/installations#list-app-installations-accessible-to-the-user-access-token).
The existing `TestOAuthFlowGitHubClientExchangesRefreshesAndVerifiesUser` fixture accepts the nonexistent singular endpoint.

A temporary Go overlay added `TestIssue4867DocumentedInstallationEndpoint` without changing repository tests.
Its HTTP server implements only `/user/installations` and returns installation 42.
The current client requests `/user/installations/42` and returns `(false, nil)`.
The expected `(true, nil)` assertion fails.

The server's list response is `{"total_count":1,"installations":[{"id":42}]}`.
Unknown paths return HTTP 404. The captured trace is:

```text
=== RUN   TestIssue4867DocumentedInstallationEndpoint
requested /user/installations/42
accessible installation 42: got (false, <nil>), want (true, nil)
--- FAIL: TestIssue4867DocumentedInstallationEndpoint
```

The false result rejects App installation and personal authorization callbacks.
During personal refresh, it also triggers `markInvalid` for a valid connection.
Current refresh classification invalidates on HTTP 401, rather than every HTTP 403 described in the issue.
The repair preserves this source behavior, including rate-limit safety.

The implementation violates criterion 001.1's verified-installation contract.
Criteria 001.16 through 001.18 supply missing acceptance detail within that existing requirement.
The design now names the documented provider protocol. No new credential boundary or ADR is required.

## Assumption check

- Confirmed: investigate the issue, assign a sufficiently understood repair, and create the fix package.
- Verified: the official endpoint, incorrect current request, false-negative result, and caller effects.
- Verified: existing state, registration, user, and generation checks must remain intact.
- Scope: restore verified installation access without changing product interactions or credential ownership.
- No material decision remains unresolved.

## Scope

### In scope

- Exact installation membership across the documented list endpoint's pages.
- Typed HTTP errors and propagation of transport, cancellation, and decoding errors.
- Callback acceptance and refresh behavior through the real OAuth client.
- Existing callback retries and personal connection invalidation rules.
- Criteria `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.1` and `001.16` through `001.18`.

### Out of scope

- Frontend markup, routes, copy, localization, and browser interaction changes.
- App registration, token grants, repository permissions, webhooks, and storage schema changes.
- Expanding the definitive-error classifier to HTTP 403.
- Automatically repairing connections invalidated before this fix.
- New retries, caches, pagination libraries, feature flags, or persistent Kandev tasks.

## Technical approach

Change `UserCanAccessInstallation` in `apps/backend/internal/github/oauth_flow.go`.
Keep the `GitHubAppOAuth` interface and all production callers unchanged.
Use the existing `apiRequest`, `doGitHubRequest`, and `maxGitHubAppResponseSize` behavior.
Keep each page's body lifetime local, using a small page helper if required.
Stop immediately on an exact ID match.
Otherwise stop on an empty page or after the fetched row count reaches `total_count`.
Reject non-200 responses as `GitHubAPIError`, including a list-endpoint 404.

Correct the singular fixture in `oauth_flow_test.go`.
Add focused client tests in `oauth_installation_access_test.go`.
Add caller integration tests in `oauth_installation_access_integration_test.go`.
Use the real `GitHubOAuthClient`, an `httptest` GitHub server, and existing in-memory connection repositories.
Keep App JWT installation verification behind `fakeAppInstallationVerifier`.

| Credential path | Transport and identity | Result and evidence | Unavailable or unsupported response |
| --- | --- | --- | --- |
| App installation callback | Registration-bound user token and existing REST base | Exact ID on any page permits existing verification and commit | Error or exhausted absence rejects callback and preserves previous automation |
| Personal OAuth callback | User token for the workspace App | Exact ID permits existing atomic connection save | Error or absence rejects callback without replacement |
| Personal refresh | Refreshed user token for the same identity and installation | Exact ID permits atomic token rotation | Absence or 401 invalidates. Other errors preserve stored state and fail the request |
| PAT, named CLI, installation tokens | Existing independent credential paths | No behavior change | No fallback to these tokens for user association |
| GitHub.com, GHE.com, GHES | Existing validated host-specific REST base and OAuth policy | Relative list path preserves configured origin and REST prefix | Provider incompatibility remains an error. No alternate host probe |

## Tests

| Criteria | Test and evidence |
| --- | --- |
| 001.1, 001.16 | `oauth_installation_access_test.go`: `TestOAuthInstallationAccessPagination` covers first-page and later-page matches, unrelated IDs, empty inventory, and total-count exhaustion |
| 001.16 | `TestOAuthInstallationAccessRequestContract` asserts GET, path, page numbers, `per_page=100`, bearer header, and existing API headers. A host-prefix case retains `/api/v3` |
| 001.16 | `TestOAuthInstallationAccessInvalidID` covers zero and negative IDs with no requests |
| 001.17 | `TestOAuthInstallationAccessErrors` covers 401, 403, 404, 429, 500, unexpected 2xx, invalid JSON, transport error, and cancellation. Include errors on a later page |
| 001.17 | `TestOAuthInstallationAccessClosesPageBodies` proves each body closes before the next page and on errors |
| 001.16, 001.17 | `oauth_installation_access_integration_test.go`: `TestAppInstallationCompleteWithPaginatedUserAccess` proves successful commit, absence then success on retry, and unchanged prior connection on rejection |
| 001.16, 001.17 | `TestPersonalAuthCompleteWithPaginatedUserAccess` proves later-page success and no replacement on absence or provider error |
| 001.16, 001.18 | `TestPersonalAuthRefreshWithPaginatedUserAccess` proves later-page success and atomic generation/token rotation. Absence and 401 invalidate. 403, 404, 429, 500, and invalid JSON preserve stored tokens and active status while returning an error |

Use mixed inventories containing both the requested ID and unrelated IDs.
At least one matching row establishes access. Requiring every row to match is incorrect.
Assert request counts for early success, total-count completion, and empty-page termination.
Keep HTTP handlers free of `t.Fatal` calls. Use deterministic retry delays and fixed token clocks.

## E2E tests

The service integration tests exercise each affected entry point through the real HTTP client and persistence boundary.
They provide deterministic end-to-end evidence for criteria 001.16 through 001.18 without live GitHub credentials.
No Playwright project is required because browser rendering and interaction remain unchanged.
The existing browser mock implements `mockAppAuthorization` and cannot detect this REST request defect.

## Work orders

- [x] [Task 01: Correct installation access and prove lifecycle behavior](task-01-verify-installation-access.md)

Run the single work order sequentially with TDD. No delegation is authorized.

## Verification results

Investigation on 2026-10-28:

- Temporary overlay regression: expected failure. Requested `/user/installations/42`, received `(false, nil)` for an accessible ID.
- Existing focused baseline passed: `(cd apps/backend && go test -trimpath ./internal/github -run 'Test(OAuthFlowGitHubClient|AppInstallationComplete|PersonalAuth(Complete|Refresh|Transient|InvalidGrant))' -count=1)`.
- `python3 scripts/list-docs.py validate`: passed, with 411 decisions and 1773 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, with 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Task 01's documentation coverage preflight: actual document diff `exempt`, proposed runtime path `covered`, and no errors.
- `git diff --check`: passed. Package status includes both untracked plan files.
- GitHub readback: issue #4867 remains open and is assigned to `carlosflorencio`.

Implementation on 2026-10-28:

- RED: `(cd apps/backend && go test -trimpath ./internal/github -run '^TestOAuthInstallationAccess' -count=1 -v)` failed against the singular client. Later-page and mixed-inventory cases requested `/user/installations/42`; an empty or unrelated inventory could not terminate. The list contract, non-positive IDs, unexpected 2xx, decoding, later-page errors, and body closure all failed as expected.
- GREEN: the same focused command passed every client case after the correction.
- Focused caller regression command passed: `(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test(OAuthFlowGitHubClient|OAuthInstallationAccess|AppInstallationComplete|PersonalAuth(Complete|Refresh|Transient|InvalidGrant))' -count=1)`.
- Full `(cd apps/backend && go test -trimpath -race ./internal/github -count=1)` passed, and `(cd apps/backend && go build ./...)` passed.
- `bash scripts/lint-go-changed` on all four changed Go files reported `0 issues`.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, and `python3 scripts/lint-spec-files.py --all` passed again.
- The documentation coverage preflight reported `actual` and `proposed` both `covered`, with no errors.
- `git diff --check` passed. The implementation and plan package remain unstaged and uncommitted.

## Risks

- Real GitHub installation propagation remains asynchronous. Preserve the existing bounded retry schedule.
- Refresh errors can follow remote token rotation. This repair preserves current storage behavior and does not redesign refresh compensation.
- An already invalidated personal connection requires the existing reconnect flow.
- No live callback was attempted. The reproduced defect and planned regressions use the documented HTTP contract.
- The original [authentication package](../github-authentication/plan.md) records historical results. This package owns the repair and its evidence.
- Public docs remain unchanged. The existing integration guide already describes the intended verified App and personal connection flows.
