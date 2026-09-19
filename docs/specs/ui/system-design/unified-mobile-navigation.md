---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-MENU-001
  - REQ-UI-MOBILE-MENU-002
  - REQ-UI-MOBILE-MENU-003
  - REQ-UI-MOBILE-MENU-004
  - REQ-UI-MOBILE-MENU-005
  - REQ-UI-MOBILE-MENU-006
  - REQ-UI-MOBILE-MENU-007
---

# Unified mobile navigation design

## Revised target after user testing (2026-09-17)

This section supersedes the original pinned-shortcut and separate View options
composition below. The earlier sections describe the first implemented iteration;
its verification results do not validate this revision. Task 03 implements this revision; its validation is recorded in the work order.

### Embedded Tasks (REQ-UI-MOBILE-MENU-004)

Reuse `SessionTaskSwitcherSheet`'s data/actions/dialog controller and
`MobileTaskList`, `SidebarFilterBar`, and task creation/action handling. Extract a
controller/body boundary instead of copying sidebar business logic. Supply an
inline body adapter for the shared menu, with a semantic Tasks disclosure and
New action; the menu already owns workspace selection and Quick Chat, so do not
repeat those inside the embedded body. Preserve the standalone title picker.

`MobileTaskNavigationProvider` lives in the app shell above responsive page
headers. It owns the lazy, workspace-keyed sidebar/controller after first use;
`AppNavSheet` registers a body outlet and origin-specific selection/navigation
callbacks. The outlet also forwards archived-task and port-forwarding context
from the task page to the retained sidebar; React portals do not inherit target
context. Only the body is portalled into the menu; action dialogs stay mounted
when the outlet closes or the header unmounts on rotation.
`AppNavSheet` supplies this Tasks body after primary/local
navigation and before workspace actions. Start expanded; preserve disclosure
state for the mounted host without new persisted preferences. Remove the
`MobileTaskShortcuts` projection and dedicated navigation pins. Suppress the
Task views action on phone shared menus. Preserve existing global route links
unless duplicating the same entry is demonstrably unnecessary; this revision
changes the embedded body, not the navigation manifest's destination contract.

The app menu owns the only vertical scroller. The embedded list grows naturally;
remove its standalone `flex-1/overflow-y-auto` wrapper in inline presentation.
Keep loading/empty/error/denied states inside Tasks, with global navigation usable.
Task rows retain existing pin controls/grouping as part of the real sidebar.

Keep the task action/dialog controller outside the drawer's unmounting portal.
After first request, retain that controller across menu closure and breakpoint
changes so creation/edit/confirmation drafts survive. The workbench shares
selection cancellation with its title picker; non-task pages use push navigation.
Do not key/remount the workbench when workspace metadata briefly refreshes.
Closing the menu must not cancel a selection that successfully transferred
ownership, and pending selections from an old workspace must remain invalidated.

### Listing-title options (REQ-UI-MOBILE-MENU-005)

Remove `ViewOptionsButton` and its toolbar row from `KanbanHeaderMobile`.
Always render `MobileListingContext` for the top-level mode label, including
Threads; each opens the existing listing-only `MobileMenuSheet` and owns focus
return. Kanban/List already have this wiring alongside the redundant button.

Move Threads saved-view controls into the options content via an explicit body
slot/controller boundary in `ThreadsViewControls`. Preserve saved-view selection,
editing, save/discard, delete confirmation, and sync retry in the same phone
options flow. Do not nest a second drawer for the same level of view choices.
Keep the controller mounted during surface handoffs. Desktop Threads controls
and wider menu composition remain unchanged.

### Mobile composition and verification

Reuse the shipped inset app drawer and 44px disclosure/listing-context targets.
Order: workspace, global/local navigation, collapsible Tasks, workspace actions,
plugins/integrations, utilities. Test the real inline task list, child action
handoffs, workspace-denied reads, long-list scrolling, task-title picker, and
rotation. Verify each of Kanban/List/Threads enters its options from its header
and returns focus there. Inspect 393px/767px and wider 768px/820px behavior.

## Boundary and evidence

