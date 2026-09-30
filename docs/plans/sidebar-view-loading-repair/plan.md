---
created: 2026-09-28
status: completed
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
legacy_specs: []
---

# Implementation Plan: Sidebar View Loading Repair

## Overview

Restore multi-repository views, responsive return switching, and actionable read
errors. Implement validation first, bounded first-page reuse second, and unified
desktop/phone status last. Execute sequentially in the primary session.

UI owns saved-view evaluation and presentation; task lifecycle, authorization,
and persisted user settings retain their existing owners. The user confirmed
that reducing the repository filter makes tasks load and requested all three
repairs. No further product choice blocks this package.

## Evidence and conformance

PR #3986 (`e802ee64e`) introduced `MaxSidebarViewValueBytes = 256` and applies
it to an entire `json.RawMessage` before decoding membership arrays. Six UUIDs
encode to 235 bytes; seven to 274. The rejection body is 55 bytes, matching
112 HTTP 400 sidebar-query failures in the retained backend logs across two days.
Repository filters use repository names/slugs, while workflow filters use UUIDs.
The actual repository count at rejection depends on name length; six UUIDs are
a size example, not a universal six-repository ceiling. The user confirmed
recovery by reducing selected repositories. The running
backend identified itself as `20d2d549a`.

The hook retains only one response and hides it on a different `viewKey`.
Its in-flight map does not cache completed reads. Both visible red messages
come from the same `page.error`. No successful live request durations were
available, so database slowness is not established. The older package reports
0.85-0.95-second warm 100,000-task fixture queries, not this installation's latency.

The collection limit violates existing filter semantics (002.3); 002.14 makes
the supported boundary explicit. Fast return switching changes the prior
single-page retention contract (002.2, .15-.16). Error ownership and recovery
are clarified by .17-.18. The system design and proposed
[ADR](../../decisions/2026-09-28-sidebar-view-page-reuse.md) preserve server paging.

The attempted temporary Go reproduction did not run because Go is absent in
this executor. Its temporary source was removed. Production and permanent
test files remain unchanged at this design checkpoint.

## Scope

### In scope

- Per-element collection validation and safe structured query errors.
- Five recently visited first pages, bounded by bytes and expiry, with
  synchronous reuse and background refresh under the same query identity.
- One localized query status per surface, accurate cold/refresh failure states,
  and preserved desktop/phone navigation and accessibility.
- Targeted regression tests, isolated browser proof, query timing evidence,
  and public API/task documentation during implementation.

### Out of scope

All-task client filtering, server result caching, prefetch, persistent browser
cache, optimistic reconstruction of unseen views, task lifecycle changes,
database migrations without measured need, and changes to the user's instance.

## Technical approach

- `models/sidebar_task_view.go`: validate list cardinality separately from
  decoded scalar type/length; preserve `in`/`not_in` empty-set semantics.
- `handlers/sidebar_task_http.go`: preserve `error`, add `error_code` and safe
  reason/index/limit details. Do not expose raw submitted values or SQL.
- Add `lib/sidebar/sidebar-task-page-cache.ts`: store-scoped controller for
  shared reuse, request ownership, finite LRU/byte/age limits, and invalidation.
  Integrate `use-sidebar-task-page.ts`; retain current-page paging semantics.
- Reuse `workspaceContextGeneration` and sidebar query revisions. Key by the
  full query/preferences, not view ID. Invalidate inactive results conservatively;
  account/workspace changes and denial clear every result before rendering.
- `ApiError` already retains structured bodies. Add a sidebar-specific error
  mapper and shared status component. Remove archive-specific duplicates from
  `task-session-sidebar.tsx`, `use-workspace-sidebar-tasks.ts`, and phone read status.
  Keep paging buttons separate from query status.
- Update six locale catalogs and generate Traditional Chinese values through
  `i18n:zh-hant`. Update public docs only with the implementation.

| Boundary | Required behavior | Evidence |
| --- | --- | --- |
| SQLite/PostgreSQL | Same list validation, global filter semantics, bounded rows | Handler/model tests and both-dialect repository cases |
| Desktop/phone/app navigation | Shared results and errors; one request per identity | Hook multi-consumer tests and both browser projects |
| Old/unknown server error envelope | Localized generic fallback, no raw error leakage | Error mapper tests |
| Workspace/account/permission changes | No retained data crosses context | Hook and denial/browser regressions |

