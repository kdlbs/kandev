---
status: current
system: ui
requirements:
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002
  - REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003
---

# Task Listing Display Preferences System Design

## Purpose and boundaries

UI owns the independent listing and Home presentation contract in the
[requirements](../requirements/task-listing-display-preferences.md). Reuse
backend-owned user settings for the explicit choice and retain browser-owned
listing memory. Workspace selection and Office mode remain governed by
[ADR 0023](../../../decisions/0023-active-workspace-cookie.md) and
[Office mode ownership](../../../decisions/2026-08-15-office-mode-follows-active-workspace.md).

The Threads page uses its existing
[conversation deck](threads-conversation-deck.md) and
[saved views](threads-saved-views.md). This design does not alter their filters,
session selection, resource budgets, swipe feedback, or topbar composition.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-001` | Preference ownership; Remembered listing and list details |
| `REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-002` | Portable settings contract; Entry resolution; Failure and recovery |
| `REQ-UI-TASK-LISTING-DISPLAY-PREFERENCES-003` | Portable settings contract; Entry resolution; Home consumers; Settings and mobile composition; Verification |

## Preference ownership

| Concept | Existing source | Scope | Values |
| --- | --- | --- | --- |
| Startup/default destination | `UserSettings.StartupPage`, wire `startup_page`, frontend `startupPage` | Portable per user | `task_overview` (default), `last_task`, `threads` |
| Remembered listing | `localStorage["kandev.taskListing.view.v1"]` | Current browser/device, existing cross-workspace scope | JSON string `kanban`, `pipeline`, `list`, or `threads` |
| Recent task target | `localStorage["kandev.recentTasks.v1"]` | Current device; target filtered by workspace | Existing recent-task entries |
| Rich List rows | `tasks_list_show_details` / `tasksListShowDetails` | Portable per user | Boolean, default `false` |
| Workflow filter | Existing `workflow_filter_id` / `workflowId` | Existing portable scope | Existing workflow ID or All Workflows |

Extend the existing startup enum instead of adding a parallel Home preference,
local key, or workspace map. The existing startup setting already owns the
explicit destination choice and the Settings save lifecycle. `threads` is the
only new fixed destination. `task_overview` retains its remembered-mode
meaning; `last_task` retains its startup-only behavior. Separate last-task
startup plus fixed Threads Home is not introduced.

Saving `startup_page` does not call `setStoredTaskListingView`. View changes
and the existing `/tasks` and `/threads` arrival effects may continue updating
listing memory; they must never PATCH `startup_page`. Arriving in Threads via
Home can therefore remember Threads as the latest visited mode without
turning listing memory into the authority for the fixed default.

## Portable settings contract

`StartupPageThreads = "threads"` is defined in
`apps/backend/internal/user/models/models.go`. `NormalizeStartupPage` preserves
the three valid values and defaults absent or unsupported stored values to
`task_overview`. `applyStartupPage` in `internal/user/service/service.go`
accepts all three values, keeps current whitespace normalization, rejects an
unsupported submitted value without changing the record, and leaves an omitted
patch unchanged.

The existing user-settings HTTP read/update endpoints, DTO/controller mapping,
`user.settings.updated` event, and revision-controlled write path remain the
transport. DTO serialization, store scan/marshal, event publication, and
`internal/backendapp/boot_state_routes.go` already call the backend normalizer;
each must retain `threads` end to end. Settings stay in the existing users
settings JSON on SQLite and PostgreSQL. No table, column, endpoint, or schema
migration is added.

`StartupPage` in `apps/web/lib/types/http-user-settings.ts` and
`parseStartupPage` in `apps/web/lib/ssr/user-settings.ts` retain all three values.
The common wire-to-store mapper serves HTTP, save responses, and WebSocket updates. Keep
revision ordering, omitted-field preservation, and default ownership from
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
Do not introduce browser persistence for this field.

## Entry resolution

There are two decisions: generate a generic Home destination, or resolve a
bare root entry. Explicit routing is not a third preference store.

| Entry in a non-Office workspace | `task_overview` | `last_task` | `threads` |
| --- | --- | --- | --- |
| Bare `/`, optionally with `workspaceId` | Remembered listing | Newest matching local task, otherwise remembered listing | Threads in that workspace |
| Home action | Existing overview destination | Existing overview destination, never recent-task resume | `/threads?workspace=<id>` |
| Explicit `/?home=overview` | Remembered listing | Remembered listing | Remembered listing, bypassing fixed default |
| Explicit workflow root URL | Board for that workflow | Same | Same |
| Explicit `/tasks`, `/threads`, task/session or focus URL | Keep destination | Keep destination | Keep destination |

An explicit overview selects the overview family, not a fixed Kanban mode.
It already restores remembered List or Threads. Preserve that meaning. A
view-toggle selection records its chosen mode before navigating through
`resolveTaskListingNavigation`; the fixed default cannot bounce the user back
to Threads when they deliberately select Kanban or Pipeline.

### Bare root

Keep startup policy in `apps/web/lib/startup-page.ts` and its integration in
`apps/web/app/page-client.tsx`. Use the existing
`isExplicitHomeDestination` checks for props and query task/session IDs,
workflow scope, and `home=overview`. A workspace ID alone scopes Home and does
not suppress startup selection.

After route settings and workspace resolution settle, a bare root with
`startupPage === "threads"` resolves directly to `linkToThreads(workspaceId)`.
Otherwise preserve `resolveStartupTaskId` and remembered-listing fallback.
Use one resolved redirect in the effect, with `router.replace`, so a fixed
choice cannot issue a competing remembered-list redirect in the same render.
An explicit workflow or task/session destination suppresses both startup
override and remembered routed-view restoration as it does today.

Hydration readiness belongs to the route/bootstrap boundary, not the local
view hook. The `useTaskListingView` effect synchronizes `kanbanViewMode` while
preserving `loaded`; it must not manufacture authoritative settings readiness.
Preserve the actual loaded state and wait for the existing
boot settings or completion of the client settings fetch before resolving a
default. Do not use board snapshots or the presence of workflows alone as a
readiness gate: an empty workspace must still reach Threads or onboarding.
Return explicit readiness from `useKanbanRouteBootstrap`, scoped to the
requested workspace and fetch lifecycle, and consume it in `KanbanRoute` before
mounting the default-resolving page.
Failed settings fetches complete with the normal fallback rather than waiting
forever. A late response for a previous workspace cannot redirect the current
route. Existing Office mismatch handling runs before this task-listing choice.

### Explicit destinations and reload

The default is not a global route guard. Do not attach a redirect to the
always-mounted sidebar, `useTaskListingView`, or a settings subscription.
`/tasks` stays List on reload even with a Threads default. `/threads` retains
workspace, `taskId`, and `sessionId` focus parameters. Browser Back/Forward
restores the explicit route. Settings changes update future Home hrefs without
replacing the current task, workflow, settings, or listing route.

Keep `linkToTaskOverview`, task Back links, workflow navigation, and
`listingHistoryHref` as explicit navigation. Never blanket-replace their
overview URLs with a Home-default resolver.

## Home consumers

Use one pure Home resolver in
`apps/web/lib/navigation/workspace-home.ts` with workspace ID, Office mode,
and the saved startup choice as inputs. Both `workspaceHomeHref` and
`homeDestinationHref` in `core-destinations.ts` delegate to it. Extend
`NavContext` and `useNavContext` to carry `startupPage` from authoritative
settings. A missing choice defaults to `task_overview` for existing callers.

Resolution order is Office home, explicit Threads home for a resolved
non-Office workspace, then the existing overview. Preserve encoded IDs and
the existing different query names (`workspaceId` for overview/Office,
`workspace` for Threads). Unknown workspace mode retains the current disabled
Home affordance. No-workspace onboarding remains available.

Audit and wire these callers; do not infer workspace type from the pathname:

- Sidebar brand/header and primary Home row.
- Sidebar settings-exit action and workspace picker, including picking the
  active workspace again when that action currently navigates Home.
- `useHomeAffordance`, used by shared topbars and mobile navigation.
- The phone listing header's `MobileBrandLink`, which is the listing drawer's
  Home affordance. Only its destination/state input changes.
- Manifest mobile Home and palette `nav-home`. The palette retains its
  workspace-less overview override for existing startup choices, but routes
  a selected Threads default through the same
  workspace-aware Home resolver. Office remains Office when applying the
  Threads choice; do not send it into a deck.

Changes in `src/kanban-route.tsx` are limited to entry readiness or wiring;
its Office mismatch redirect is not a new Home action. Office recovery links
and explicit task overview/Back links retain their existing behavior.

## Remembered listing and list details

`view-preference.ts` remains responsible for parsing the local JSON enum,
legacy `kanban_view_mode === "graph2"` fallback only when no local value
exists, and same-document/storage notifications. A present invalid local
value returns Kanban rather than using the legacy fallback. Failed writes keep
the chosen mode transiently for the current document.

`use-task-listing-view.ts` exposes the preferred/effective modes and updates
the in-memory Kanban rendering mode. Pipeline renders as Kanban on phones
without changing stored Pipeline; Threads uses the native phone deck.
`view-navigation.ts` owns explicit toggle routes and path-preserving scope
changes. None of these boundaries owns the portable Home choice.

Keep the existing workflow resolver's All Workflows behavior and List's
`tasksListShowDetails` flow through `useKanbanDisplaySettings`, display
controls, and `tasks-list-view.tsx`. Details remain false by default, portable,
and independent of Home selection. Missing metadata omits only that metadata;
secondary row actions do not activate the row. No rich-row implementation
change is required for Threads Home.

## Settings and mobile composition

`StartupPageSettingsCard` includes `threads` in its radio options. Keep
`AppearanceState.startupPage`, `buildAppearanceUserSettingsPatch`, draft rebase,
and the `SettingsSaveProvider` contributor. Add no page-local Save button and
do not preview the destination by navigating while the user edits the form.

Revise the existing localized descriptions so they accurately explain all
three options: Task overview restores the last-used listing, Last visited task
only resumes on startup, and Threads always opens on startup and Home in task
workspaces. The shared description must no longer claim every Home action
always opens overview. Reuse the existing Threads label key where possible;
include Threads/home-default search terms in Startup Page discovery aliases.
Update English, pt-pt, and zh-cn, generate zh-hk/zh-tw with `i18n:zh-hant`, and
regenerate pseudo. Resolve copy with `t()` at render time; no em dashes.

Mobile contract:

- Entry is the existing phone Settings navigation into Appearance. The nearest
  shipped form is `startup-page-settings-card.tsx`, with full-width labelled
  radio rows. The shared phone menu follows `mobile-menu-sheet.tsx`'s inset
  drawer, fixed heading, internal scroll, and safe-area treatment.
- The persistent preference stays inline in the full-page Settings form.
  This short single choice needs no extra picker overlay. Option label comes
  first, its effect second; the shared floating Save changes action commits it.
- Reuse the Settings page's existing vertical scroll owner and safe-area
  clearance. Labels wrap; phone hit areas are at least 44px high. Preserve
  desktop density and current breakpoints; do not normalize topbar geometry.
- After navigation, reuse the native single-column Threads deck. Do not add a
  mobile fallback or change deck/session loading solely because it is Home.
- Settings draft, persistence, routing, and failure semantics are shared
  across desktop and phone; only established surface composition differs.

## Failure and recovery

Unsupported stored startup strings normalize to overview; unsupported writes
are rejected. Save errors leave the prior authoritative default and a
retryable Settings draft. A reload receives the backend value, never a retry
marker from browser storage. A successful save or live update changes future
Home navigation through the existing revision-aware settings store.

Blocked local storage cannot prevent fixed Threads navigation. An empty deck
is a valid destination and keeps its empty state. Failure to find a recent
task uses the remembered-listing fallback only for `last_task`. No-workspace
and failed-bootstrap paths retain the existing recovery surface and must not
loop or permanently display a startup loader.

Existing user authorization and workspace access checks remain unchanged;
startup choice conveys no additional access. Existing settings request/event
errors and route tests provide evidence; no new logs, metrics, or feature flag
are required.

## Verification

Backend tests cover enum normalization, rejected/omitted patches, JSON
round-trip, user isolation, DTO/event/boot retention, and reopening persisted
settings. Frontend tests cover parser/mapper/WS parity, shared Home hrefs,
explicit-destination precedence, deferred bootstrap, and independent local
view memory. Keep existing last-task and Pipeline fallback regressions.

Extend `e2e/tests/settings/startup-page.spec.ts` and
`mobile-startup-page.spec.ts` for selection, Save/discard/failure, reload,
Home after using List, workspace switching, and protected explicit routes.
Use isolated test fixtures. Phone proof uses actual taps, checks the labelled
row hit area, verifies reachable Save, and checks no horizontal overflow.
The [implementation plan](../../../plans/threads-home-default/plan.md)
maps acceptance criteria to exact tests and commands.
