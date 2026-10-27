---
created: 2026-10-26
status: in_progress
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-001
  - REQ-UI-MOBILE-LIST-VIEWS-002
  - REQ-UI-MOBILE-LIST-VIEWS-003
  - REQ-UI-MOBILE-LIST-VIEWS-004
  - REQ-TASKS-WORKSPACE-SEARCH-001
system_design:
  - ../../specs/ui/system-design/mobile-list-saved-views.md
  - ../../specs/tasks/system-design/workspace-search.md
legacy_specs: []
---

# Implementation plan: Mobile List layout

## Overview

Phone List renders the user's existing desktop sidebar views. The default
screen shows saved-view tabs and task rows. A header search icon reveals
search and temporary result filters. The top-left List control owns saved-view editing.

Implement the query adapter first, then the phone list, controls/editor, and
header/menu correction. The final work order requires rendered comparison with
these previews and real-iPhone evidence for the blurred Default label.

Implementation is authorized by the user's 2026-10-26 request. Work orders are
executed sequentially; update each status and result as its checks complete.

## Settled intent

- Use actual workspace sidebar saved views, not a copied mobile collection.
- Preserve the full Running, Red, Orange, then Last activity sort chain.
- Keep current mobile control heights and density. Do not enlarge controls to 44px.
- Replace fixed archive tabs with the user's saved-view tabs.
- Hide search and Filters until the header search icon opens them.
- Keep temporary result filters separate from portable view changes.
- Open saved-view editing through top-left List options, never the hamburger.
- Correct menu padding and verify the actual iPhone workspace-label blur.
- Verify visual composition against the previews before marking work complete.

## Scope

### In scope

- Phone List data/view integration, rows, tabs, and bounded pagination.
- Request-only search support and immutable result-scope composition.
- Header search toggle, result-filter drawer, and shared saved-view editor.
- Menu padding, collapsed contextual Tasks, and no duplicate editing entry.
- Header diagnosis and an evidence-backed correction.
- Compatibility for explicit links, archive navigation, and eligible plugin facets.
- Focused desktop/mobile regression checks, rendered evidence, and public guidance.

### Out of scope

- Replacing desktop standalone List preferences or Threads saved views.
- A new saved-view database, user-settings migration, or task lifecycle changes.
- New sort semantics, new dependencies, app-wide sizing changes, or a general redesign.
- A font-only substitute for the reported blur correction.

## Technical approach

### 1. Query and result ownership

Extend `SidebarTaskViewQuery` / `SidebarTaskQuery` with an optional request-only
`query` string. Apply the existing workspace literal search fields before
ranking/counting/page selection. Leave saved `titleMatch` semantics unchanged.

Add optional consumer scope to `useWorkspaceSidebarTasks`, `useSidebarTaskPage`,
and `useSidebarPageContext`. The default caller behavior stays compatible.
Include refinements and search in all query/cache identities. Nonempty workspace
search uses the bounded server path; empty queries retain covered local paging.

Add a pure mobile result-query helper and controller. Keep temporary clauses,
link overrides, and text admission outside `SidebarViewDraft`. Explicit link
scope replaces only its owned dimensions for that request. Archive navigation
retains its all-scope destination without saving a definition.

### 2. Primary phone List

Split the responsive consumer before query effects mount in `TasksPageClient`.
The wider client retains existing standalone List behavior. The phone client
reads the shared selected view and uses existing task row/action/pagination
builders. Rendering and data use the same resolved view.

Saved-view tabs use existing names, order, selection, and fallback. Task rows
follow the view's presentation. Secondary row actions use a visible More target.
A grouped view still shows the shared group/tree hierarchy; UI-01 illustrates
an ungrouped view. Keep pins, child order, and complete-tree ranking.

### 3. Controls and editor

Add the search icon beside `AppNavSheet` in `KanbanHeaderMobile`. The icon
controls one search/Filters row below the tabs. It has an accessible expanded
state and retains the existing clear/cancel behavior on close.

Compose the shared view editor in `MobileMenuSheet` when List is active.
Extract its controller from `SidebarFilterPopover` rather than copying journals
or mutation guards. Remove the independent List sort/group controls from normal
phone List options. Keep existing desktop and task-picker wrappers.

Retain the immediate automatic-color settings owner. Save controls cover only
view drafts. Optional plugin facet presentation remains a clearly labelled
result override inside the left sheet, not a second saved-view model.

### 4. Menu and header

