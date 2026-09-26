---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001
---

# Sentry Watcher Lookback Periods System Design

## Purpose and boundaries

The integrations system owns the Sentry issue-watch filter, the manual issue browser, and the outbound Sentry issue search they share. `SearchFilter.StatsPeriod` stays the persisted wire field only because existing settings and rows use it: its product meaning is a lookback token.

Sentry behaves differently per issue endpoint, and `issuesSearchPath` already picks between them:

- The project-scoped endpoint `/projects/{org}/{project}/issues/` accepts only `''`, `24h`, and `14d` in `statsPeriod`, defaults to `24h`, and answers any other value with a 400 (`Invalid stats_period`). A watch poll always reaches this endpoint, because a watch always carries at least one project, and the browser reaches it whenever a project is selected.
- The organization-scoped endpoint `/organizations/{org}/issues/` accepts any `<n><s|m|h|d|w>` value there and uses it as its request time range, defaulting to 90 days when it is absent. The browser reaches this endpoint when it browses an org without selecting a project.

Kandev expresses issue eligibility with the `age:` search term, which accepts `m`, `h`, `d`, and `w` and therefore every offered token, on both endpoints.

The two endpoints also differ in what the parameter sizes:

- On the organization-scoped endpoint it becomes the request time range that reaches the query and the serializer, so it scopes both the issues the search returns and the per-issue `count` and `userCount` the browser renders.
- On the project-scoped endpoint no `start`/`end` window reaches the serializer, so the value sizes only the event series Kandev never parses, and nothing else it reads.

