---
created: 2026-09-18
status: implemented
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001
system_design:
  - ../../specs/integrations/system-design/sentry-watcher-lookback-periods.md
legacy_specs: []
---

# Implementation Plan: Sentry Watcher Lookback Periods

## Overview

Send the selected Sentry lookback only to the issue endpoint whose accepted values include it. The project-scoped endpoint currently rejects every offered token except `24h` and `14d`, which is the reported watch failure; the organization-scoped endpoint accepts all of them and uses the value as its request time range, so it keeps receiving it. Order: prove the per-endpoint request shape, gate the parameter, guard the write and poll paths, correct the comments that assert the old mechanism, then confirm the user-visible save and reload path.

## Scope

### In scope

- Keep the existing `1h`, `24h`, `7d`, `14d`, and `30d` choices unchanged.
- Stop sending `statsPeriod` on the project-scoped issue request; keep sending it on the organization-scoped request when a lookback is selected.
- Preserve the existing `age:-<period>` eligibility constraint on both.
- Reject a non-empty lookback value outside Kandev's supported lookback syntax, on watch create and on a filter-carrying update; keep an empty value accepted.
- Fail a poll closed when a stored lookback value is outside that syntax, so such a row creates no tasks.
- Reject an invalid non-empty lookback on direct issue-browse requests before the provider client runs.
- Correct the request/comment claims that the parameter never affects which issues are returned.
- Add REST-client, service, and settings-E2E coverage.

### Out of scope

- Changing watch polling cadence, deduplication, or task dispatch.

## Technical approach

`RESTClient.searchIssues` sets `statsPeriod` unconditionally today behind its existing non-empty check, while `issuesSearchPath` already selects the endpoint from `projectSlugForRequest(filter)`. Gate the parameter on that same predicate while keeping the non-empty condition, so the value travels only to the endpoint that accepts it and an empty lookback sends nothing, and comment the pair as one decision.

Add `TestRESTClient_SearchIssues_ForwardsLookbackOnlyWhereAccepted` to `apps/backend/internal/sentry/rest_client_test.go` using the existing `newMockServer`/`pointTo` harness: for every offered token, assert a project-scoped request carries `age:-<token>` with no `statsPeriod`, and an org-scoped request carries both; also assert that an empty lookback carries neither, on either path. Run it red before the change, then green.

Add the fail-closed guard in `internal/sentry/service_issue_watch.go`: reject `StatsPeriod` only when it is non-empty and `parseStatsPeriodUnits` cannot express it, on create and on the `req.Filter != nil` branch of update, mirroring `validateFilterStatuses` so an unrelated partial update never re-validates a stored value. The deleted project-scoped parameter used to be the only rejection of an inexpressible token, so `CheckIssueWatch` also needs the matching poll-time guard: a row that already stores such a value must stamp the watch error and return without searching, mirroring its existing project-less guard, or the fix would let that row dispatch tasks for issues of any age.

Apply the same `validateFilterStatsPeriod` guard in `Service.SearchIssues` for the direct issue browser. Trim the value first, reject invalid non-empty values before resolving the instance client, and rely on the existing handler mapping from `ErrInvalidConfig` to HTTP 400. This keeps project- and organization-scoped browse requests from silently dropping an invalid age constraint.

Correct the comments that misstate the parameter's effect: three assert that it never affects which issues a search returns — false for the organization-scoped endpoint, where the value becomes that request's time range — and two present Kandev's hour/day/week set as Sentry's own relative-duration syntax, which also accepts seconds and minutes. The work order lists the exact sites.

Extend `apps/web/e2e/tests/integrations/sentry-settings.spec.ts` with a scenario titled `persists selected lookback period` in the existing issue-watcher block: select the `Last 30 days` option (`30d`), create the watch, reload, and assert the persisted summary.

## Tests

