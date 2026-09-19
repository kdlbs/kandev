---
id: "01-honor-sentry-watcher-lookback-periods"
title: "Honor all Sentry watcher lookback periods"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001
acceptance_criteria:
  - AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.1
  - AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.2
  - AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.3
  - AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.4
  - AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.5
  - AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.6
system_design:
  - ../../specs/integrations/system-design/sentry-watcher-lookback-periods.md
---

# Task 01: Honor All Sentry Watcher Lookback Periods

## Summary

Make every offered Sentry lookback token work against the provider by forwarding it only to the issue endpoint whose accepted values include it, keeping the `age:` eligibility constraint on both, guarding the stored value, and correcting the comments that assert the old mechanism.

## In scope

- Add the failing per-endpoint request-shape regression for all five tokens.
- Gate `statsPeriod` on the same project check that selects the issue endpoint, keeping the existing non-empty condition so an empty lookback sends nothing.
- Validate a non-empty lookback token on create and on a filter-carrying update by reusing `parseStatsPeriodUnits` as the single accepted-set authority; do not add a second parser.
- Fail a poll closed when a stored lookback value is outside the syntax.
- Correct the five comments that deny or misstate the mechanism: the `statsPeriodPattern`, `parseStatsPeriodUnits`, `statsPeriodAgeToken`, and `buildIssueQueryString` doc comments in `rest_client.go`, plus the stats-period test comment in `rest_client_test.go`. Three assert the parameter never affects which issues a search returns, and the other two present Kandev's hour/day/week set as Sentry's own relative-duration syntax, which also accepts seconds and minutes.
- Add the service round-trip, update-persistence, rejection, empty-acceptance, unaffected-update, and poll-refusal cases.
- Carry an `@covers AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.<n>` anchor on each new test function and on the E2E scenario, matching the traceability guide.
- Pin the offered lookback values in the watch form's unit tests, so the dialog cannot offer a token the backend's write guard rejects.
- Add the Sentry watcher settings E2E scenario for a non-default token.

## Out of scope

- New lookback values or custom durations.
- Unifying the endpoints' accepted sets or their per-issue event-count windows.
- Database migrations, public API field changes, or rewriting stored tokens.

## Acceptance

- A project-scoped request carries `age:-<token>` and no `statsPeriod`; an organization-scoped request carries both, for every offered token; and an empty lookback carries neither, on either path.
- A non-empty lookback token outside the supported hours/days/weeks syntax, for example `30m`, is rejected on create and on a filter-carrying update, while an empty value stays accepted and an untouched stored value never blocks an unrelated update.
- A stored non-default token round-trips through the store and reaches the search client unchanged, while a watch whose stored lookback is outside the syntax fails its poll with a recorded error and creates no tasks.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -v -run 'TestRESTClient_SearchIssues_ForwardsLookbackOnlyWhereAccepted|TestService_CheckIssueWatch_PollsStoredLookbackPeriod|TestService_UpdateIssueWatch_PersistsLookbackPeriod|TestService_IssueWatch_LookbackPeriodValidation' ./internal/sentry)
(cd apps/web && pnpm e2e:run tests/integrations/sentry-settings.spec.ts -- --grep "persists selected lookback period")
(cd apps/web && pnpm e2e:sleep-ratchet)
make fmt
make typecheck
make test
make lint
python3 scripts/list-docs.py validate
```

## Files likely touched

- `apps/backend/internal/sentry/rest_client.go`
- `apps/backend/internal/sentry/rest_client_test.go`
- `apps/backend/internal/sentry/service_issue_watch.go`
- `apps/backend/internal/sentry/service_issue_watch_test.go`
- `apps/web/e2e/tests/integrations/sentry-settings.spec.ts`
- `docs/specs/integrations/requirements/sentry-watcher-lookback-periods.md`
- `docs/specs/integrations/system-design/sentry-watcher-lookback-periods.md`
- `docs/plans/sentry-watcher-lookback-periods/plan.md`

## Dependencies

None.

## Risks

- Dropping the parameter on the project-scoped path without keeping the `age:` term would silently widen every watch search; the request test must assert both.
- Validation must keep accepting an empty lookback: the field is optional, API clients may omit it, and a legacy row may store an empty value, even though the first-party watch form always sends one of the offered tokens.
- The poll guard must stamp the error and return before any search call, or a row with an out-of-syntax token would dispatch tasks for issues of any age.
- The organization-scoped request still depends on that endpoint's accepted set; the request test pins the current contract.
- The E2E mock enforces the window from the filter itself and never sees the outbound request, so it cannot detect this regression alone.

## Parallelism

`sequential`

## Inputs

- `docs/specs/integrations/requirements/sentry-watcher-lookback-periods.md`
- `docs/specs/integrations/system-design/sentry-watcher-lookback-periods.md`
- `apps/backend/internal/sentry/rest_client.go`
- `apps/backend/internal/sentry/service_issue_watch.go`
- `apps/web/e2e/tests/integrations/sentry-settings.spec.ts`

## Results

Implemented in commit `0c82dd006` (5 files, +365/−26): the endpoint-gated `statsPeriod`, the write
guard on create and on a filter-carrying update, the poll-time guard, the five corrected comments,
the four Go tests with `@covers` anchors, and the `persists selected lookback period` E2E scenario.

Verification: the focused Go tests pass (re-run with `-count=1`), the E2E scenario passes (15.5s),
`pnpm e2e:sleep-ratchet` is clean, `make fmt`, `make typecheck`, and `make lint` pass, and the
specification gates pass. `make test` fails only in four packages unrelated to this change that
read this session's ambient `KANDEV_*` variables; the failure reproduces under that environment and
clears with those variables unset. Exact commands and outcomes are in the plan's Verification
results.

Review: the design package passed 28 adversarial rounds before implementation, and the implemented
change passed its first implementation round with no findings.
