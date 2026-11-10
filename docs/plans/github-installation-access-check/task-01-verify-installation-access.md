---
id: "01-verify-installation-access"
title: "Correct installation access and prove lifecycle behavior"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.1
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.16
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.17
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.18
system_design:
  - ../../specs/integrations/system-design/github-authentication-02.md
---

# Task 01: Correct installation access and prove lifecycle behavior

## Summary

Replace the nonexistent singular request with exact membership verification across GitHub's installation list.
Prove the App callback, personal callback, and personal refresh paths through the real OAuth client.
Keep the production caller interfaces and authorization boundaries intact.

## In scope

- `UserCanAccessInstallation` and the existing singular endpoint fixture.
- The client and service tests named in the [plan test matrix](plan.md#tests).
- Provider query parameters, exact numeric IDs, early completion, and error propagation.
- Page body closure, caller cancellation, response size limit, and host-prefix preservation.
- Existing bounded callback retries and atomic connection/token commits.

## Out of scope

- Production changes to caller services, token classification, or persistence.
- Browser UI, public endpoints, secrets, schema, feature flags, and dependencies.
- Automatic recovery of connections invalidated before the repair.
- Live GitHub credentials or App setup.

## Acceptance

1. The real-client regression fails before the correction, then passes for every client case in the plan matrix.
2. Each lifecycle integration case reaches the real list endpoint and asserts the stored connection, generation, tokens, and invalidation outcome.
3. The exact verification commands pass. The implementation changes no caller interface, host selection, or definitive-error classification.

## Implementation sequence

1. Mark this work order `in_progress` and synchronize the plan.
2. Read the requirement, design section, scoped guidance, and existing test helpers.
3. Add `TestOAuthInstallationAccessPagination` first with a server that supports only the documented list endpoint.
4. Run the focused command and record the expected accessible-ID failure against the current client.
5. Complete the client and lifecycle regression matrix before the production correction.
6. Correct the client and existing fixture. Keep page decoding and body closure within the existing Go quality limits.
7. Run every verification command from the repository root.
8. Record actual results in this work order and the plan, then mark the work order `done`.

Return `(false, nil)` without network I/O for non-positive installation IDs.
For each valid ID, request pages from 1 with `per_page=100`.
Return success only for an exact ID match.
Without a match, stop on an empty page or cumulative `total_count` completion.
Every non-200 response returns a typed provider error.
Preserve context, transport, JSON, body limits, and per-page cleanup behavior.

Use `oauthFlowMemoryStore`, `appInstallationMemoryStore`, `personalAuthMemoryRepository`,
`activeAppWorkspace`, and `expiringPersonalAuthRepository` for caller integration fixtures.
Use the real OAuth client for exchange, user lookup, refresh, and installation access.
Use `AppInstallationService.Start` and `Complete` for the installation flow.
Use `PersonalAuthService.Start`, `Complete`, and `ResolvePersonalToken` for personal flows.
Use fixed clocks and valid expiry metadata so tests reach the installation-access boundary.
Set existing installation retry delays to zero in tests and assert the exact call count.

## Verification

Run this block from the repository root. The first command supplies the red phase before implementation.
Repeat it after the correction, then run the remaining commands sequentially.

```bash
(cd apps/backend && go test -trimpath ./internal/github -run '^TestOAuthInstallationAccess' -count=1 -v)
(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test(OAuthFlowGitHubClient|OAuthInstallationAccess|AppInstallationComplete|PersonalAuth(Complete|Refresh|Transient|InvalidGrant))' -count=1)
bash scripts/lint-go-changed apps/backend/internal/github/oauth_flow.go apps/backend/internal/github/oauth_flow_test.go apps/backend/internal/github/oauth_installation_access_test.go apps/backend/internal/github/oauth_installation_access_integration_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/github-installation-access-check
```

### Documentation coverage preflight

Run this command from the repository root.
It checks the actual diff, including untracked files, and the proposed runtime path.
The proposed-path result proves the package links before implementation begins.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  ...execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0'),
].filter(Boolean);
const documents = [
  'docs/plans/github-installation-access-check/plan.md',
  'docs/plans/github-installation-access-check/task-01-verify-installation-access.md',
  'docs/specs/integrations/requirements/github-authentication.md',
  'docs/specs/integrations/system-design/github-authentication-02.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const [label, entries] of [
  ['actual', paths],
  ['proposed', [...paths, 'apps/backend/internal/github/oauth_flow.go']],
]) {
  const changedFiles = [...new Set(entries)].map(filename => ({ filename, status: 'modified' }));
  const result = validateCoverage({ changedFiles, fileContents });
  console.log(JSON.stringify({ label, status: result.status, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

Record the command and its result with the other verification evidence.

## Files likely touched

- `apps/backend/internal/github/oauth_flow.go`
- `apps/backend/internal/github/oauth_flow_test.go`
- `apps/backend/internal/github/oauth_installation_access_test.go` (new)
- `apps/backend/internal/github/oauth_installation_access_integration_test.go` (new)
- `docs/plans/github-installation-access-check/plan.md`
- `docs/plans/github-installation-access-check/task-01-verify-installation-access.md`

## Dependencies

None. This work order owns one complete repair and its regression evidence.

## Risks

- The existing fake OAuth clients bypass HTTP behavior and cannot prove this repair.
- HTTP 403 can represent rate limiting. Preserve the current non-definitive classification.
- A callback must preserve prior automation on any failed verification.
- Never emit token values or provider response bodies in errors or test diagnostics.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-authentication.md), criteria 001.1 and 001.16 through 001.18.
- [Design](../../specs/integrations/system-design/github-authentication-02.md#user-access-to-an-app-installation).
- [Plan](plan.md), including the exact test matrix and reproduction evidence.
- [ADR 0047](../../decisions/0047-github-authentication-ownership.md).
- `apps/backend/AGENTS.md` and `apps/backend/internal/github/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and its `references/backend-tests.md`.
- Existing `oauth_flow_test.go`, `app_installation_service_test.go`, and `personal_auth_service_test.go` fixtures.

## Results

Implementation on 2026-10-28:

- RED: the focused client command failed before the correction. Every pagination
  case requested `/user/installations/{id}`; unrelated and empty inventories
  could not terminate through `total_count`; a non-positive ID was accepted; a
  404 or unexpected 2xx was accepted; invalid JSON returned success; an error on
  a later page was never requested; and response bodies were not decoded.
- GREEN: `(cd apps/backend && go test -trimpath ./internal/github -run '^TestOAuthInstallationAccess' -count=1 -v)`
  passed all client cases. `UserCanAccessInstallation` now pages
  `GET /user/installations?per_page=100&page=N`, returns success only for an
  exact ID, stops on an empty page or cumulative `total_count`, returns
  `(false, nil)` without I/O for non-positive IDs, and returns typed
  `GitHubAPIError` for every non-200 response.
- Caller integration passed:
  `TestAppInstallationCompleteWithPaginatedUserAccess`,
  `TestPersonalAuthCompleteWithPaginatedUserAccess`, and
  `TestPersonalAuthRefreshWithPaginatedUserAccess`. App installation commits
  after a later-page match and retries confirmed absence. A provider error does
  not retry, commit, or replace the previous connection. Personal complete and
  refresh reach the real list endpoint. Refresh rotates tokens atomically for a
  later-page match; confirmed absence and HTTP 401 invalidate the connection;
  HTTP 403, 404, 429, 500, and invalid JSON preserve stored status and tokens
  while failing the request.
- `(cd apps/backend && go test -trimpath -race ./internal/github -run 'Test(OAuthFlowGitHubClient|OAuthInstallationAccess|AppInstallationComplete|PersonalAuth(Complete|Refresh|Transient|InvalidGrant))' -count=1)`
  passed, as did the full `-race` package suite and `go build ./...`.
- `bash scripts/lint-go-changed` on the four changed Go files reported `0 issues`.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`,
  and `python3 scripts/lint-spec-files.py --all` passed.
- The documentation coverage preflight reported `actual` and `proposed` both
  `covered` with no errors. `git diff --check` passed.
- The implementation and plan package remain unstaged and uncommitted.