## ASCII UI preview

UI-01: Desktop Tasks, returning to a cached view while its response is held.

```text
BEFORE                          AFTER
TASKS [Kandev Tasks v] [Filter]  TASKS [Kandev Tasks v] [Filter]
[blank while request runs]      Updating tasks...
                                Task A           Running
                                Task B           Waiting
                                [Previous] Page 1 [Next]
```

UI-02: One status region, directly below view controls.

```text
Cold load:       Loading tasks...        (existing loading treatment)
Invalid filter:  Filter 1 has too many selections. Select up to 1,000.
                [existing Filters control remains available above]
Read failure:    Tasks could not be loaded. [Retry]
Refresh failed:  Tasks could not be refreshed. [Retry]
                Task A
                Task B
Empty success:  [existing empty-view state]
```

UI-03: Phone task-title picker, same states inside the existing inset drawer.

```text
+----------------------------------+
| Tasks                    [Close] | fixed header
| [Kandev Tasks v]        [Filters] |
| Updating tasks...                | one status region
|----------------------------------|
| Task A                  Running  | body scrolls
| Task B                  Waiting  |
| [Previous] Page 1 [Next]          |
+----------------------------------+
```

The nearest exemplar is `SessionTaskSwitcherSheet`: retain its inset drawer,
dynamic viewport containment, safe-area spacing, focus return, and sole body
scroller. App navigation retains its own menu scroller. Primary action is tapping
a task to navigate and dismiss; switching views keeps the picker open. Desktop
controls retain 28px fine-pointer sizing, phone actions have at least 44px targets.
No new modal, overlay, nested scroller, or layout animation. Structure and status
ownership are required; wording is illustrative and must be localized.
UI-01 maps to 002.15-.16, UI-02 to .17-.18, and UI-03 additionally to .9.

## Tests

| Criteria | Planned failing regression |
| --- | --- |
| 002.3, .14 | `TestSidebarTaskViewQueryMembershipLimits`: long repository-name arrays and seven/1,000 UUIDs accepted; scalar/cardinality/total-body boundaries rejected correctly |
| 002.14, .17 | `TestHTTPQuerySidebarTasksStructuredValidation`: safe typed details and unchanged workspace authorization |
| 002.3, .14 | `TestQuerySidebarTaskPageLargeMembership`: real SQLite/PostgreSQL results for `in`/`not_in`, maximum combined parameters, and a later matching task |
| 002.2, .7, .15-.16 | New cache unit tests plus hook `shows cached first page before background refresh resolves`: identity, LRU, bytes, TTL, revision/context races |
| 002.6, .8, .12, .18 | Hook refresh failures and existing archive/unarchive/delete/status-summary regressions with retained snapshots |
| 002.9, .17-.18 | Error mapper and shared status component; one accessible error per desktop/phone surface |

## E2E tests

Extend `e2e/tests/task/sidebar-task-pagination.spec.ts` (chromium) and
`mobile-sidebar-task-pagination.spec.ts` (mobile-chrome). Seed real repositories,
tasks, views, and distinct current conversation on an isolated backend.

1. Select at least seven repositories whose combined encoded names exceed 256 bytes
   and show matching tasks; also seed a workflow UUID-list case (002.3, .14).
2. Load A and B, hold A's refresh response, return to A and assert its rows are
   already visible before releasing the response; no B rows or conversation
   change (002.7, .15).
3. Return after membership invalidation and workspace change; no removed/foreign
   rows; release delayed responses to prove fencing (002.8, .16).
4. Inject validation, network, refresh, and access errors; exactly one relevant
   status and correct Retry/filter recovery (002.6, .12, .17-.18).
5. On phone, prove drawer/menu parity, open-state preservation on view switch,
   task navigation, 44px controls, internal scrolling, and no horizontal overflow (.9).

Intercept only to hold or fail requests. Prefer explicit response gates over
wall-clock assertions. Use the managed E2E runner to rebuild and isolate data.

## Work orders

