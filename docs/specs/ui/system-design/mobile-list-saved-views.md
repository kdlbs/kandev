---
status: current
system: ui
created: 2026-10-26
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-001
  - REQ-UI-MOBILE-LIST-VIEWS-002
  - REQ-UI-MOBILE-LIST-VIEWS-003
  - REQ-UI-MOBILE-LIST-VIEWS-004
owners:
  - kandev
---

# Mobile List saved views and controls design

## Boundary and mapping

UI owns the phone rendering of an existing sidebar view. This surface introduces
no new saved-view collection. Tasks retains query membership and literal search.
Existing user settings retain portable writes and workspace revision guards.

| Requirement | Design sections |
| --- | --- |
| REQ-UI-MOBILE-LIST-VIEWS-001 | Shared query, Phone composition, Compatibility |
| REQ-UI-MOBILE-LIST-VIEWS-002 | Result state, Search transport, Compatibility |
| REQ-UI-MOBILE-LIST-VIEWS-003 | Editor ownership, Menu integration |
| REQ-UI-MOBILE-LIST-VIEWS-004 | Geometry and density, Header diagnosis, Verification |

## Current source boundaries

- `app/tasks/tasks-page-client.tsx` owns standalone List query/preferences.
  `TasksListFilters` always renders archive modes and search, including on phones.
- `components/kanban/kanban-header-mobile.tsx` owns the left listing trigger.
  Its existing search toggle clears the hidden query, but List does not wire
  its inline search through this entry point.
- `MobileMenuSheet` and `MobileTasksListOptions` expose the independent List enum.
  The options content can start with an empty `MobileListingMenuActions` wrapper.
- `useEffectiveSidebarView` and `selectSidebarViews` own the active workspace view.
  `SidebarFilterPopover` owns editing state and composes `SidebarViewEditor`.
- `useWorkspaceSidebarTasks`, `useSidebarTaskPage`, and `useSidebarPageContext`
  own accepted task records and bounded query identity. `MobileTaskList` renders
  the sidebar hierarchy through shared row and pagination components.
- `AppNavSheet` retains its contextual task controller through
  `MobileTaskNavigationProvider`. `AppNavSurface` supplies the fixed header and
  workspace picker. The picker wrapper currently has no top padding.

The implementation may use shorter file names while keeping these boundaries.

## Shared query

Branch `TasksPageClient` into phone and existing wider consumers before their
query effects mount. Use `useResponsiveBreakpoint`; 640-767px is still phone.
Do not mount both active query controllers and hide one with CSS.

The phone client consumes `useWorkspaceSidebarTasks` and the selected view.
Extend the query hooks with optional consumer input for effective result scope
and search. Existing callers keep their current defaults. Add this input to
`useSidebarPageContext`, cache identity, local coverage admission, and response guards.
A temporary filter must never reuse an unfiltered page under the same key.

Use existing preferences for pins, child order, grouping, collapse, and colors.
Preserve the 100-row sidebar page bound and complete-tree ranking. Render one
page through the shared task-row builders and paginator. Do not fetch the whole
workspace or resort the generic List page to approximate the shared view.

`MobileTaskList` currently re-reads the global effective view. Allow its page
consumer to supply the resolved display view, or extract its presentation body.
The renderer and query must use the same effective group and sort. Retain shared
action hooks and stable task-dialog ownership; do not mount a task-picker sheet
as the primary page or copy its business logic.

## Result state

A planned `useMobileListResults` controller owns local result state, separate
from the persisted `SidebarViewDraft`:

- Selected saved-view identity comes from existing workspace settings.
- `searchOpen`, immediate text, admitted text, and applied result clauses belong
  to the current page and view context.
- The result-filter sheet has a local draft. Apply admits it; Close discards it.
- A view or manual workspace change clears result state and returns to page one.
- Closing the header search control cancels admission, clears result state, and
  restores focus. An ordinary empty entry starts closed.

Reuse the existing 300ms admission policy. A pure query-composition helper
produces an immutable effective query. Ordinary filters append conjunctively
to the saved filters. Neither path calls `updateSidebarDraft` or
`updateUserSettings`. Opening View options still shows the saved view and its
existing shared draft, not the composed result query.

