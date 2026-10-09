---
created: 2026-10-05
status: implemented
requirements:
  - REQ-UI-SIDEBAR-ARCHIVED-FILTER-002
system_design:
  - ../../specs/ui/system-design/sidebar-archived-filter.md
legacy_specs: []
---

# Implementation Plan: Sidebar Collapse Continuity

## Overview

Keep the task list visible when repository groups collapse while a server page
refreshes. One sequential work order owns the controller fix, safety regressions,
desktop and phone browser evidence, and delivery documentation. The user's
request authorizes implementation, PR review handling, and merge after the
repository's design-package handoff and later execution request. No delegation
is authorized or needed.

## Evidence and requirement conformance

On the uncovered/server-paged path, `collapsed_group_keys` contributes to
`useSidebarPageContext`'s full `viewKey`. A new collapse state has no matching
cache page; `sidebarResponseState` hides the accepted response and
`sidebarLoadingState` reports initial loading. `TaskSwitcher` consequently renders
`TaskSwitcherSkeleton` for the whole list.

A temporary hook reproduction and the existing cache-reuse regression both
passed during diagnosis: first collapse changed the key, issued a read, and
returned `response: null` with `isLoading: true`; returning to the cached state
preserved its response. The temporary test was removed. Those tests establish
the defect, not completion of the proposed repair.

The relevant context, loader, coverage, and cache code is identical before and
after navigation PR #4063 (`dd7dfa816`). Earlier PR #4026 reused exact recently
visited first pages; #4062 enabled complete local inventory projection. Neither
covers a new disclosure identity on an uncovered view. Archive-inclusive views
cannot use complete active-inventory coverage. Diagnosis did not inspect the
user's exact live-browser filter state. Isolated browser fixtures reproduce the
same failure on an archive-inclusive server-paged view.

Existing criteria 002.7, .15-.18, and .21-.24 constrain the repair. Criterion
002.25 defines the missing disclosure-only exception; .15 links that exception
instead of silently extending reuse to unrelated unvisited views.

## Scope

### In scope

- Same-view disclosure continuity for groups and subtasks in the shared loader.
- Existing response ownership, deletion reconciliation, access and context fences.
- Page-one replacement and retry, rapid changes, transient errors, and empty
  visible task rows caused intentionally by collapse.
- Focused hook and desktop/phone Playwright coverage with held HTTP responses.

### Out of scope

Backend query changes, all-task fetching, prefetch, cache budget changes,
persisted settings changes, sidebar redesign, new copy, and changes to the
user's live instance. Existing completed packages remain historical records;
this work order records the new repair.

## Technical approach

Preserve `useSidebarPageContext.viewKey` exactly. Add a content identity that
excludes only collapse fields and compare it with the existing accepted display identity in
`useSidebarTaskPage`. On a disclosure-only cache miss, use the bounded accepted
page as transitional display while the full-key page-one request runs. The
existing `TaskSwitcher` applies current disclosure preferences immediately.
Keep provisional display distinct from authoritative current-query totals,
prevent navigation with old bounds, and never synthesize unseen expansion rows.
Use the existing error status and page-one retry. Preserve strict response
identity, deletion handling, workspace/account/access fences, local coverage,
and cache ownership. Extract a small helper only when lint limits require it.

No ADR is needed: the requirement and design record this local display repair;
existing bounded query, record ownership, and cache-retention decisions stand.

## ASCII UI preview

UI-01: Desktop Tasks, repository collapse with a held replacement response.

```text
BEFORE                           AFTER
TASKS [All v] [Filters]           TASKS [All v] [Filters]
[task loading placeholders]       > Repository A       (1)
                                 v Repository B       (1)
                                   Task B
```

UI-02: Phone Tasks picker, same disclosure inside its existing drawer.

```text
+----------------------------------+
| Tasks                    [Close] | fixed header
| [All v]                [Filters] |
| > Repository A               (1) | existing body scroller
| v Repository B               (1) |
|   Task B                         |
+----------------------------------+
```