- `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.1`, `.2`, and `.5`: `TestRESTClient_SearchIssues_ForwardsLookbackOnlyWhereAccepted` in `apps/backend/internal/sentry/rest_client_test.go` covers every offered token across both endpoint paths, and `TestService_UpdateIssueWatch_PersistsLookbackPeriod` in `apps/backend/internal/sentry/service_issue_watch_test.go` covers the update half of `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.1`; that criterion's create half rides the E2E scenario below.
- `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.3`: `TestService_CheckIssueWatch_PollsStoredLookbackPeriod` in the same file proves a stored non-default token survives a store round-trip and reaches the search client unchanged.
- `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.4`: `TestService_IssueWatch_LookbackPeriodValidation` in the same file rejects a value outside the supported syntax (`30m`) on create and on a filter-carrying update, accepts an empty value, and lets an unrelated partial update through when the stored value is one the syntax rejects.
- `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.1` (offered set): `STATS_PERIOD_OPTIONS` in `apps/web/components/sentry/sentry-issue-watch-form.test.ts` pins the five option values, so the dialog cannot offer a token the backend's write guard rejects.
- `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.6`: the same test proves the poll refusal: a stored value outside the syntax stamps the watch error and returns without calling the search client.
- Direct browse validation: `TestService_Browse_RejectsInvalidStatsPeriod` rejects invalid periods for project- and organization-scoped searches before the client call, and `TestHTTP_SearchIssues_RejectsInvalidStatsPeriod` proves the HTTP boundary returns 400.

## E2E tests

- `AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.1` and `.3`: the `persists selected lookback period` scenario in `apps/web/e2e/tests/integrations/sentry-settings.spec.ts` (project `chromium`) selects `Last 30 days` (`30d`), creates the watch, reloads, and asserts the saved summary.

## Work orders

- [x] [Task 01: Honor all Sentry watcher lookback periods](task-01-honor-sentry-watcher-lookback-periods.md)

## Verification results

Ran from the repository root by the implementing sub-agent (commit `0c82dd006`), with the parent
re-running the focused tests and diagnosing the one failing gate:

- `(cd apps && pnpm install --frozen-lockfile)` — pass, workspace already current.
- `(cd apps/backend && go test -v -run '<the four tests>' ./internal/sentry)` — pass. The parent
  re-ran it with `-count=1`: `ok github.com/kandev/kandev/internal/sentry 0.097s`. The whole
  `./internal/sentry` package passes.
- `(cd apps/web && pnpm e2e:run tests/integrations/sentry-settings.spec.ts -- --grep "persists
  selected lookback period")` — 1 passed (17.5s).
- `(cd apps/web && pnpm e2e:sleep-ratchet)` — clean.
- `make fmt` — clean.
- `make typecheck` — clean.
- `make test` — fails in four packages unrelated to this change: `internal/common/config`,
  `internal/launcher`, `internal/system/updates`, and `internal/agentctl/server/config`. This
  session exports `KANDEV_SERVER_PORT`, `KANDEV_BACKEND_PORT`, `KANDEV_TRUSTED_PROXIES`,
  `KANDEV_RUNNING_AS_SERVICE`, and `KANDEV_VERSION`, which those tests read instead of their
  fixtures. The parent reproduced one on the same tree: it fails under the ambient environment and
  passes with those variables unset. No failing package is touched by this change.
- `make lint` — golangci-lint 0 issues, eslint clean, harness/specification/architecture lint pass.
- `python3 scripts/list-docs.py validate` — validated 291 decisions and 1036 specifications.

Test evidence: `TestRESTClient_SearchIssues_ForwardsLookbackOnlyWhereAccepted` and
`TestService_IssueWatch_LookbackPeriodValidation` were red before their production change; the two
persistence tests were green before and after, exactly as the design's test strategy states.

Delivery state lives here and in the work order, not in the paired specification: the design is
`current` because it describes the implemented system, while the requirement stays `draft` until the
change lands on `main`, since its `active` status describes the product contract rather than the
state of this branch.

## Risks

- The project-scoped request must lose the parameter without losing the `age:` term, or its searches would match issues of any age; the request test asserts the term rather than only asserting absence.
- The organization-scoped request keeps the parameter, so it stays exposed to that endpoint's accepted set; the request test pins the current contract.
- A previously stored inexpressible token is not rewritten and its polls fail closed, so the row stays idle until its lookback is replaced from the watch dialog: that dialog always sends the persisted filter, so saving another field alone is refused, and the period selector offers no way to clear the stored value.
- A watch whose polls previously failed for every stored token other than `24h` and `14d` resumes on the next poll and can import up to one page (100 issues) per configured project before deduplication catches up.
- The E2E mock enforces the window from the filter itself, so the E2E scenario cannot detect this regression on its own.
- Direct issue-browse requests now reject invalid non-empty periods at the service boundary, so an API caller cannot receive an unfiltered project search after the project endpoint stops receiving `statsPeriod`.

## Follow-up verification

- `(cd apps/backend && go test -run 'TestService_Browse_RejectsInvalidStatsPeriod|TestHTTP_SearchIssues_RejectsInvalidStatsPeriod' ./internal/sentry)` — pass.