`AppNavSheet` / `AppNavSurface` keep the workspace picker above one navigation
scroller. Correct its top inset and use consistent section spacing. Contextual
Tasks starts collapsed on List and exposes selection without an editing shortcut.
Other task-picker presentations retain their controllers and editing behavior.

Diagnose the label blur on the affected iPhone before selecting CSS changes.
Use an opaque viewport-attached phone header and exactly-once safe-area spacing
as a bounded experiment, not as an assumed fix. Preserve intentional drawer
backdrops. An unresolved device result blocks closure of that fix.

### Compatibility matrix

| Consumer | Data/identity | Result | Required evidence |
| --- | --- | --- | --- |
| Ordinary phone List | Existing workspace sidebar view | Complete shared view semantics | Exact chain and cross-page results |
| Phone explicit List link/archive command | Local request overrides over shared presentation | Requested scope, no saved-definition writes | Reload/history, scope indicator, reset |
| Phone plugin facet selection | Existing plugin facet values, page-local override | Existing facet behavior, separate from saved definition | Updated mobile facet scenario |
| Desktop/tablet standalone List | Existing scalar List preferences | Existing controls, links, and saves | Desktop List regression |
| Desktop sidebar / phone task picker | Existing shared view controller | Same persisted identities and query defaults | Workspace and sort regression |
| Threads | Separate Threads view collection | Existing layout and editing | Existing title/menu navigation checks |
| Unknown plugin facet / inaccessible workspace | Existing unavailable/access guards | Visible unavailable/reset or access error | No false empty result or foreign view |

## ASCII UI preview

These are the authoritative implementation previews. UI labels remain localized.
ASCII spacing is illustrative. Control order, surface ownership, hidden/expanded
states, and scroll regions are required. Heights use the pre-change baseline.

`[S]` means search icon. `[=]` means hamburger. `[...]` means a More menu.
Names such as Archived and My view represent actual saved views, not new built-ins.

### UI-00: Current phone List, before

Entry: ordinary `/tasks`. Evidence: supplied screenshots and isolated WebKit check.

```text
+--------------------------------------+
| Default                              |
| List v                           [=] |
+--------------------------------------+
| [Active] [Archived] [All]             |
| [ Search tasks...        ] [Filters] |
|                                      |
| Grouped task rows                    |
+--------------------------------------+
```

The new ordinary entry must not retain the fixed archive toolbar or permanent search row.

### UI-01: Phone List, search closed

Entry: ordinary `/tasks`, selected saved view, no result refinements.
AC: 001.1-.7, 002.1, 004.1-.2, 004.6.

```text
+--------------------------------------+
| Default                              |
| List v                       [S] [=] |
+--------------------------------------+
| [All tasks] [Archived] [My view]      |
+--------------------------------------+
|                                      |
| | Running task title       2m  [...] |
| | Another task title      12m  [...] |
| | Task title               1h  [...] |
|                                      |
| [Previous]             [Next]        |
|                                [+]   |
+--------------------------------------+
```

Header and tab row do not scroll vertically. Tabs own contained horizontal
scrolling when needed. The task body owns vertical scrolling. Pagination appears
only when required; the FAB clears the last row and bottom safe area.

### UI-02: Phone List, search open

Entry: tap `[S]`. AC: 002.1-.6, 004.2, 004.6.

```text
+--------------------------------------+
| Default                              |
| List v                       [S] [=] |
+--------------------------------------+
| [All tasks] [Archived] [My view]      |
| [ Search tasks...        ] [Filters] |
| Status: Running [x]                   |
+--------------------------------------+
|                                      |
| Matching task rows                   |
+--------------------------------------+
```

The summary appears only for applied filters. Search focuses on open. A second
search-icon tap clears refinements and restores UI-01. No new saved view is written.

### UI-03: Temporary result-filter drawer

Entry: Filters in UI-02. AC: 002.2, 002.5, 004.3, 004.6.

```text
+--------------------------------------+
| Filter results                   [x] |
|                                      |
| Status                       All   v |
| Workflow                     All   v |
| Repository                   All   v |
|                                      |
| [Clear]                      [Apply] |
|              safe area               |
+--------------------------------------+
```

This inset drawer fits its content and grows only within the viewport limit.
Long options scroll internally. It refines saved scope; conflicting saved filters
remain effective. Close discards unapplied changes. It cannot edit a saved definition.

### UI-04: View options, sections collapsed

Entry: top-left List v. AC: 003.1-.3, 003.5-.7, 004.2-.3.

