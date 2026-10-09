---
created: 2026-10-07
status: completed
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001
  - REQ-UI-LIST-STEP-GROUPING-001
  - REQ-UI-MOBILE-MENU-005
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
  - ../../specs/ui/system-design/task-list-workflow-step-grouping.md
  - ../../specs/ui/system-design/unified-mobile-navigation.md
legacy_specs: []
---

# Implementation Plan: Single Task List Search Debounce

## Overview

Keep the existing input's trailing 300 ms debounce and remove the second wait
in the List page. One sequential work order owns the regression, minimal page
correction, targeted checks, and a small before/after measurement. The user subsequently authorized implementation with “ok, go for impl”;
implementation and the targeted validation gates are complete.

## Evidence and cause

Investigation HEAD: `330e02a47808c11ca315ae30456fcce7f4806db5`.
`git ls-remote origin refs/heads/main` returned that exact SHA on 2026-10-07.
The managed branch is `feature/remove-duplicate-tas-4d8fc5`; initial status was
clean. The historical audit baseline was
`059260b30fc68bbcbead629f7fa7e80f1ee0a5e8`, not this checkout.

`TaskSearchInput.handleChange` immediately sets `localValue`, then schedules
`onChange` using default `debounceMs = 300`. List passes `setSearchQuery` to it.
`useTasksPageSetup` then calls `useDebounce(viewState.searchQuery, 300)`, whose
own effect schedules a second timer. Only that result reaches the fetch effect.
A single last input therefore crosses two serial 300 ms timer windows in source;
React scheduling, request and rendering costs are additional. This is a source
trace, not an independently measured 600 ms latency contribution.

The supplied audit reported roughly 0.8-0.9 seconds to filter readiness across
20 warmed actions per repetition, three repetitions at 100/1,000/5,000 tasks in
dense/distributed datasets. Sixteen desktop runs have historical source/build
provenance limits; checked mobile runs provide separate support. Parent raw
reports are unavailable here. No current browser timing or speedup is claimed.

Workspace-local raw source snapshots, exact line trace, SHA-256/Git identity,
and validator output are retained under `.tmp/task-search-debounce/`. Fourteen
initial source snapshots were byte-compared with `git show HEAD:<path>` and all
matched. `source-identity.json` records the remote main SHA and absence of a
build/timing run. No package install was necessary for source-only probes.

## Scope

In scope: List search admission, clear/cancellation, retained filter/pagination
semantics, existing reply fencing, desktop/tablet/phone wiring, regression
coverage and a bounded timing comparison.

Out of scope: UI redesign, shared input/hook behavior changes, fetch freshness
contract changes, Zustand architecture, virtualizers, backend/API changes,
dependencies/lockfiles, audit/sibling artifacts, commits/push/PR, executor
settings, and additional tasks/sessions/native agents.

## Requirement and design reconciliation

UI owns reusable listing interaction; Tasks owns lifecycle and persisted task
data. Existing `REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001` covers List and
workflow choices (existing AC .1/.2), but did not define search admission.
The package adds the smallest clarification, AC .13-.15, to that owner and its
paired design's **List search admission** section. This is missing performance
intent plus incomplete technical design, rather than a proven violation of a
previously specified 300 ms criterion. It creates no standalone repair spec.

Retain `AC-UI-LIST-STEP-GROUPING-001.6` (filter/pagination/hierarchy invariants)
and `AC-UI-MOBILE-MENU-005.2/.3` (reachable search, focus and hide clearing).
The grouping and mobile-navigation specifications need no correction.

Companion inventory: `task-listing-display-preferences` and
`threads-home-default` plans are done; `homepage-view-settings` is implemented;
`task-list-workflow-step-grouping` is done. Their scopes and recorded results
remain intact; this package owns only the added search checks. Existing ADRs
for portable preferences remain applicable; this local timer removal needs no
new architectural decision.

Assumption check: the caller settled scope, immediate text, bounded frequency,
clear/latest-input correctness and planning-only authorization. Source verifies
the duplicate timer and all consumer paths. No material product choice remains
unresolved. Timing attribution was tested during the authorized implementation;
results and limitations are recorded below.

## Technical approach