- [x] [Task 01: Correct membership validation](task-01-membership-validation.md)
- [x] [Task 02: Reuse bounded first pages](task-02-view-page-reuse.md)
- [x] [Task 03: Unify status and prove desktop/phone recovery](task-03-query-status.md)

## Verification results

Design validation passed on 2026-09-28:

- `python3 scripts/list-docs.py validate`: 322 decisions and 1,220 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: all three work orders and
  linked REQ/AC/design references pass a planned-runtime coverage preflight.
- `git diff --check -- docs/specs docs/decisions docs/plans`: passed.
- Existing test/input paths were checked; new cache/status test paths are
  explicitly marked for creation at the design checkpoint.

Implementation completed on 2026-09-28. Each work order records its exact checks
and scope. Backend membership/model/HTTP tests passed with both SQLite and a
real disposable PostgreSQL 17 instance. Frontend validation includes 105 targeted
unit/event/consumer tests after the review regressions. Desktop and phone paging/recovery
browser tests pass against the managed production build.

Cold-query benchmark results and the incomplete PostgreSQL middle/final run are
explicitly recorded in Task 02. There is no SQL performance claim. Workspace
race proof uses controllable hook promises; browser coverage focuses on visible
filter/reuse/recovery behavior. No live user instance was mutated.

Fresh synthetic desktop and phone refresh-error captures confirm one status and
retained rows. The phone retains its existing draggable drawer (no added Close
button); dismissal uses the existing native behavior. Public docs, all translations,
and linked specifications describe the implemented behavior.

## Risks

- Event churn invalidates snapshots and can reduce cache benefit. Cold-query
  latency remains separate; record timings rather than promising instant cold loads.
- Shared controllers must not leak between stores/accounts or multiply requests.
- Large selections require real dialect parameter-limit coverage.
- Error precedence must preserve workspace-access errors and distinguish empty success.

## Related records

[Archived sidebar loading](../archived-sidebar-loading/plan.md) and
[original archive views](../sidebar-archived-filter/plan.md) are completed history.
Their prior command results remain unchanged. This package owns the new matrix
and supersedes only filter admission, cache retention, and query-status behavior.

## Review follow-up

Reproduced two P1 findings with failing hook tests: a revision-invalidated response
could briefly replace the accepted page, and a denied read left an idle sibling's
local page visible. Both now pass, including late completion after shared denial.
The reported missing Retry after validation correction already passed; a contract
test preserves that behavior. Also removed a redundant pagination guard, added
initial-loading status coverage, and clarified request/cache key names.

The first CI monitor stopped after five failed policy lookups (26 passed,
0 failed, 31 pending at that snapshot). Direct GitHub ruleset retrieval succeeded;
this monitor result does not establish terminal CI. Follow-up delivery preserves
that limitation and does not claim the PR merged or deployed.

Backend CI follow-up: integrated current main to include its corrected unordered
Codex background-work assertion. Fixed the Codex utility fake-npx readiness race
by publishing captured arguments with a same-directory rename after writing,
instead of exposing an empty file before `printf` finishes. Both failing tests
passed 20 race-enabled repetitions; both owning packages passed three complete
race-enabled runs. Merged sidebar cache/hook tests (27) and frontend typecheck
passed; desktop sidebar and drag/drop E2E passed all nine scenarios without
retries. The change is confined to test fixtures and base integration.
Phone sidebar E2E also passed all three scenarios with retries disabled. Harness
validation, specification lint, and conflict/whitespace checks passed.
Backend lint against the integrated main SHA passed with zero issues using
`GOMAXPROCS=2` and `--concurrency=2`; the initial higher-concurrency run was
interrupted after exceeding its budget and is not counted as a pass.

## Native-memory follow-up

[Sidebar query memory repair](../sidebar-query-memory/plan.md) adds SQLite preparation and reader-pool memory coverage.
This package's completion and historical results remain unchanged.
Its existing warm timing and Go allocation measurements do not establish the new native-memory budgets.
The follow-up preserves this package's user-visible contracts and owns the expanded resource test matrix.

The follow-up also proposes normalized homepage/sidebar records, complete-coverage local evaluation, lazy archive reads, and initial-load progress under soft invalidation.
Its [shared-state decision](../../decisions/2026-09-29-shared-sidebar-task-state.md) identifies the revised rules. Historical completion remains unchanged.
