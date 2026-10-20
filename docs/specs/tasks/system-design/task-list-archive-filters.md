---
status: current
system: tasks
requirements:
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-001
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-002
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-003
  - REQ-TASKS-LIST-ARCHIVE-FILTERS-004
---

# Task list archive access and filters system design

## Purpose and boundaries

Tasks owns archive membership and workspace task-query scope. This feature
composes the existing query API with a filtered List destination. The same
query model supplies desktop and phone controls. It does not create another
task store, search service, archive endpoint, or settings store.

The [workspace search design](workspace-search.md) owns matching and backend
totals. The [listing preferences design](../../ui/system-design/task-listing-display-preferences.md)
owns saved listing mode, portable workflow defaults, sort, grouping, and row
details. The [archived result design](command-panel-archived-task-results.md)
continues to own command-panel task rows.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-TASKS-LIST-ARCHIVE-FILTERS-001` | Command registration. Route query and scope |
| `REQ-TASKS-LIST-ARCHIVE-FILTERS-002` | Route query and scope. Existing API mapping. Query transitions |
| `REQ-TASKS-LIST-ARCHIVE-FILTERS-003` | Desktop composition. Phone composition. Feedback and polish |
| `REQ-TASKS-LIST-ARCHIVE-FILTERS-004` | SPA bootstrap. Query transitions. Request ownership. Failure and recovery |

## Implementation baseline

These observations describe the source before this feature.

- `GlobalCommands` mounts throughout the app. `HomepageCommands` is page-specific.
- `TasksListControls` and `MobileTasksListOptions` expose a `showArchived`
  checkbox. `TasksPageClient` maps it to `includeArchived`, mixing both sets.
- `listTasksByWorkspace` already accepts `onlyArchived` and `includeArchived`.
  The HTTP handler, service, and repository already support archive-only paging.
- `TasksDataRoute` in `src/spa-routes.tsx` is the live SPA integration point.
  `app/tasks/page.tsx` is not the route bootstrap for the current runtime.
- Go `tasksPageBootData` supplies page one of active tasks using saved scope.
  It does not honor the new archive, text, workflow, or repository route fields.
- `useLatestWorkspaceRequest` guards request sequence and workspace. It does
  not independently prove that a response still matches the visible query.
- Header display surfaces obtain workflow and repository values from
  `useKanbanDisplaySettings`. Those values need explicit list-route overrides.

## Components and responsibilities

Keep `TasksPageClient` as the feature coordinator. Extract its query derivation
and transitions before adding more fields to this already large file.

Feature modules:

- `lib/tasks/tasks-list-query.ts`: archive enum parsing, explicit-scope parsing,
  query-to-API mapping, boot eligibility, and metadata readiness.
- `hooks/use-tasks-list-query-state.ts`: URL-backed selections, saved-scope
  fallback, user edits, page resets, and current-query transitions.
- `app/tasks/tasks-list-filters.tsx`: desktop filter fields and applied indicators.
- `app/tasks/tasks-list-filters-drawer.tsx`: phone presentation of the same fields.

Existing boundaries remain responsible for their current work:

- `GlobalCommands` registers archive navigation.
- `TasksDataRoute` admits matching initial rows.
- `hooks/use-tasks-list-data.ts` owns reads, request identity, cancellation,
  current-query refreshes, and last-page recovery.
- `lib/links.ts` builds the encoded archive destination.
- `app/tasks/tasks-list-read-feedback.tsx` owns error and result announcements.
- `TasksListControls` owns list controls and existing sort/group affordances.
- `TasksPageContent` composes header, toolbar, feedback, list, and pagination.
- `KanbanHeader`, `KanbanDisplayDropdown`, `MobileMenuSheet`, and
  `MobileTasksListOptions` retain shared listing chrome and display preferences.

## Command registration

Register one `CommandItem` with ID `nav-archived-tasks` in `GlobalCommands`.
Use the existing Navigation group and an `IconArchive` glyph. Resolve label and
search keywords through i18next. Include archive, archived tasks, history, and
completed task navigation terms. The visible label is **Go to archived tasks**.

Use `disabled` while the current workspace is unresolved. Construct the link
from the current workspace when commands update. The command must not retain
another workspace's cached href.

Add a dedicated `linkToArchivedTasks` helper beside `linkToTasks`. Build its
query with `URLSearchParams`. Its shape is:

`/tasks?workspace=<id>&archive=archived&workflow=all&repository=all`

Do not include search, sort, or group overrides. The route begins at page one.
The command is a filtered variant of Tasks, so it does not add a top-level
entry to `APP_DESTINATIONS` or another `HomepageCommands` registration.
Existing task actions, including **Archive task**, keep their registration and
relative ordering. Add a regression for the shared search term `archive`.

## Route query and scope

| Field | Route representation | Resolution |
| --- | --- | --- |
| Workspace | Existing `workspace` parameter | Existing authenticated workspace resolution |
| Archive mode | `archive=active`, `archive=archived`, `archive=all` | Missing or unsupported value resolves to Active |
| Search | `q=<text>` | Missing means empty. preserve existing text matching and normalization |
| Workflow | Omitted, `workflow=all`, or a workspace workflow ID | Omitted uses saved scope. `all` is explicit unrestricted scope |
| Repository | Omitted, `repository=all`, or a workspace repository ID | Omitted uses saved scope. `all` is explicit unrestricted scope |
| Sort/group | Existing `sort` and `group` parameters | Keep existing parsers, preferences, and plugin facet behavior |

Distinguish an omitted scope field from an explicit `all`. Never use a truthy
fallback that changes `all` back to the saved workflow or repository.
For an omitted workflow, use the active workflow resolved for the workspace.
Do not reapply a raw saved workflow ID after bootstrap rejects it for the
resolved workspace. This resolution does not save portable settings.
Validate explicit IDs against loaded metadata for the resolved workspace.
Wait for that metadata before classifying an ID as unavailable.

An explicit scope value is a route override, not a portable preference import.
Archive-command entry, reload, and history restoration perform no user-settings
PATCH and do not call `setActiveWorkflow`. This prevents an archive visit from
changing a later board visit. Do not introduce localStorage or sessionStorage.

Explicit user workflow/repository edits still use the existing portable-save
actions. Update the URL and effective list query for the same edit. Clear
filters is also an explicit edit. Reuse revision-aware setting writes rather
than constructing a parallel settings payload. A failed save keeps the normal
authoritative-reload behavior of the portable setting.

## Existing API mapping

Use `GET /api/v1/workspaces/:id/tasks` through `listTasksByWorkspace`.
Use one archive flag per request:

| Mode | `includeArchived` | `onlyArchived` |
| --- | --- | --- |
| Active | Omitted/false | Omitted/false |
| Archived | Omitted/false | true |
| All | true | Omitted/false |

Pass effective workflow, repository, search, sort, page, and page size in the
same request. Never fetch All and filter one page in the browser. Use response
`total` and rows from the same accepted request. Existing grouping still acts
on the returned page and does not acquire new global grouping semantics.
The existing HTTP, service, repository, and authorization contracts need no changes.

## SPA bootstrap

Go boot task rows are compatible only with the active, empty-query, saved-scope
first page for the same workspace. New explicit filter fields conservatively
make that task-page seed ineligible, even if their values happen to match defaults.
Keep boot workspace, repository, and workflow metadata.

In `TasksDataRoute` and `resolveTasksDataRouteInitialData`, suppress ineligible
task rows and totals and pass `initialDataLoaded: false`. Use a shared pure
eligibility check. Reuse `shouldSkipInitialTasksFetch` only behind the full query-eligibility gate.
The data hook supplies its legacy archive boolean from the resolved archive mode. The first matching API response
releases loading. This avoids a backend boot contract change and prevents active
rows from flashing beneath an Archived selection.

On client navigation, query fields must update an already mounted list.
On workspace navigation, do not reuse task-page boot identity from a previous
workspace. The current workspace resolves before reads begin. Do not modify the
unused async `app/tasks/page.tsx` as a substitute for SPA wiring.

The SPA route validates an explicit `workspace` against the authorized workspace
list, then uses the canonical workspace action before mounting the List client.
Changing that URL field cancels the previous bootstrap. Key the List client by
resolved workspace so browser history preserves explicit filters. A failed
workspace lookup shows a translated route error with Retry and Home. An unknown
workspace shows Workspace not found with Home. Neither state mounts List or
falls back to another workspace.

The initial Tasks boot metadata marks both fields of `initialMetadataLoaded`
ready. SPA workflow and repository reads mark their own field ready on success.
A failed read does not block a query that does not use that filter. Cached
repository data alone does not complete the current read. A matching workspace
ID alone does not prove readiness. Invalidate the Tasks boot seed after leaving Tasks or changing its
workspace. Keep this invalidation in the SPA owner so returning from Settings
cannot reuse metadata from a previous visit. Pending metadata remains loading;
failed reads retain the existing metadata recovery path. While SPA metadata
reads are pending, show the route loading state. Mount List after those reads
settle so filter controls do not show unavailable labels for pending metadata.

## Query transitions

Use one transition to change a filter and reset pagination. Do not issue a
request for the previous page with the new filters before page one settles.
Archive, workflow, and repository picks use one history entry per selection.
Non-empty search keeps one 300ms request debounce. Clearing search cancels
queued input and admits the empty query immediately. Search replaces its current URL
entry. Reconcile Back/Forward without saving restored scope or replacing history.
Preserve sort, group, and unrelated supported query fields when editing filters.

Clear filters writes an empty query and explicit `workflow=all` and
`repository=all`. It retains archive, sort, group, and details. Removing a
workflow or repository indicator changes that field only. Search has its own
clear control. Changing archive mode retains the other filters.

A manual workspace change that keeps List open retains archive mode, clears search,
sets both scope fields to explicit `all`, and resets page one. Clear previous rows before rendering
the new workspace context. Do not apply old IDs while new metadata loads.
Keep this normalization inside the Tasks route. Shared board and Threads
history behavior must remain unchanged. `AppSidebarWorkspacePicker` selects a
workspace's Home destination. Preserve that established route choice. The List
effect must not replace a committed navigation outside the Tasks route.
An explicit URL workspace change follows link restoration instead of this
manual-selection normalization.

List selectors receive effective values and actions from the feature query
model. Suppress only duplicate built-in workflow/repository fields in the
Tasks branch of shared display surfaces. Keep plugin contributions and display
settings. Do not let a global default-labelled selector contradict the list.

## Request ownership

Identify reads by workspace, archive mode, effective workflow/repository,
debounced search, sort, page index, and page size. At publication, check both
the current query identity and the latest request generation. Invalidate the
previous identity when intent changes, before a replacement read can start.
Abort obsolete reads where supported, but do not rely on cancellation alone.

Associate rows and totals with the accepted identity. A replacement query shows
loading instead of publishing previous-scope data as current. A response for an
older query cannot clear a newer loading/error state or issue its toast.
On unmount or workspace loss, reject pending publications and clear visible data.

Mutations and pull-to-refresh use the same current query owner. After removing
the last row on the last page, compute the last available page from the returned
total and request it once. Do not reset filters to recover pagination.

## Desktop composition

Keep the shared page header and navigation. Move List's search out of the shared
header into one list toolbar, so search and scope controls form one region.

The first toolbar row contains Active / Archived / All, a flexible search field,
and the current result total. The second row contains labelled workflow and
repository selectors and Clear filters. Existing Sort and Group controls align
at the other end of that row and can wrap as a group. Applied workflow and
repository indicators wrap below. Do not show hardcoded per-mode counts or
invent totals for modes that were not requested.

Use shared ToggleGroup or equivalent single-choice semantics. Follow ordinary
28px desktop sizing and existing tokens. Keep the list body as the vertical
scroll owner. The toolbar does not introduce a nested vertical scroller.
Show Clear filters only when search or either scope field is restrictive.

## Phone composition

Use `useResponsiveBreakpoint` at the existing 768px phone boundary.
Keep `KanbanHeaderMobile` and its app-navigation entry. List owns a permanent
search field instead of requiring the old hidden-search toggle.

For touch entry from a listing, use its existing View options to choose List,
then tap Archived. The command panel remains available with a hardware keyboard.
Existing phone command tests open it with Control+K and tap the result. Do not
assume a shipped touch command-panel opener or add one to this feature.

Below the header, show the archive selector as a full-width three-part control.
Place search beside a Filters button. The Filters badge counts non-All workflow
and repository fields, from zero through two. The applied indicators wrap below.
The task rows remain the main content and retain direct row navigation.

Filters opens a dedicated inset bottom `Drawer`, following the geometry of
`MobileMenuSheet` and its `ResponsiveMenuSurface`. This is a short, temporary
choice. A full-page route would add navigation without more useful content.
Use a fixed heading, one `min-h-0` internal scroll body, and a safe-area-aware
Done action. The fields are workflow, repository, and Clear filters.
Selections apply immediately. Done dismisses and returns focus to Filters.
Closing the drawer does not discard selections. Do not nest View options
inside this drawer. Sort, Group, and row details remain in existing View options.

Phone and coarse-pointer controls have targets of at least 44px. Use 48px
nominal sizing for the drawer's Done action. Bound the drawer by dynamic
viewport height, contain overscroll, and preserve input anti-zoom styling.
At 640-767px with a fine pointer, the phone composition and targets still apply.
Tablet controls retain touch dimensions through the existing sizing helper.

## Feedback and polish

Keep controls mounted while rows load. Use one translated status announcement
for the current loading/result state. Do not display a stale numeric total as
the current result. A failed read shows a stable error and Retry for its query.
An invalid scope shows its unavailable field and a clear or replacement action.
Neither state renders the successful-empty message.

An unrestricted empty Archived view says **No archived tasks**. An empty view
with list filters says **No tasks match these filters** and offers Clear filters.
An empty Active/All view retains its existing appropriate empty message.

Use existing theme tokens and Tabler icons with `currentColor`. Keep selected
states visible without relying on color or animation alone. Routine archive
and filter changes are immediate. Do not add stagger, blur, or row transitions.
Use existing overlay motion and reduced-motion support. Keep forced-colors
boundaries, visible focus, and `overscroll-behavior: contain` on the drawer.

All host copy, labels, keywords, empty states, status text, and accessible names
use locale keys. Supply en, pt-pt, zh-cn, ja, and ko, generate zh-hk/zh-tw with
`i18n:zh-hant`, and regenerate pseudo. Do not use em dashes or module-scope `t()`.
Existing plugin-supplied labels keep their owner and registration keys.

## Failure and recovery

Unknown archive values normalize to Active. Unknown or wrong-workspace scope
IDs remain explicit unavailable filters after metadata readiness. The client
makes no broader task request until the user clears or replaces them. A metadata read
failure offers retry rather than classifying all IDs as deleted.

Retry and mutation refreshes capture the current identity. A response after
workspace access loss cannot restore its rows. Authorization stays at the
existing task endpoint. URL parameters grant no access and contain no credentials.
Archive navigation does not call lifecycle or session endpoints.

## Persistence and decisions

This feature needs no migration or new persistence layer. Explicit URL state lasts with
the link. Portable defaults remain backend-owned under
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
Workspace boot authority remains under
[ADR 0023](../../../decisions/0023-active-workspace-cookie.md).

A new ADR is unnecessary for this scoped query composition. Automatically
clearing saved board filters would change a later board visit. Route overrides
carry the temporary scope instead. Manual edits retain existing portable semantics.

This feature needs no new log, metric, feature flag, or polling loop.

## Verification and documentation

The [implementation package](../../../plans/task-list-archive-filters/plan.md)
maps every criterion to targeted unit, component, and Playwright coverage.
Tests must include delayed same-workspace responses, incompatible boot data,
query-only navigation, history restoration, first-page resets, and saved-scope
preservation. The phone check includes actual taps, overlay containment,
44px targets, narrow fine-pointer geometry, focus return, and zero horizontal overflow.

Update the existing **Find and organize tasks** and **Archive, unarchive, and
delete** sections in `docs/public/tasks-and-workflows.md` with implementation.
Keep this guidance aligned with the shipped controls.