| Source boundary | Current role | Proposed disposition |
| --- | --- | --- |
| `components/kanban/task-search-input.tsx` | Immediate local text; replaceable trailing callback; immediate clear; unmount cleanup | Retain timer and API unchanged; single debounce owner |
| `components/kanban/kanban-header.tsx` | Desktop and tablet inputs; tablet menu receives shared callbacks | Retain all wiring |
| `components/kanban/mobile-search-bar.tsx`, `mobile-menu-sheet.tsx` | Phone inline input / tablet menu input | Retain default 300 ms input behavior |
| `components/kanban/kanban-header-mobile.tsx` | Search reveal/focus; hide publishes empty query | Retain clear and unmount cancellation |
| `app/tasks/tasks-page-content.tsx` | Header and phone input compose over one callback | Retain composition |
| `app/tasks/tasks-page-client.tsx` | Local stored query; second timer; fetch/reset/loading and request sequence | Sole production edit: remove page debounce; use admitted `searchQuery` directly |
| `hooks/use-debounce.ts` | Shared value/callback debounce | Retain unrelated users |
| `app/tasks/tasks-page-fetch-policy.ts` | Initial hydrated empty-query fetch skip | Retain API; page passes `debouncedQuery: searchQuery` |
| `lib/api/domains/kanban-api.ts` | List HTTP query serialization | Retain request contract |
| `hooks/use-task-list-facet-selection.ts`, `use-task-list-workflow-steps.ts` | Facet sort/group over accepted rows; explicit/foreground refresh | Retain projections and admitted-query refresh |
| `components/kanban-board.tsx` -> `hooks/domains/kanban/use-kanban-data.ts` | Shared input updates board local search; synchronous local filtering | Unrelated caller; retain its single input timer |
| `app/threads/threads-page-client.tsx` | Uses header without search callback | No List search consumer; retain |

Remove the `useDebounce` import and call in `useTasksPageSetup`. Rename page
internal `debouncedQuery` plumbing to `searchQuery`, using
`viewState.searchQuery` directly. Avoid a stale alias suggesting a second timer.
Adapt the fetch-policy call's field and loading predicate within this file.
Retain request sequence/workspace checks around rows, total, errors and loading.
Retain the microtask pagination reset. From a later page, current code can start
a transient request for that page before requesting page one; tests require
correct final page-one results and newer-request protection, without a
pagination redesign or an invented one-request guarantee for that case.

The alternative of keeping the page timer requires bypassing timers in every
List input surface, including tablet-menu search. Shared zero delay still queues
a timer. Keeping the existing input owner removes the redundant page wait with
one production-file change and retains all unrelated callers.

## ASCII UI preview

### UI-01: Desktop/tablet List search

Entry: `/tasks`, existing header search. Same controls and geometry before/after.

```text
[ Workspace | List ]       [ Search tasks: Alpha | X ]
[ Sort v ] [ Group v ] [ Archived ]
Alpha task
[ Previous ] [ 1 ] [ Next ]
```

### UI-02: Phone List search

Entry: List title/context -> View options -> Search tasks. Existing inline flow.

```text
[ Workspace | List v ]                    [ Menu ]
[ Search tasks: Alpha                         X ]
Alpha task
[ Previous ] [ 1 ] [ Next ]
```

Before: local text -> input timer -> page timer -> request -> accepted rows.
After: local text -> input timer -> request -> accepted rows.
Clear/hide cancels queued text and admits empty query without either wait.
These are source-supported interaction sketches; spacing/data are illustrative.
No new label, localization, hierarchy or geometric requirement is introduced.

Phone keeps the shipped `MobileSearchBar` and `MobileMenuSheet` exemplars: a
short temporary choice in the inset options drawer, followed by inline search
for frequent editing. Existing header/search stay outside the list scroller;
List owns vertical content scrolling. Preserve current safe-area/viewport
behavior, keyboard focus and touch targets. Business state and filtering remain
shared. Existing layout is retained, not asserted to meet new geometry criteria.
Map UI-01/UI-02 to AC listing .13-.15 and mobile menu .2/.3; rendered checks are
specified below.

## Tests

New `apps/web/app/tasks/tasks-page-search-debounce.test.tsx` uses fake timers
with real `TaskSearchInput`, `useDebounce` (on RED), and `TasksPageClient` query,
fetch and pagination effects. Mock API, unrelated subscriptions and bulky
presentation only. Header test doubles must render the real input with the
received value/callback, including phone composition through real
`TasksPageContent`/`MobileSearchBar`; never replace the input timer, page hook or
fetch effects with an immediate mock. Use the existing MR-hydration test's
StateProvider setup, but do not copy its mocked debounce.