```text
+--------------------------------------+
| View options                     [x] |
+--------------------------------------+
| [Kanban] [Threads] [List]             |
|                                      |
| Saved view: All tasks          [...] |
| Filters                           > |
| Sort                              > |
| Group by                          > |
| Task row                          > |
| Automatic colors                  > |
|                                      |
+--------------------------------------+
| [Discard] [Save as new]       [Save]  |
|              safe area               |
+--------------------------------------+
```

The title is fixed; the settings body owns one vertical scroller. The draft
footer is fixed and appears only for a view draft. Section summaries can wrap.
No empty action wrapper creates a gap above the layout selector. View creation,
rename, and delete remain inside the view actions, with existing confirmation rules.

### UI-05: View options, sort expanded

Entry: Sort in UI-04. AC: 001.3, 003.3-.5, 004.2-.3, 004.6.

```text
+--------------------------------------+
| View options                     [x] |
+--------------------------------------+
| [Kanban] [Threads] [List]             |
| Saved view: All tasks          [...] |
|                                      |
| Sort                           + Add |
| :: 1 Running                   [...] |
|      Running first                   |
|                                      |
| :: 2 Color: Red                [...] |
|      Matching first                  |
|                                      |
| :: 3 Color: Orange             [...] |
|      Matching first                  |
|                                      |
| :: 4 Last activity             [...] |
|      Newest first                    |
|                                      |
| Group by                   None    > |
| Task row                   Summary > |
| Automatic colors           Off     > |
+--------------------------------------+
| [Discard] [Save as new]       [Save]  |
+--------------------------------------+
```

Each rule is one stacked card with the existing fields and visible drag grip.
More offers Move up/down and existing applicable actions. Card fields remain
editable without squeezing several desktop selectors into one phone row.

### UI-06: App navigation menu

Entry: hamburger from List. AC: 003.2, 004.3-.6.

```text
+--------------------------------------+
| Menu                             [x] |
|                                      |
| [ Default                        v ] |
+--------------------------------------+
| [ + New Task                       ] |
| Home                                 |
| [ Quick Chat ] [ Terminal ]          |
|                                      |
| Canvases                           > |
| Automations                        > |
| Integrations                       > |
| Eligible plugin tools/status         |
|                                      |
| Tasks                              > |
|                                      |
| Utilities                            |
| Settings                             |
| Status                               |
| Customize sidebar                    |
| Theme                                |
|              safe area               |
+--------------------------------------+
```

Header and workspace picker stay fixed. One scroller owns the remaining menu.
Honor configured visibility and relative tool order. Tasks starts collapsed;
its expanded rows remain selectable. No Edit view action or filter editor appears here.

### UI-07: Wider consumers, compatibility

Entry: viewport at or above 768px. AC: 001.7, 003.6.

```text
+--------------------+------------------------------------------+
| Desktop sidebar    | Standalone List                         |
| Tasks [All tasks v]| Existing archive/search/filter controls |
| [View options]     | Existing List sort/group/details         |
| Shared view rows   | Existing standalone rows                 |
+--------------------+------------------------------------------+
```

Sidebar edits and phone List edits share saved definitions. Standalone desktop
List retains its own preferences. Threads stays independent. Narrow fine-pointer
widths below 768px still use the phone composition.

### UI-08: Task-body feedback

Entry: loading, successful empty result, or failed read. AC: 001.5, 004.6.

```text
| Saved-view tabs remain usable        |
+--------------------------------------+
| Loading tasks...                     |
|                                      |
| No matching tasks. [Clear filters]    |
|                                      |
| Tasks could not load. [Retry]        |
+--------------------------------------+
```

These are separate states, never simultaneous messages. A background refresh
retains eligible rows. Access denial clears them. Feedback stays inside the task body.

### UI-09: Explicit link or plugin compatibility state

Entry: a filtered List link, archive command, or explicit plugin facet choice.
AC: 002.7-.8, 003.7.

```text
| [All tasks] [Archived] [My view]      |
| [ Search tasks...        ] [Filters] |
| Linked result scope          [Clear]|
+--------------------------------------+
| Requested result rows                |
```

The left options sheet identifies overrides separately:

```text
| Saved view: All tasks          [...] |
| Shared saved-view sections           |
| Additional List options            > |
| Result sort: Fixture tag             |
| Result group: Fixture tag            |
| [Reset result overrides]             |
```

The optional disclosure renders only for a relevant compatibility capability.
It never becomes a second default toolbar or a portable sidebar-view field.

