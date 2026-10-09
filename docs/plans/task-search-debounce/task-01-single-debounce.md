---
id: "01-single-debounce"
title: "Keep one input-owned Task List search debounce"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001
  - REQ-UI-LIST-STEP-GROUPING-001
  - REQ-UI-MOBILE-MENU-005
acceptance_criteria:
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.1
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.2
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.13
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.14
  - AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-001.15
  - AC-UI-LIST-STEP-GROUPING-001.6
  - AC-UI-MOBILE-MENU-005.2
  - AC-UI-MOBILE-MENU-005.3
system_design:
  - ../../specs/ui/system-design/task-listing-display-preferences.md
  - ../../specs/ui/system-design/task-list-workflow-step-grouping.md
  - ../../specs/ui/system-design/unified-mobile-navigation.md
---

# Task 01: Keep One Input-Owned Task List Search Debounce

## Summary

Remove the List page's extra query timer while retaining `TaskSearchInput`'s
existing 300 ms trailing callback. Prove admission after one logical window and
preserve clear, filter/pagination, latest-input and existing reply protection on
desktop/tablet/phone. The later explicit implementation request authorized
execution; this work order is complete.

## In scope

- Sole production correction: `apps/web/app/tasks/tasks-page-client.tsx`.
- Real-page fake-timer regression and request/clear/pagination/freshness controls.
- Native desktop/phone E2E checks and the plan's bounded before/after timing probe.
- Update this work order and plan with exact results; preserve others' edits.

## Out of scope

Shared input/hook changes, new raw-query freshness contracts, backend/API/store
or virtualizer changes, UI redesign, dependencies/lockfiles, other task search
surfaces, audit/sibling artifacts, commits/push/PR and additional workers/tasks.

## Acceptance

1. The real List input updates displayed text immediately and issues its latest
   typed query after one 300 ms trailing window on page zero. Rapid edits replace
   queued work; clear/hide cancels it. No second query timer exists in the page.
2. Active filters/sort/archive/page size remain effective; search resets to page
   one. Existing newer-started-request and workspace fences continue protecting
   rows, totals, loading, errors and finalizers. Desktop and phone flows pass.
3. RED and GREEN are demonstrated by the same production-wiring regression;
   targeted checks and raw measurement identities/results are recorded without
   claiming debounce-only wall time or an unmeasured speedup.

## Source correction