This is phone presentation and entry-point work. No backend, schema, plugin SDK,
or permission changes are required. Existing source currently has three shells:
`MobileMenuSheet` mixes listing controls and app navigation,
`SessionMobileTopBar` uses `onMenuClick` for `SessionTaskSwitcherSheet`, and
`AppNavSheet` uses a left Sheet for other pages. `AppNavSections` already shares
the manifest destinations and hoisted dialog controls between some of them.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-UI-MOBILE-MENU-001 | Shared shell and page controls; Mobile composition |
| REQ-UI-MOBILE-MENU-002 | Task title and navigation handoff |
| REQ-UI-MOBILE-MENU-003 | Pinned shortcuts and recovery |
| REQ-UI-MOBILE-MENU-004 | Embedded Tasks |
| REQ-UI-MOBILE-MENU-005 | Listing-title options |
| REQ-UI-MOBILE-MENU-006 | Menu hierarchy revision |
| REQ-UI-MOBILE-MENU-007 | Action-first composition |

## Shared shell and page controls

Extend `components/navigation/app-nav-sheet.tsx` as the app-level entry point.
Use `useResponsiveBreakpoint` to choose a phone Drawer while retaining its
existing wider Sheet. Reuse `AppNavTrigger` and its connection-health accessible
name on all phone shells. Retain a controlled-open adapter where existing
listing state is externally controlled; do not create a global modal store.

Extract only the reusable inset geometry from `MobileMenuSheet` if needed;
the navigation shell belongs in `components/navigation`, not in the task domain.
Use `AppSidebarWorkspacePicker`, `AppNavSections`,
`MobileWorkspaceActionsSection`, and the existing manifest resolver. Keep
availability-gated hooks distinct from static destinations, following the
[navigation manifest ADR](../../../decisions/2026-08-04-navigation-manifest-boundaries.md).
Do not hardcode a new destination catalog. Preserve the relative order of
eligible destinations. Update selected-route presentation to map task detail to
Tasks without changing href resolution or Home/startup rules.

The phone shell stops using listing-only `omitDestinations` to hide Tasks and
Threads. Office-specific suppression still prevents a Kanban Tasks link from
being mistaken for Office Tasks. Keep `pageNav` for existing Settings/Office
navigation, grouped below global links with a localized section heading.
The drawer body order is workspace, global destinations, page-local navigation
when present, optional pins, workspace actions, plugins/integrations, utilities.

`KanbanHeaderMobile` renders the shared hamburger and a separate labelled View
options entry beside listing context. Keep Threads' existing saved-view picker.
Its View options control sits in the content toolbar rather than taking the
picker's title slot. Use `MobileMenuSheet` in listing-only mode, reusing `MobileDisplayOptions`,
`useMobileMenuSheetState`'s view and display wiring, and
`MobileListingMenuActions` after separating page actions from navigation.
The page-context button on Kanban/List may open the same options surface, but
the labelled control remains visible. Avoid two simultaneously open overlays.

Keep phone New Task/Quick Chat/Quick Terminal and plugin workspace actions
available through existing primary affordances and shared workspace actions.
Do not move a workspace-wide action into listing display settings. Preserve
search-hide clearing and focus restoration to the originating options control.
Wider `KanbanHeader` callers retain their combined menu behavior.

## Task title and navigation handoff

In `SessionMobileTopBar`, make the title text plus chevron a semantic button.
Keep branch/diff metadata out of its accessible name; do not nest controls.
Add explicit task-picker callback/open-state props and reuse the current
mobile-session task-switcher state. `ResponsiveTaskPicker` owns the requested
`SessionTaskSwitcherSheet` above the phone/tablet layout branches.
Replace the hamburger action with the shared app navigation shell. Preserve
the existing back-to-task-overview link and current-session picker separately.

The title picker keeps the existing task selection controller and URL-replace
policy. A non-task entry uses the existing `useTaskViewNavigation` push callback.
Extend navigation dialog controls to accept the workbench's task-picker opener
and share `TaskSheetSelectionProvider` between the workbench picker and
shortcuts, so Task views and shortcuts on the workbench do not
instantiate a competing task drawer or use the non-task push policy.

Retain `useAppNavDialogs` outside the Drawer's unmounting body. Close navigation
before opening Task views, status, task creation, or other dialogs. Use the
existing close-auto-focus handoff pattern rather than a timed delay. Remember
the real opener separately for the title picker and app menu. Keep a requested
task controller mounted across a breakpoint change so child dialog drafts live.
Workspace identity changes invalidate pending selection without remounting the
workbench. The picker remains keyed by workspace, while its shared selection
provider keeps a stable identity across metadata refreshes.