Result filters initially expose workflow, repository, and status, with the
existing filter registries and localized labels. Archive membership is visible
as an inherited or explicit result scope. Clear removes temporary refinements.
A temporary selection cannot broaden a conflicting explicit saved filter.

## Search transport

The sidebar's `titleMatch` clause searches titles only. It cannot replace the
workspace search contract. Extend `SidebarTaskViewQuery` and TS
`SidebarTaskQuery` with an optional request-only `query` string. Bound it to
256 UTF-8 bytes, matching the saved clause value bound and remaining well below
the existing request body limit. Do not extend persisted view clauses or change
`titleMatch` semantics.

Tasks applies literal workspace matching before ranking, counting, and paging.
Reuse the predicate contract in the
[workspace search design](../../tasks/system-design/workspace-search.md#planned-phone-list-query-adapter).
Include the string in the existing JSON query digest, client cache identity,
and cancellation guards. Keep authorization and personal color preferences
server-derived. No request can select another user's settings.

For a nonempty query, use the bounded server path until complete local coverage
can prove the same searchable fields and matching semantics. This package does
not add that local proof. An empty query retains existing local fast paths.

## Phone composition

The manifest owns authoritative previews UI-01 through UI-09.

- UI-01: workspace/layout context, search icon, hamburger, saved-view tabs, then rows.
- UI-02: one search/Filters row appears beneath tabs. An active filter summary
  stays inside this revealed region. No permanent input or Filters row remains.
- UI-03: a content-sized result-filter drawer owns a single focused operation.
- UI-04/UI-05: the left List control opens View options with layout choices,
  selected view, and the shared editor. The editor body owns vertical scrolling.
- UI-06: the hamburger opens navigation, with a padded fixed workspace picker.
- UI-07: desktop sidebar and standalone List retain their established compositions.
- UI-08: loading, empty, and error feedback occupy the task body.
- UI-09: explicit links and plugin result overrides remain visibly temporary.

`MobilePickerSheet`, `SidebarFilterPopover`'s phone drawer, and `AppNavSurface`
are the shipped interaction references. Short temporary choices fit their
content. The view editor and navigation can use tall inset drawers for deep
content. Use dynamic viewport units and safe-area clearance.

## Editor ownership

Extract a shared editor controller from `SidebarFilterPopover` without duplicating
its workspace guards, deletion confirmation, view limit, rename requests, or
save journal. Compose that controller and `SidebarViewEditor` inside List's
View options. Existing desktop and task-picker consumers retain their wrappers.

The phone sheet shows the layout selector above the current saved-view name.
The view actions menu provides create/rename/delete and Save as new entry as
appropriate. Draft actions remain visible in a fixed footer only for a draft.
Remove duplicate action rows when adapting `ViewHeaderRow`; use one save owner.

Sort, grouping, and row edits update the existing shared draft. Saving writes
that workspace entry. Discard uses the established action. Automatic colors
continue their immediate writes through `useAutomaticColorSettingsController`.
They never become draft fields. Their failure feedback remains independent.

Preserve drafts across orientation/layout changes. Capture workspace identity
before each mutation. A stale callback or deletion confirmation cannot target
the next workspace. Temporary result state never enters a Save payload.

## Menu integration

The phone List menu has no saved-view editing shortcut. Its contextual Tasks
body remains selectable but starts collapsed and omits editing controls in this
presentation. Pass a presentation capability through the existing task outlet;
do not globally remove `SidebarFilterBar` from the dedicated task picker.

Reuse saved sidebar visibility and relative tool order. Group New Task,
Home/quick actions, tools, Tasks, and utilities with one spacing scale. Preserve
plugin controls and metrics, dialog lifetimes, task navigation, and focus return.
Add top padding above the fixed workspace picker. Remove empty action wrappers
rather than hiding their children while retaining parent gaps.

## Geometry and density

The user explicitly rejects enlarged mobile controls. Preserve pre-change
computed control heights at the same root font and viewport. The current
layout choices measure 40px and select options measure 28px. Other control
families use a captured pre-change baseline; do not reduce an existing height.

Use existing typography and tokens. Proposed content insets are 16px, related
control gaps 8px, and section gaps 12px. These are spacing targets, not a reason
to enlarge controls. Keep each hit region unobstructed and keyboard reachable.
New icons match adjacent existing header controls. No global size override belongs here.

This explicit user constraint refines the generic 44px guidance for these
surfaces. Existing sizing requirements on unrelated surfaces remain effective.
Measure both height and min-height. Check phone width independently of pointer
precision, including a narrow fine-pointer viewport.

## Header diagnosis

The user's crop proves Default is blurred while List remains sharp. Source
inspection found no label blur, and isolated WebKit dismissal left no overlay.
The header is transparent and relatively positioned. Native iOS top-edge
compositing is a hypothesis, not a diagnosed cause.

Task 04 first captures the same device state, viewport, Safari/PWA mode, and
safe-area placement. Compare cold load and post-overlay states. Try an opaque
phone header attached to the viewport and exactly-once safe-area allocation as
a bounded experiment. Inspect actual label pixels and text bounds.

Choose the smallest correction supported by that evidence. Do not ship a font
weight/color change as proof of a blur correction. Do not remove intended
backdrop blur from all drawers. Without device evidence, retain this item as
unverified and leave Task 04 and final acceptance incomplete.

## Compatibility

Normal phone List uses the selected sidebar view. Desktop standalone List
retains its scalar sort/group/details preferences. Existing stored values are
not rewritten or used to replace the phone's selected view.

Explicit route parameters enter the local result-state adapter. Preserve query,
workflow, repository, archive mode, and supported scalar sort/group requests.
Link-owned dimensions replace the corresponding effective dimensions only for
that result request. This exception does not alter saved filters or selection.
Show an active-result indicator and reveal the search row on explicit entry.
Clear restores the selected view. Manual view selection removes route overrides.

The archive command retains its explicit Archived/all-workflow/all-repository
scope and empty search. Treat it as a link-owned result request. Do not create
an Archived saved view or rewrite a view merely to satisfy that command.

Eligible plugin facet sort/group remains a page-local List presentation override,
under an optional Additional List options disclosure in the left sheet. It is
clearly separate from Saved view. Retain existing facet validation, unavailable
feedback, multi-value grouping, and counts. Do not add facet fields to
`SidebarView`, globally rank facets using a partial page, or silently apply
legacy facet preferences over a newly selected view. Explicit links or user
selection activate this compatibility mode. Leaving it restores base ordering.

These compatibility states are additional to the ordinary preview, and must
have their own screenshots. They cannot introduce a second default toolbar.

## Failure and persistence

Reuse existing query-error/status and pagination controls. Empty results do not
imply a read error. Access denial clears affected records. Background errors
retain eligible rows with Retry. Delayed requests cannot restore old refinements.

Saved views remain in existing backend-owned settings. Result refinements stay
local, with explicit links as their portable representation. No database or
saved-settings schema migration is required. The request-only search field is
additive and omitted by existing consumers.

## Verification

Tests cover query composition, no unintended writes, complete sorting before
paging, search fields and punctuation, live updates, and responsive controller
ownership. Phone E2E exercises the actual entry points and rendered geometry.

The final work order compares screenshots with every named preview. Record
structural matches, actual control heights, spacing, scroll owners, and every
unresolved difference. Test visibility alone is insufficient. Real iPhone
Safari and PWA screenshots are mandatory for the Default blur criterion.

## Decisions and delivery

- [Phone List view identity](../../../decisions/2026-10-26-phone-list-sidebar-views.md)
- [Composable sort rules](../../../decisions/2026-10-06-composable-sidebar-sort-rules.md)
- [Task-wide running rank](../../../decisions/2026-10-07-task-wide-running-sidebar-rank.md)
- [Shared sidebar records](../../../decisions/2026-09-29-shared-sidebar-task-state.md)
- [Plan](../../../plans/mobile-list-layout/plan.md)

## Implementation result

The phone saved-view page lives in `app/tasks/mobile-tasks-page-client.tsx`.
`tasks-page-client.tsx` selects it below 768 CSS pixels and preserves the wider
standalone List consumer. `MobileListToolbar`, `useMobileListResults`, and the
shared sidebar view editor own temporary refinements and saved-view editing.
`MobileMenuSheet` and `AppNavSheet` own the listing and global menu surfaces.
The plan records focused tests, rendered comparisons, and the outstanding
real-device blur gate.