Investigation HEAD/remote main: `330e02a47808c11ca315ae30456fcce7f4806db5`.
Before resuming, reconcile any moved base with the source trace and these specs.
Source and consumer inventory is in [the plan](plan.md#technical-approach).

Remove the page's `useDebounce` import and
`useDebounce(viewState.searchQuery, 300)`. Rename internal query plumbing to
`searchQuery` and feed `viewState.searchQuery` directly to `useTaskOperations`
and `useTasksPageEffects`. Preserve the exported fetch policy's existing field
with `debouncedQuery: searchQuery`; adapt the loading predicate to the admitted
query. Keep clear, query replacement and unmount timer cleanup in the unchanged
input. Keep existing pagination effects, request-sequence checks, silent/explicit
refresh behavior and hydrated initial-fetch skip.

Do not change `TaskSearchInput` globally or pass `debounceMs={0}` through shared
surfaces. Do not extract a new debounce abstraction. Existing input ownership
is sufficient; `searchQuery` in page state is already admitted, whereas
`TaskSearchInput.localValue` is the immediate draft.

## Regression and TDD sequence

1. Add `app/tasks/tasks-page-search-debounce.test.tsx` using real page query/fetch
   effects and real `TaskSearchInput`. Retain real `TasksPageContent` and phone
   `MobileSearchBar`; unrelated hooks, list markup and header chrome can be
   mocked. A desktop/tablet header double renders the actual input with received
   props. The real input/default timer and real page debounce must remain live.
   Reuse StateProvider setup from `tasks-page-client.mr-hydration.test.tsx`, but
   do not reuse its mocked `useDebounce`. Run RED before production edits.
2. Main test name: `admits the last input after one debounce window`. Render a
   hydrated page-zero baseline, reset initial API calls, type Alpha and assert
   displayed text immediately. In async `act`, advance 299 ms and flush effects;
   no matching request. In a separate async `act`, advance 1 ms and flush effects;
   expect exactly one matching Alpha request. Before correction, this assertion
   fails with zero requests because the page timer has only just been scheduled.
   Do not advance all timers at once or fake an independent two-timer harness.
3. Add rapid A/AB/ABC replacement, clear while queued/after settled results,
   typed-empty input, phone hide/unmount before callback, page-two reset and
   page-size/filter/sort/archive retention controls. Flush beyond the former
   two windows to prove cancelled text stays cancelled. Defer API promises to
   settle B before A; late A success/error/finalizer and previous-workspace
   response cannot overwrite B. Keep the current started-request boundary;
   do not require retirement on every raw input event.
4. Make the minimal page correction, run GREEN and all focused commands below.
   Test page-zero request counts separately from later-page reset: a transient
   old-page read is permitted by the existing reset implementation; accepted
   final rows and pagination must be for page one.
5. Complete desktop/mobile E2E plus the measurement described below. Inspect
   screenshots, record every actual result/failure/blocker, and synchronize plan
   and work-order statuses only after all required checks pass.

## ASCII UI preview

Relevant excerpt from [combined preview](plan.md#ascii-ui-preview), mapped to
listing AC .13-.15 and mobile AC .2/.3. No rendered structural change is planned.

```text
UI-01 desktop/tablet: [ Workspace | List ] [ Search: Alpha | X ]
                     [ Sort v ] [ Group v ] -> Alpha rows -> page 1
UI-02 phone:         [ Workspace | List v ] [ Menu ]
                     [ Search: Alpha                    X ]
                     Alpha rows -> page 1
```

Desktop uses existing header input; tablet also retains its menu input. Phone
uses List context -> View options -> Search tasks, focusing the existing inline
bar. Reuse `MobileSearchBar`/`MobileMenuSheet` geometry, menu dismissal, safe-area
clearance and scroll ownership. List body remains the content scroller. Phone
hide clears the query and unmounts its input. Example data/spacing are
illustrative; screenshot checks compare existing composition after the fix.

## E2E scenario matrix

| File/project | Required outcome |
| --- | --- |
| `e2e/tests/task/task-list.spec.ts`, chromium | Search from page two, final page one; clear restores matching scope; workflow/repository/archive/sort retained; existing grouping and pagination pass |
| New `e2e/tests/task/task-search-debounce.spec.ts`, chromium | Rapid A/B input admits final query; clear while queued; deferred A reply after accepted B cannot overwrite rows/total or current loading/error |
| `e2e/tests/task/mobile-task-list-search.spec.ts`, mobile-chrome | Tap native reveal/focus; rapid edit/clear; hide while queued and reopen empty; page-two search resets; existing display/grouping scenario retained; combination with native filters/sort/archive |

Arm HTTP observers before input and await matching query/page/filter responses
before asserting DOM. Use exact active input scoping, route latches for deferred
replies, and sanctioned negative observation windows. Do not use timing
thresholds as the debounce regression. Extend existing mobile tests rather than
adding a different device override. Capture and inspect desktop and configured
Pixel-5 phone images during focused runs. No new UI geometry contract is added.

## Verification

Run from repository root. Install once if this fresh checkout has not yet done
so. Go may require `/usr/local/go/bin` on PATH; inspect installed Chromium first
and install the matching version only if missing.

```sh
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run app/tasks/tasks-page-search-debounce.test.tsx app/tasks/tasks-page-fetch-policy.test.ts app/tasks/tasks-page-client.mr-hydration.test.tsx components/kanban/kanban-header-mobile.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 app/tasks/tasks-page-client.tsx app/tasks/tasks-page-search-debounce.test.tsx e2e/tests/task/task-list.spec.ts e2e/tests/task/task-search-debounce.spec.ts e2e/tests/task/mobile-task-list-search.spec.ts)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run e2e:sleep-ratchet)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium tests/task/task-list.spec.ts tests/task/task-search-debounce.spec.ts --list)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=mobile-chrome tests/task/mobile-task-list-search.spec.ts --list)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --shards 1 --project=chromium tests/task/task-list.spec.ts tests/task/task-search-debounce.spec.ts)
(cd apps/web && PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --shards 1 --project=mobile-chrome tests/task/mobile-task-list-search.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Confirm intended counts using `--list` before the run; record actual counts and
compare with the runner's first line. Do not pipe gates, override workers,
overlap suites, accept zero tests, or suppress freshness failures. The managed
runner builds fresh web/runtime/plugin artifacts and tears down disposable
fixtures. The new test files above were created and executed during the authorized
implementation phase.

Re-run the local coverage preflight retained at
`.tmp/task-search-debounce/validate-coverage.cjs` with
`node .tmp/task-search-debounce/validate-coverage.cjs`. The script reads the
canonical `.github/scripts/pr-docs.cjs` validator and this package/specs; its
prospective production path is explicitly synthetic, not an actual source edit.
If scratch evidence is unavailable in a later workspace, recreate the same
`validateCoverage` call using the work-order paths and their referenced specs.

## Before/after measurement

Use [the plan's full protocol](plan.md#measurement-protocol). Add only a
throwaway `e2e/tests/task/task-search-debounce-measurement.spec.ts`, with a
`measurement` test recording three 20-action warmed repetitions at 100 tasks.
The spec chooses inline mobile search when `testInfo.project.name` is
`mobile-chrome` and otherwise the header input. Because mobile discovery requires
`mobile-*.spec.ts`, name the phone variant
`mobile-task-search-debounce-measurement.spec.ts`; share temporary probe logic
outside permanent test files if needed. Both are removed after measurement.

Before changing production, run these exact managed commands; repeat after the
correction with a new phase label. Each project/spec performs all three
repetitions, using uniquely named disposable tasks and verifying each result.

```sh
(cd apps/web && KANDEV_SEARCH_MEASURE_PHASE=before PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --shards 1 --project=chromium tests/task/task-search-debounce-measurement.spec.ts)
(cd apps/web && KANDEV_SEARCH_MEASURE_PHASE=before PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --shards 1 --project=mobile-chrome tests/task/mobile-task-search-debounce-measurement.spec.ts)
```

Repeat the commands with `KANDEV_SEARCH_MEASURE_PHASE=after`. Retain JSON samples,
logs, source/build/browser/seed identity and screenshot readbacks under
`.tmp/task-search-debounce/measurement/<phase>/<project>/`. Capture in-browser
input/fetch/response/settled-row times and driver action duration separately.
Fake timers isolate intentional debounce; real input-to-request also includes
React scheduling. Stop on source/bundle mismatch and retain suspicious bytes.
Remove only the owned throwaway spec/helper files afterward, even on failure;
never remove another contributor's artifacts. No speedup target is asserted.

## Files likely touched

Production ownership:

- `apps/web/app/tasks/tasks-page-client.tsx`

Test ownership:

- `apps/web/app/tasks/tasks-page-search-debounce.test.tsx` (new)
- `apps/web/e2e/tests/task/task-list.spec.ts`
- `apps/web/e2e/tests/task/task-search-debounce.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-task-list-search.spec.ts`

Planning/result ownership:

- `docs/plans/task-search-debounce/plan.md`
- `docs/plans/task-search-debounce/task-01-single-debounce.md`
- Existing `docs/specs/ui/{requirements,system-design}/task-listing-display-preferences.md`
  clarification is already in this planning package; synchronize only if needed.

## Dependencies

None. No native delegation is authorized. You are not alone in the repository:
do not revert others' work or modify sibling audit/planning artifacts.

## Risks

Mocks that omit the real input conceal duplicate waiting. Page reset is
microtask-based and can transiently read the old page. Old reply finalizers must
not clear newer loading. Phone hide must cancel pending input on unmount.
Timing attribution requires verified fresh assets; a source trace alone cannot
prove wall-clock improvement.

## Parallelism

`sequential`

## Inputs

- [Listing requirement](../../specs/ui/requirements/task-listing-display-preferences.md),
  existing .1/.2 and clarified .13-.15 under requirement 001.
- [Listing design](../../specs/ui/system-design/task-listing-display-preferences.md#list-search-admission).
- [Grouping requirement](../../specs/ui/requirements/task-list-workflow-step-grouping.md), .6;
  [design](../../specs/ui/system-design/task-list-workflow-step-grouping.md).
- [Mobile requirement](../../specs/ui/requirements/unified-mobile-navigation.md), 005.2/.3;
  [design](../../specs/ui/system-design/unified-mobile-navigation.md).
- Root/scoped AGENTS, `/tdd`, `/e2e`, `/mobile-parity`, and `/planner-orchestration`.
- [Plan evidence and task context](plan.md#task-context-and-handoff), local
  `.tmp/task-search-debounce/source-identity.json` and `source-trace.txt`.

## Results

Completed after the explicit implementation request. Only `tasks-page-client.tsx`
changed in production: consume the input-admitted query directly and remove the
page's second timer. Shared input, store, fetch freshness and pagination contracts
remain intact.

The real-input/page fake-timer test failed at 300 ms before the edit on desktop
and phone, then passed after it. Four targeted suites passed 26 tests. Fresh
managed browser runs passed 11 desktop and four phone tests. Typecheck (4 GB
heap), focused lint and both ratchets passed. Native search, clear/hide, page reset,
filter retention and stale success/error/finalizer behavior were checked.

The bounded before/after probe observed median visible results 317.35 ms sooner
on desktop and 320.60 ms sooner on phone. See the manifest's
[verification results](plan.md#verification-results) for sample limits, component
timings, provenance, raw evidence and the corrected phone clock-test sequencing.
Screenshots were inspected; disposable probes were retained outside the product
tree. Changes are uncommitted; no push or PR was performed.

### PR remediation validation

The initial remote E2E failures and fixture-only scope are recorded in the
[manifest](plan.md#pr-ci-remediation-2026-10-09). Post-main integration, the four
search Vitest suites still pass 26 tests. Focused ESLint and Prettier passed for
the two repaired specs. Managed E2E reproduction used one worker and one shard,
with `--retries=0`; the three desktop leaf failures passed two repetitions each
and the mobile leaf passed three. Full repaired specs passed with `--retries=0`: 13 multi-session tests and two
mobile admission tests. Original search E2E gates passed again: 11 desktop and
four phone tests with retries disabled. Commands, run logs and policy evidence are retained under
`.tmp/task-search-debounce/fixup/`; fresh remote results remain a delivery gate.

```sh
PATH=/usr/local/go/bin:$PATH pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/session/multi-session-ux.spec.ts -- --retries=0
PATH=/usr/local/go/bin:$PATH pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-queue-admission-reliability.spec.ts -- --retries=0
```