## Pinned shortcuts and recovery

Use `mobile-pinned-tasks.tsx` and the pure `mobile-pinned-task-items.ts`
projection. `mobile-task-shortcuts.tsx` adapts the existing `useSheetActions`
selection behavior and uses the shared workbench cancellation controller;
non-task surfaces use a local controller and push navigation. Read pins from
`useSidebarTaskPrefs`; read task data through the existing workspace-scoped
`useWorkspaceSidebarTasks` boundary. Mount the shortcut reader only after the
menu is requested. Reuse its cache/request leases rather than adding API calls
or mounting the full task picker merely to render links.

Project eligible tasks before applying pin order and taking three. Deduplicate
pin IDs, exclude archived/deleted/foreign tasks, and preserve pin order without
writing preferences. The compact list is independent of a saved task-view
filter: a filtered-out pin is still a shortcut if workspace-authorized.
Use the workspace read boundary's identity/pending/access-denied signals;
never infer authorization from the presence of an old cached task. While the
new workspace is pending, withhold shortcuts. On a recoverable read error show
existing localized error/retry behavior in that section while keeping global
links enabled. On denial hide all task titles. Task deletion during selection
uses the existing missing-task recovery route, without auto-starting a session.

Pins are optional; the existing Task views action remains available even if
there are no pins or shortcut reads fail. Keep that label distinct from the
manifest's Tasks route. No new user setting, recency store, or telemetry is added.

## Mobile composition

- **Entry points:** hamburger at the trailing edge on phone page/task headers;
  title chevron for workbench task choice; View options beside listing content.
  Desktop sidebar, desktop Dockview, and tablet composition retain their layout.
- **Exemplars:** `MobileMenuSheet` supplies inset Drawer geometry and internal
  scroll; `SessionTaskSwitcherSheet` supplies task-choice behavior and recovery.
  `AppNavSheet` supplies app destinations and hoisted dialogs, but its current
  phone side-sheet geometry is replaced.
- **Hierarchy:** active workspace and destinations first; bounded shortcuts
  second; workspace actions and utilities later. The focused page owns dense
  content and display controls. A temporary navigation choice merits a drawer;
  the existing bottom panel navigation remains reserved for workbench content.
- **Geometry:** use dynamic viewport bounds, a fixed heading/close control,
  one `min-h-0 flex-1 overflow-y-auto overscroll-contain` body, inset rounded
  surface, dimmed backdrop, and bottom safe-area padding. Touch controls are
  at least 44 CSS pixels; do not impose phone sizing on desktop controls.
- **Accessibility:** localized names and visible section labels, current-route
  indication, title chevron and expanded state, normal keyboard activation,
  Escape/dismiss and focus restoration. Long titles truncate without reducing
  hamburger or picker hitboxes. Missing title uses existing loading copy.
- **State:** selection, saved views, request freshness, task mutations and
  plugin contributions remain shared domain behavior. Only presentation and
  temporary overlay state change.

## Verification and compatibility

The [plan](../../../plans/unified-mobile-navigation/plan.md) maps criteria to
unit tests and phone E2E. Cover the same hamburger contract from listings, task,
Settings, Office, and an integration page; separately exercise title switching,
local options, pin isolation, dialog handoff, and workspace errors. Check phone
geometry at the configured device and 767px, plus retained wider behavior at
768px and 820px. Keep tests for original task actions; migrate their entry
selectors rather than deleting scenarios.

New copy uses all five locale catalogs and generated Traditional Chinese values.
Public navigation instructions are updated with implementation, not in advance.

## Related decisions

- [Phone navigation entry-point ownership](../../../decisions/2026-09-15-phone-navigation-entry-points.md)

The Threads page supplies its board-derived position as `mobileListingStatus`,
separately from saved-view controls. The header keeps pagination and sync status
visible while the options drawer owns the view editor.

## Menu hierarchy revision (2026-09-19)

REQ-UI-MOBILE-MENU-006 owns the reusable navigation composition, not integration
credentials or automation lifecycle. This section supersedes earlier primary
Tasks/Threads destination descriptions. Task 04 implements this revision and
the isolated preview contains its final production build. Verification is
recorded in the Task 04 work order.