The lookback therefore reaches only the endpoint that accepts it, which fixes the rejected option without narrowing or widening the set of issues any surface returns.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001` | [Data and contracts](#data-and-contracts), [Failure and recovery](#failure-and-recovery), [Compatibility](#compatibility) |

## Components and responsibilities

- `apps/web/components/sentry/sentry-issue-watch-form.ts` owns the offered lookback tokens for the watch dialogs and sends the selection in the persisted watch filter, while the issue browser keeps its own period list in `sentry-issue-dialog.tsx`. Both lists currently hold the same five tokens, so a token change has to update both — but the lists are not the binding constraint: every offered token must be one `parseStatsPeriodUnits` accepts, a whole number of hours, days, or weeks from 1 to 3650, because a token outside that set is rejected at watch create and on a filter-carrying update.
- `apps/web/components/sentry/sentry-issue-dialog.tsx` sends the browser's selection in the same filter shape, with no project when the user browses an org.
- `apps/web/components/sentry/sentry-issue-watch-table.tsx` renders the stored token in the watch summary.
- `internal/sentry.Service` normalizes and validates the filter, persists it, and passes it unchanged to each per-project search.
- `internal/sentry.RESTClient.searchIssues` builds the provider request through `buildIssueQueryString` and `statsPeriodAgeToken`, and gates the outbound parameter on the same project check that selects the endpoint, for a non-empty lookback only.
- `MockClient` keeps enforcing the same lookback against `FirstSeen` for deterministic settings E2E behavior.

## Data and contracts

No API, wire-field, or schema migration is required: `SearchFilter.StatsPeriod` keeps the values it already holds.

The outbound request contract is:

- `query` includes `age:-<selected-token>` whenever a lookback is selected, on both endpoints; an empty lookback sends neither an age term nor a parameter.
- The `statsPeriod` parameter is sent only when the lookback is non-empty and the filter carries no project, which is the organization-scoped endpoint whose accepted values cover every offered token; the project-scoped endpoint never receives it, and an empty value is never sent, because the organization-scoped endpoint rejects an empty value.
- The accepted lookback set is exactly what `parseStatsPeriodUnits` accepts. The write sites reject anything else (`AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.4`) and a stored value outside it fails the poll (`AC-INTEGRATIONS-SENTRY-WATCHER-LOOKBACK-PERIODS-001.6`).

## Control flow

1. A user selects an offered token in the watch dialog or the issue browser.
2. `Service` stores it in `filter_json` (watch) or passes it straight to the search (browser).
3. `SearchIssues` reaches the project-scoped or organization-scoped endpoint, depending on whether a project is present.
4. `buildIssueQueryString` appends `age:-<token>` to the Sentry search query, and the parameter is set only for a non-empty token on the organization-scoped request.
5. Sentry returns only issues within the requested age window; watcher deduplication and task dispatch continue unchanged.

## Failure and recovery

- An offered token always yields an age constraint, so an unsupported value can only arrive through a direct API call or a database write.
- `validateIssueWatchCreate` and the filter branch of `UpdateIssueWatch` are the two write sites. They reject such a value through `parseStatsPeriodUnits`, returning `ErrInvalidConfig` (400); an empty value stays valid because the field is optional. The removed project-scoped parameter was the watch path's only rejection, so this guard and the poll guard below replace it.
- `CheckIssueWatch` fails closed for a row that already stores one, mirroring its existing project-less guard: it stamps the watch error and returns `ErrInvalidConfig` without searching or dispatching. Without that guard, the fix would un-shadow the helper's leave-unfiltered tolerance and let old issues become tasks.
- Validation stays scoped to the request's own filter, mirroring `validateFilterStatuses`: an update that omits the filter never fails on a previously stored value, and untouched rows are not rewritten.
- A request that carries the filter is validated as a whole, and the watch dialog always sends the persisted filter back, so a row storing a value outside the accepted set cannot be saved from that dialog until its lookback is replaced with one of the offered tokens. Editing another field alone is refused; the period selector offers only the accepted tokens, so the stored value cannot be re-sent unchanged.
- `parseStatsPeriodUnits` remains the single accepted-value authority; `statsPeriodAgeToken` and the mock's duration parse both build on it, so the three cannot disagree. Its syntax is narrower than Sentry's own relative-duration grammar, which also accepts seconds and minutes, so a direct caller cannot store a `30m` lookback even though the `age:-30m` term would be valid upstream.

## Persistence

Watch rows keep their stored token. The corrected request shape applies on the next poll, and no row is rewritten. A tick already in flight when the change lands holds no durable state: it reads the dedup set and searches once, then the next tick issues the corrected request.

## Compatibility

- Existing watches keep loading and polling with their stored token.
- The project-scoped issue browse is repaired: selections other than `24h` and `14d` currently fail against Sentry, and now succeed. Its per-issue counts are unaffected, for the reason given under [Purpose and boundaries](#purpose-and-boundaries).
- The organization-scoped issue browse is unchanged: it keeps sending its time range, so its returned issues and the per-issue counts it displays keep their current scope.
- Omitting the parameter on both endpoints was rejected: it would widen the organization-scoped browse's request range to Sentry's 90-day default and change the window its rendered per-issue counts are drawn from, for no gain, since that endpoint accepts every offered token.
- The browse service validates every non-empty lookback before it selects an instance client. A value outside the accepted set returns HTTP 400 for both project-scoped and organization-scoped browse requests, so neither path can silently search without an age limit. Empty values remain accepted. The dialog only sends an offered token, but this guard keeps direct API callers aligned with watcher writes and polls.
- A row that already stores a value outside the accepted set is not rewritten: its polls fail closed and create no tasks. That restores the pre-change behavior, where the removed parameter made every such poll fail at the provider.

## Observability

No new metrics. A failed watch search surfaces as the poller's warning log, and a manual trigger returns the error to its caller; the project-less, instance-resolution, and lookback failures stamp the row, and the settings table renders no error column. Surfacing that recorded error in the Sentry settings table is out of scope here, and the watch-row comments that describe a settings-UI pin or a single stamping caller were already inaccurate before this change and are left uncorrected.

## Test strategy

The request-shape, write-rejection, and poll-refusal tests are the pre-change failures. The stored-value round-trip and the update-persistence tests characterize behavior that already holds. The settings E2E proves retention and reload only, so the request-shape test is the sole provider-compatibility guard. The per-criterion mapping lives in the plan.

## Related decisions

- No ADR: the change restores the existing lookback contract without introducing a new boundary, and the rejected alternative is recorded under [Compatibility](#compatibility).