## Tests

| Criteria | Targeted evidence |
| --- | --- |
| 001.1-.7 | `mobile-tasks-page-client.test.tsx`: shared selection, fallback, scoped queries, live rows, wider-branch isolation |
| 001.3, 001.5 | Existing sidebar query/sort tests plus `mobile-list-query.test.ts`: full chain, colors, identity, page reset |
| 002.1-.6 | `mobile-list-query.test.ts`, `mobile-list-toolbar.test.tsx`: refinement composition, cancellation, no writes, sheet draft |
| 002.3 | Planned `TestSidebarTaskPageWorkspaceSearch` and PostgreSQL counterpart: fields, punctuation, count/page consistency |
| 002.7-.8 | Route adapter unit tests and existing mobile archive/filter/facet scenarios |
| 003.1-.7 | Shared editor/controller tests and `mobile-menu-sheet.test.tsx`: entry, save/discard, stale workspace, immediate colors |
| 004.1 | Real-iPhone Safari/PWA before/after evidence; computed CSS alone cannot pass |
| 004.2-.6 | Header/nav component tests plus screenshot and geometry evidence in Task 05 |

New test paths above are planned. Work orders provide full paths and exact commands.

## E2E tests

All commands run from the repository root through an explicit `pnpm --dir`.
Use the managed runner so tests use a fresh Go-served production build.

| Flow | File/project | Criteria |
| --- | --- | --- |
| Shared view selection, full chain, cross-page result | New `tests/task/mobile-list-saved-views.spec.ts`, mobile-chrome | 001.1-.7 |
| Search toggle, refinement isolation, left editor, CRUD/save/discard | Same new file, mobile-chrome | 002.1-.6, 003.1-.7 |
| Existing archive links, search, facets | Existing mobile List specs and plugin facet spec | 002.3, 002.7-.8 |
| Preview screenshots and geometry | New `tests/task/mobile-list-preview-conformance.spec.ts`, mobile-chrome | 004.2-.6 and UI-01 through UI-09 |
| Desktop List/sidebar parity | Existing `task-list-filters`, workspace-sidebar and sort specs, chromium | 001.3, 001.7 |
| Header/menu integrations | Existing mobile topbar/menu hierarchy/navigation specs | 003.2, 004.3-.6 |

## Work orders

Execute sequentially because each work order depends on the prior result.

- [x] [Task 01: Shared query and result adapter](task-01-query-adapter.md) (complete)
- [x] [Task 02: Phone List saved views](task-02-phone-list.md) (complete)
- [x] [Task 03: Search and left view options](task-03-controls-and-editor.md) (complete)
- [ ] [Task 04: Header and menu geometry](task-04-header-and-menu.md)
- [ ] [Task 05: Preview conformance and documentation](task-05-preview-conformance.md)

## Companion records

The following packages describe implemented contracts, not new work orders.
Do not reopen their completed statuses for this feature.

- `workspace-sidebar-task-views`: existing view identity and settings journal.
- `sidebar-query-memory`: accepted records, coverage, bounds, and cache ownership.
- `sidebar-running-first-activity-sort` and `sidebar-task-wide-running-rank`: ordering semantics.
- `unified-mobile-navigation`: shared app menu and controller lifetime.
- `task-list-archive-filters`: archive links and explicit scope.
- `task-list-workflow-step-grouping`: wider List grouping and legacy aliases.

The linked requirement/design revision notes identify this package as the new
phone-specific intent. Existing tests migrate to the new entry points; their
behavioral coverage cannot disappear solely because selectors changed.

## Required visual acceptance

Task 05 is the requested verify step. It is specific to these previews, not a
broad repository audit. A test pass alone cannot complete it.

For every applicable preview, capture the matching UI in a fresh production
build. Open each image and compare control order, hierarchy, density, and spacing.
Record actual screenshot paths and observed differences in `verification.md`.
Never mark a row matched from selectors or class strings alone.

Required matrix:

- 393x852 phone, light and dark: UI-01 through UI-06 and UI-08/UI-09.
- 360x800 phone with long workspace/view names and many tabs: containment and reachability.
- 767x900 phone, 768x900 coarse-pointer tablet, and 1024x900 fine-pointer desktop: breakpoint composition.
- 393x852 fine pointer: same phone composition and current heights.
- Real iPhone Safari and installed PWA: Default label sharpness before and after
  scrolling, options/menu close, orientation change, and cold reload.