Use phone-scoped `omitDestinations` at `AppNavSheet` for tasks/threads rather
than removing routes or palette entries from the manifest. Adjust phone Home
active treatment for `/`, `/tasks`, and `/threads`; preserve Office local Tasks
and desktop/tablet semantics. Use the existing Home href resolver, including
workspace and startup preferences; do not introduce navigation persistence.

Restyle `InlineTaskHeader` around the Utilities exemplar in `app-nav-sections`:
section divider and padding, `text-sm font-medium`, label aligned with Utilities,
small trailing chevron, and a restrained ghost plus action at the right edge.
The heading hit area remains full-width excluding the 44px plus. Remove the
persistent filled disclosure treatment and excess indentation; keep visible
focus/hover feedback. Do not move or duplicate the task controller.

Add a phone Automations section after workspace actions and before integrations.
Reuse `useWorkspaceAutomations`, `useAutomationSummaries`, `buildAutomationRows`,
and the existing automation route constants. Do not mount the desktop sidebar
component directly: its summaries explicitly disable reads on phones and its
controls use desktop geometry. Use touch rows, a collapsed initial disclosure,
a list link and setup action. Read summaries only when expanded; preserve domain
errors/retry and workspace isolation. Setup uses existing settings routing.

`MobileIntegrationsSection` currently returns null for an empty resolved list.
Keep a labelled section and an integration settings entry in that state; retain
`useAppDestinations` filtering for actual provider/plugin links. Reuse the
existing workspace settings links; do not invent a new integrations page.

Phone hierarchy: workspace, Home, existing local navigation, Tasks, workspace
quick actions, Automations, plugin/integration sections, Utilities. The fixed
menu header and single safe-area-aware scroller remain. Desktop/tablet stay as
shipped. Validate 393px, 767px and the 768px boundary; compare dark and light
screenshots to the Utilities section. No new animation or dependency is needed.

## Action-first composition (REQ-UI-MOBILE-MENU-007)

Task 05 supersedes Task 04's section ordering only. The UI system owns this
reusable composition and disclosure contract; integrations, workspaces and
quick-action launchers retain all domain state and availability rules.

In `AppNavSheet`/`AppNavSections`, supply a phone-only quick-actions slot grouped
with `PrimaryNavSection` rather than moving the entire `workspaceActions` slot.
`MobileListingMenuActions` also contains plugin actions and fallback metrics;
separate the two built-in quick actions into a reusable render boundary with the
existing launch hooks, activity indicator and close/focus handoff. Retain default
composition for other callers. Only the phone navigation variant gets a two-column
quick-action row; use min-width zero and wrapping labels with minimum 44px height.
Do not move plugin content, search or metrics into that row.

Phone sequence: workspace picker; Home and quick actions; existing page-local
navigation; Tasks; existing workspace plugin/canvas and fallback metric content;
Automations; existing plugin navigation groups; Integrations; Utilities. Optional
extension slots remain reachable without breaking the adjacency of Home/quick
actions or the relative order of the named sections. Office retains its local
navigation and suppression of the Kanban task and automation sections.

Add an explicit phone disclosure option to `MobileIntegrationsSection`; leave
its default/wider behavior unchanged. Use a local initially-false expanded state,
semantic Button, aria-expanded/aria-controls and stable body ID, using existing
Tasks/Automations heading tokens. Both provider rows and settings belong inside
the collapsed body. Keep the heading even for an empty provider list. Do not
snapshot destination arrays into local state or gate availability subscriptions
on expansion. The shared resolver remains responsible for permission, plugin and
workspace changes. No domain mutation, credential changes, persistent preference,
API or database change is needed.

Use `phoneNavigation` to order Settings before Stats in the utility projection;
do not change the global destination manifest. Conditional Status remains above
those links; theme, Improve Kandev and conditional Health issues follow them.
On phones, Utilities follows Integrations with the normal section gap; it must
not use an automatic top margin to consume unused drawer height. Wider menus
retain their footer anchoring. Keep the fixed drawer header, one scroll body and
safe-area inset. Keyboard
activation toggles sections; Escape still closes the menu and restores its trigger.

Validate ordered geometry and both quick launch outcomes from Home and task
workbench, disclosure keyboard/touch behavior, empty/configured integrations,
workspace switches, long translated labels, and unchanged 768px/wider composition.
Reuse the populated isolated preview and reapply its mock seed after any restart.