| Criterion | Regression scenario |
| --- | --- |
| Listing .13 | `admits the last input after one debounce window`: page zero, hydrated baseline, input text is immediate; zero query calls at 299 ms; exactly one matching query call after advancing 1 ms in a separate async act. RED: still zero at 300 ms; source trace predicts admission only after another 300 ms |
| Listing .13/.14 | Rapid A -> AB -> ABC inside 300 ms windows: no intermediate query, one final request after last edit +300 ms; clear before timer, clear after accepted results, typed empty string, phone hide/unmount before timer; advance beyond former two windows to prove cancelled text never returns |
| Listing .15; grouping .6 | Page two -> query -> final page one; retained page size/workflow/repository/archive/sort; facet/group projection unchanged; initialized empty query skips redundant initial read |
| Listing .15 | Deferred A/B promises: start B after A, resolve B then A; rows/total/loading/error remain B. Repeat old error/finalizer and previous-workspace response; characterize the existing started-request boundary, not raw keystroke freshness |

Do not add a test that only reproduces two hand-written timers. The regression
must fail through production page wiring. Meaningful request counts apply on
page zero with foreground/mutation refreshes held idle; those explicit refresh
paths are outside the typing rate bound.

## E2E tests

Extend `tests/task/task-list.spec.ts` (chromium) with a search/clear/page-reset
flow including selected workflow/repository and archive/sort retention. Retain
existing pagination and grouping scenarios. Add
`tests/task/task-search-debounce.spec.ts` (chromium) for rapid latest-query
replacement and deferred out-of-order success/error control against List UI.
Use a route latch, not random backend delay; release B then A and inspect rows,
counts and current loading/error feedback. Test byte identity as well as actual
query params so an unrelated list refresh cannot satisfy the waiter.

Extend `tests/task/mobile-task-list-search.spec.ts` (mobile-chrome) to use real
taps to reveal/focus, edit rapidly, clear, hide during queued input and reopen,
then search from page two and verify page-one rows. Include filter/sort/archive
combination through existing native options; retain existing two scenarios.
Use causal HTTP observers armed before actions; replace touched `networkidle`
and budgeted waits with `waitForHttp`/default DOM assertions. Negative request
checks use a live counter and sanctioned `dwell(..., "negative-assertion", ... )`.
Capture/inspect one desktop and phone screenshot during these checks; no UI
geometry change is expected. No broad suite, worker override, or overlapping run.

## Measurement protocol

After the implementation request, create disposable diagnostic specs at
`apps/web/e2e/tests/task/task-search-debounce-measurement.spec.ts` and
`apps/web/e2e/tests/task/mobile-task-search-debounce-measurement.spec.ts` (not
permanent tests). Use the same managed fixture, 100 uniquely named tasks, no active
background/mutation refresh, page one, and matching browser/device/build.
Run baseline before editing production, then corrected source. For each phase,
run three repetitions of 20 alternating warmed queries for desktop and phone.
Discard initial navigation/warmup from samples and verify expected rows each time.

Capture browser `performance.now()` at the real input event (`t_input`), a
browser-side wrapper around matching `fetch` dispatch (`t_request`), the matching
resource's response end (`t_response`), and first correct settled rows observed
by MutationObserver plus the next animation frame (`t_rows`). Keep each query's
identifier/page/filter tuple to correlate traffic and DOM. Record driver action
start/end separately; all browser deltas use one clock. Preserve raw JSON and
summarize median/p95 for input-to-request, request-to-response,
response-to-rows, input-to-rows and driver duration.

Input-to-request includes intentional debounce plus React/effect scheduling;
it is not a pure timer measurement. The fake-timer regression isolates the
single logical window. Request/response includes transport/server work;
response-to-rows includes decoding/React/paint. Browser-visible rows and driver
observation are separate costs. Do not subtract controller duration from a
browser delta or compare historical audit timings as equivalent samples.

Record exact Git SHA/diff, source SHA-256, Vite asset hashes, backend binary
hash/version, pnpm/Node/Chromium versions, viewport, seeded task count and command
with each run. Verify served asset identities after every build. Managed runner
builds fresh; never bypass freshness checks. Preserve suspicious readbacks and
stop on byte/build mismatch, rather than retrying broken binaries. This bounded
100-task comparison diagnoses the timer; it does not replicate the full audit
or establish scaling at 1,000/5,000 tasks. No measured speedup is promised.

## Work orders

- [x] [Task 01: Keep a single input-owned debounce](task-01-single-debounce.md)