Record the pre-change control-height baseline before implementation. Require
unchanged heights within 1 CSS pixel at the same root font and scale. Check
actual card/row bounds, hit-test centers, no overlap, no document overflow,
internal scrolling, safe-area clearance, and focus return.

A failed image comparison, absent iPhone evidence, unresolved blur, or unexplained
preview drift keeps the relevant work order pending/in_progress. Report an
unavailable device as unverified; do not silently waive the gate. Resolve UI
mismatches before handoff, or update the design with explicit user direction.

## Verification results

Planning checks completed on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed, 404 decisions and 1742 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Package relative links: passed for all ten new package files.
- Embedded work-order previews: matched their authoritative plan sections.
- Local PR-documentation reference preflight: covered all five work orders with zero errors.

Implementation and rendered-browser checks ran on 2026-10-27. The affected
mobile suite passed 62/62 tests, and the wider desktop/sidebar/Threads suite
passed 14/14 tests. The current screenshots and geometry are recorded in
`verification.md`. Task 03 is complete; Tasks 04 and 05 remain in progress only
because real-iPhone Safari and installed-PWA evidence for the Default label is
unavailable in this environment. The headless run does not waive that gate.

Code-review remediation on 2026-10-27 fixed archive result projection and
coverage, effective link group/sort presentation, responsive dialog retention,
and settled search-clear geometry. The targeted hook/component tests and 10
phone E2E regressions pass. See the review-remediation notes in Tasks 01-03 and
the behavioral evidence in `verification.md`. Real-iPhone Safari/PWA validation
of the Default label remains pending.

The subsequent PR E2E run `40472095546` reported three retry-only flakes in
shared LSP, pause/resume, and mobile workflow-target tests. Their focused fixes
passed three local repetitions each with retries disabled. A fresh PR run must
validate the pushed fixes; the device acceptance gate remains pending.

PR run `40476223800` then reported first-attempt failures in workflow import
refresh and turn-change history rendering. Both tests now wait for the page
response that supplies their asserted state; each passed three targeted local
runs with retries disabled. The next PR run must verify these synchronization
fixes. The real-iPhone Safari/PWA gate remains pending.

PR run `40480462861` reached the 45-minute wait deadline with 86 checks passed,
11 skipped, three E2E shard leaves failed, and backend plus E2E aggregate checks
pending. The mobile Stats retry route now stays failed until explicit recovery;
the terminal and storage test fixtures now use an available `mock-agent` /
`mock-fast` profile. Each affected scenario passed twice in focused managed
runs with retries disabled; both terminal shell scroll cases passed in those
repetitions. Current `main` was merged locally to resolve two locale
conflicts. A fresh PR run after pushing the merge and fixes is required. The
actual iPhone Safari and installed-PWA Default-label blur check remains pending.

## Package reference preflight

Run from the repository root. The existing validator receives a representative
runtime path so it checks delivery references rather than exempting a docs-only
diff. This does not claim that the runtime file changed.

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/mobile-list-layout';
const files = fs.readdirSync(root).filter(x => x.endsWith('.md'))
  .map(x => `${root}/${x}`);
const contracts = [
  'docs/specs/ui/requirements/mobile-list-saved-views.md',
  'docs/specs/ui/system-design/mobile-list-saved-views.md',
  'docs/specs/tasks/requirements/workspace-search.md',
  'docs/specs/tasks/system-design/workspace-search.md',
];
const fileContents = Object.fromEntries([...files, ...contracts]
  .map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({
  changedFiles: ['apps/web/app/tasks/tasks-page-client.tsx', ...files, ...contracts],
  fileContents,
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Risks

- Native iOS browser chrome is absent from headless WebKit. The blur correction needs real-device evidence.
- Saved-view drafts are portable. Result-filter state must never enter their mutation journal.
- Adding query refinements to only the server path can break covered local views and cache identity.
- Legacy links and plugin facets require explicit adapters, not silent loss or view migration.
- Reusing shared editor components can expose duplicate saves or change immediate automatic-color behavior.
- The current screenshot density intentionally overrides generic enlargement guidance on these surfaces.

## Public documentation

This design turn changes internal documents only. Task 05 updates
`docs/public/mobile-remote-access.md` and the List/archive section of
`docs/public/tasks-and-workflows.md` after the behavior exists. Both are how-to
content. Keep desktop instructions accurate and describe temporary versus saved
filters explicitly. Update the screenshot catalog only when actual media changes.