These previews specify retained headings and unaffected rows, not pixel
spacing. Refresh remains screen-reader-only. Group collapse hides its rows
immediately, including when every group is collapsed. Existing controls and
localized copy remain in use. UI-01 and UI-02 map to 002.25; phone also maps to
002.9. A later-page display remains transitional until page-one settlement.

### Mobile design contract

Entry points: task-title Tasks picker and app-navigation Tasks outlet.
Exemplars: `SessionTaskSwitcherSheet`, `MobileTaskList`, and `GroupHeader`.
Keep their hierarchy, drawer/menu surfaces, body/menu scrollers, focus return,
dynamic viewport containment, and 44px phone/coarse-pointer controls. Primary
action remains selecting a task to navigate. Collapse keeps the surface open.
The shared data fix must not persist a viewport-specific preference or move the
open conversation. Desktop retains its existing sidebar scroll owner.

## Tests

The primary red test extends
`hooks/domains/kanban/use-sidebar-task-page.disclosure.test.tsx`:
`keeps eligible rows while an uncached repository collapse loads`.
Hold the network boundary and assert non-null safe display, no initial loading,
page-one replacement, and final settlement. Cover expansion, all groups
collapsed, subtask disclosure, later-page retry, rapid changes, deletion during
refresh, and negative filter/sort/group/locale/preferences/workspace/account
reuse cases. Existing access-denial and local-coverage tests remain controls.

## E2E tests

Create `e2e/tests/task/sidebar-collapse-loading.spec.ts` (chromium) and
`mobile-sidebar-collapse-loading.spec.ts` (mobile-chrome), with shared fixture
support in `sidebar-collapse-loading-fixtures.ts`. Seed at least two real
repository groups and archived tasks so resident active coverage cannot hide
the server path. Gate collapse query responses instead of sleeping. Assert
headers and unaffected task rows remain while the target group hides, expansion
settles, and the open conversation remains selected. Assert phone containment,
touch reachability, and no document horizontal overflow. Include phone
app-navigation disclosure coverage when it uses a distinct rendered outlet.
Use `prCapture` for fresh desktop/phone screenshots while the response is held;
publish only sanitized synthetic data after implementation checks pass.

## Work orders

- [x] [Task 01: Preserve sidebar disclosure continuity](task-01-preserve-disclosure.md)

## Verification results

Implementation completed after the user's later execution request.

- Hook/cache/paging/error suite: 73 tests passed. The disclosure test extraction
  also passed all three affected hook suites (37 tests).
- Web typecheck and targeted ESLint with zero warnings: passed.
- PR remediation: four E2E request-cleanup unit tests passed after reproducing
  an assertion masked by a response timeout. The work order records the
  isolated helper, desktop/phone reruns, and runner cancellation evidence.
- Managed Chromium regression: one test passed; original-loader red reproduced
  missing headings while the response was held.
- Managed mobile-chrome regression: one test passed, covering both phone entry
  points and unchanged conversation selection.
- Public docs validator: 62 tests and 47 pages passed.
- Catalog validation and full specification lint: passed.
- Diff whitespace checks: passed.

The public task-browsing guide now explains disclosure continuity. The pagination
requirement and system design describe the implemented invariant. Fresh desktop,
phone picker, and phone navigation screenshots show the held refresh with
synthetic data; they are published separately from the merge branch. Exact
commands and delivery gates are recorded in the [work order](task-01-preserve-disclosure.md).

## Risks

- Ignoring collapse fields in the request/cache key would corrupt paging reuse.
- Marking retained rows as a complete new view could fabricate counts or enable
  navigation against outdated bounds.
- The empty-provisional loading predicate currently protects deletion recovery;
  intentional collapse must be distinguished without weakening that behavior.
- Retry from a retained later page must still request page 1.
- Whole-list continuity must never restore deleted tasks or bypass hard context
  barriers, including a rapid collapse followed by a filter/account change.