Sequential; no dependencies or delegation. Estimated implementation/check time:
45-90 minutes with a healthy local toolchain; timing repetitions add 10-20 minutes.

## Verification results

Planning validation on 2026-10-07:

- Source trace: confirmed two serial timers on current remote main; all 14
  initial source snapshots match HEAD bytes. Raw evidence retained locally.
- `python3 scripts/list-docs.py validate`: passed, 360 decisions and 1,425 specs.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node .tmp/task-search-debounce/validate-coverage.cjs`: passed, one covered
  work order and zero errors against the prospective List source edit.
- `git diff --check -- docs/specs/ui/requirements/task-listing-display-preferences.md
  docs/specs/ui/system-design/task-listing-display-preferences.md
  docs/plans/task-search-debounce`: passed; status includes both untracked plan
  files and only the two intended existing spec edits.

Implementation validation completed on 2026-10-09 at the same HEAD:

- RED: the real input/page integration issued no request at 300 ms on desktop
  and phone before the fix. GREEN: four targeted Vitest suites, 26 tests passed.
- Fresh managed E2E: 11 desktop tests passed (30.4 s), four phone tests passed
  (22.3 s), one worker and one shard per sequential run. Search replacement,
  clear/hide cancellation, pagination/filter retention and stale responses passed.
- Targeted ESLint, i18n ratchet and E2E sleep ratchet passed. Typecheck passed
  with `NODE_OPTIONS=--max-old-space-size=4096`; the default 2 GB heap exhausted
  memory. An interrupted earlier run was not counted as success.
- The first phone hide test froze the native action's animation frame; advancing
  32 ms before asserting closure corrected the test sequencing. No product
  change was needed. The final four-test phone run passed.
- Desktop and phone screenshots inspected: existing layout and native search
  interactions preserved. Public docs need no update: navigation, terminology,
  API and operator contracts are unchanged.

### Bounded before/after measurement

100 seeded tasks, three repetitions of 20 warmed queries per device and phase.
Values below are medians in milliseconds; fetch and render costs were measured
separately, and driver action duration was recorded separately from browser time.

| Device | Phase | Input to request | Request to response | Response to rows | Input to rows |
| --- | --- | ---: | ---: | ---: | ---: |
| Desktop | Before | 636.40 | 4.90 | 28.25 | 670.20 |
| Desktop | After | 321.15 | 5.15 | 27.75 | 352.85 |
| Phone | Before | 635.80 | 5.55 | 22.05 | 664.10 |
| Phone | After | 311.65 | 5.40 | 23.00 | 343.50 |

Observed median input-to-results reduction: 317.35 ms desktop, 320.60 ms phone.
Input-to-results p95 was 675.1 to 362.3 ms desktop and 674.7 to 353.6 ms phone.
These are bounded local measurements, not a scaling claim or a universal gain.
Median component durations need not sum to the median end-to-end duration.

Raw evidence stays in this workspace under `.tmp/task-search-debounce/`:
`red.txt`, `green-final.txt`, `typecheck-final.txt`, managed E2E logs,
`measurement-summary.json`, and `measurement/{before,after}/{chromium,mobile-chrome}`
with source/build/browser identity, served-asset hashes, samples and screenshots.
Before-source bytes matched HEAD; served JS matched each fresh build. After-source
hashes matched the final production edit. Disposable timing probes were removed
from the product tree and retained under `probe-snapshot/` for repeatability.
No byte divergence was observed. Final catalog/spec lint, work-order coverage and whitespace checks passed.
No commit, push or PR was performed.

## Risks

Clearing removes the former page wait as well; test pending callback cancellation.
Microtask page reset can produce a transient later-page read, so response
ordering must stay intact. Broad component mocks can conceal a surviving timer.
Fresh build/browser provenance is required before attributing any timing change.

## Task context and handoff

Own task `4d8fc57a-b3c1-433e-8281-832c6a8896dd`, session
`f5e3000c-a992-41af-9317-74b704d454d8`; parent
`bcd8fa7d-1701-4721-8665-5cdd4a58344c`, primary parent session
`24f280c8-6523-474c-aff3-369b603e25eb`. The requested planning checkpoint preceded the later explicit implementation
authorization. No completion signal, title mutation, persistent plan write
or parent message is inferred from file creation. Kandev tools are absent from
the available catalog, so requested title/parent-message operations could not
be performed. Final conversation handoff reports the completed implementation and checks.
