---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-001
  - REQ-UI-SIDEBAR-CUSTOMIZATION-002
  - REQ-UI-SIDEBAR-CUSTOMIZATION-003
  - REQ-UI-SIDEBAR-CUSTOMIZATION-004
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
---

# Sidebar Customization Design

## Ownership and existing boundaries

UI owns navigation composition and personal layout. Automation activity,
canvas lifecycle, plugin registrations, and workspace access keep their owners.
This extends the portable settings pattern from
[workspace sidebar task views](workspace-sidebar-task-views.md).

`AppSidebarModeNav` currently assembles fixed sections. `AppSidebarPrimaryNav`
owns Home and New Task. `TasksSection` owns the remaining scroll space.
`AppSidebarSettingsMode` temporarily replaces navigation on settings routes.
`MobileMenuSheet` and `AppNavSections` compose the phone menu separately.

Static links already use `lib/navigation/core-destinations.ts`,
`resolveDestinations`, and plugin registrations. Reuse these identities and
availability rules. Do not create another table of hrefs or change global
catalog order to implement a personal layout.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-SIDEBAR-CUSTOMIZATION-001 | Persistence; Composition |
| REQ-UI-SIDEBAR-CUSTOMIZATION-002 | References and catalog; Composition |
| REQ-UI-SIDEBAR-CUSTOMIZATION-003 | Activity |
| REQ-UI-SIDEBAR-CUSTOMIZATION-004 | Persistence; Editor; Recovery |
| REQ-UI-SIDEBAR-CUSTOMIZATION-005 | Phone; Verification |

## Persistence

Proposed new contracts are marked here; they are not existing symbols.
Add `SidebarLayoutsByWorkspace` to `internal/user/models.UserSettings`, exposed
as `sidebar_layouts_by_workspace`. Store entries in existing user-settings JSON,
using the current settings revision and CAS transaction. No new table is needed.

A proposed `SidebarLayout` contains `version: 1`, `revision`, and ordered `nodes`.
Each node has a stable client-generated ID and a `visible` boolean:

- `builtin`: a registered layout entry ID, such as `home` or `integrations`.
- `plugin`: an owner-qualified destination ID for an original plugin nav entry.
- `shortcuts`: a nonempty display name and ordered shortcut references.

A shortcut has an instance ID and a discriminated target:
`destination {id}`, `host_action {id}`, `canvas {id}`, or `automation {id}`.
Names and icons for targets resolve at runtime; they are not copied into settings.
Section names are user data. Never serialize React nodes, callbacks, tokens,
provider credentials, resource content, or arbitrary executable strings.

The proposed PATCH object is `sidebar_layout_state` with explicit `workspace_id`,
`expected_revision`, and a complete `layout`; `layout: null` means reset.
Omission leaves settings unchanged. Reject whole-map replacement.
Resolve the authenticated user through the existing user-settings path and
validate workspace access using the scoped-sidebar service dependency.

Each CAS retry reads fresh settings and changes only the addressed workspace.
Compare its layout revision; a mismatch returns a conflict without changing it.
Keep a revision tombstone after reset to reject pre-reset delayed writes.
Concurrent changes to unrelated settings or another workspace remain intact.
Validate the full replacement atomically. Reject unsupported schema versions,
unknown node/target kinds, invalid IDs, and protected built-in nodes.
Retain syntactically valid unresolved resource and plugin references.

Implementation limits: 20 shortcut groups per workspace, 20 shortcuts per group,
100 total shortcuts, and 1-60 Unicode code points per trimmed section name.
Reject duplicate node/instance IDs and duplicate targets within one group.
The same target may appear in different groups. Mirror validation in the editor;
the server remains authoritative.

An absent layout resolves to a backend-owned canonical default preserving current
ordering and visibility. No legacy import is needed: task views and collapse
preferences are separate. The canonical layout skeleton and version must reach
the client with complete settings; do not invent defaults in components.

Extend model, DTO, PATCH mapping, store serialization/deserialization, HTTP,
boot payload, WebSocket projection, and `mapUserSettingsData` in `lib/ssr/user-settings.ts`.
Expose only accessible workspace entries. Add settings-catalog discovery and
agent-settings schema coverage through existing domain machinery. Do not create
a parallel API or MCP-specific preference implementation.

## References and catalog

Create proposed `lib/sidebar/layout-types.ts`, `layout-operations.ts`, and
`shortcut-catalog.ts`. The catalog adapts existing resolved navigation data,
not a second navigation manifest. Preserve `pluginDestinationId` ownership:
`plugin:<encoded plugin ID>:<encoded item ID>`.

Include eligible registered main, integrations, and sidebar-footer plugin links.
Their original positions can be hidden independently of pinned references.
Unregistered slot components are not selectable. No plugin SDK extension is
required to pin registered links. GitHub resolves through existing integration
availability. Slack resolves only through an installed plugin registration.

Resource adapters resolve active workspace canvases and automations by ID.
Canvas eligibility follows `isActiveWorkspaceCanvas`; use `canvasHref` and
existing automation history routes. Access or feature gates always take
precedence over personal visibility. Captured workspace identity accompanies
picker loads and action callbacks; late results cannot cross workspace scope.

The host action allowlist initially includes New Task, Quick Chat, and Quick
Terminal through existing launchers. Each invocation remains an explicit click.
No action transport is persisted. Pinned automations only navigate to history.

Settings holds a searchable catalog grouped by Built-in, Plugins, Canvases,
and Automations. Availability and resource loading have explicit pending/error
states. A failed catalog load is not treated as an empty resource collection.

## Composition

Apply layout ordering as a surface-specific projection after destination
resolution. Leave command palette ordering, feature eligibility, startup page,
brand links, and settings navigation unchanged. Defaults preserve existing order.
Newly registered plugin destinations append in their canonical location until
customized. Retain hidden entries rather than treating omission as hidden.
Unknown future node kinds must not be written back by an older editor.

Render optional nodes above the fixed Tasks region in regular workspaces.
Keep Office-only regions in their existing order; apply common customizable
nodes before them. Required Inbox/Needs you entries retain their existing mode
and feature gates and cannot be hidden in this editor.

Create proposed `ShortcutSection` with sibling disclosure and action controls.
Do not nest links/buttons inside the disclosure button. Name and chevron toggle
only expansion. Header icons activate only their target. The expanded list uses
the same resolved array, including order, identity, unavailable state, and status.
Use existing section collapse mechanics with workspace/group-qualified IDs.
Collapse remains device-local and separate from portable layout revisions.

The header reserves the name/disclosure area and fits as many 28px icon controls
as remaining width permits. Four icons fit the ordinary expanded desktop width.
Extra icons enter a More menu; they remain listed when expanded. Overflow never
shrinks controls below their applicable pointer size. Full names appear on
hover/focus and in accessible labels. In the 56px rail, render one group launcher
opening a labelled menu of all shortcuts. These menus use existing responsive
primitives. Empty groups remain visible as named disclosures with empty content;
the settings editor supplies Add shortcut. Never add or run anything implicitly.

The agreed icon header and labelled expanded list are fixed interaction choices.
An optional icon-plus-label header style is deferred: it would crowd this layout.

## Activity

Reuse `automationState(automation, openRuns)` from `automation-rows.ts`.
The current source returns running, idle, or paused. Running wins when open runs
exist, even if the automation is disabled. Preserve these meanings; do not map
paused to failed or invent waiting/error lifecycle states.

Reuse the bubble geometry of `QuickChatActivityIndicator`: an overlaid circular
marker with a background ring. Extract a presentation-only primitive if needed;
keep Quick Chat selectors, test IDs, and unread semantics unchanged. Automation
markers use existing state colors: blue running, green idle, muted paused.
No new animation is needed. A tooltip and accessible name provide state text;
expanded rows also show the label. Unknown/loading/error is visually distinct
from all three domain states and never passes through an idle fallback.

Use one proposed workspace activity controller per mounted navigation surface,
shared by all shortcut groups and the visible automation section. Adapt existing
`useWorkspaceAutomations`, `useAutomationSummaries`, and `useLiveRefresh`.
Do not create one request loop per button. Poll summaries every ten seconds
while at least one pinned automation is visible, including folded groups and
rail/overflow launchers. Poll idle pins too, so later starts become visible.
Refresh immediately on mount, workspace change, reconnect, and return to visible
navigation. Pause when the document or phone menu is hidden and no consumer needs
activity. Invalidate/refresh automation definitions for pause changes too.

Keep request generations and workspace identity. Failed refreshes show unknown
status rather than an authoritative stale running/idle marker; retry at the next
refresh or through the normal recovery control. Successfully loaded summaries
with no record may use existing never-run semantics. An unloaded collection may
not. Aggregate a running bubble on More and collapsed-rail group launchers when
any shortcut behind them is running. Their accessible text identifies activity;
opening the menu shows each resource's state.

## Editor

Add an Appearance entry opening `/settings/sidebar` and register it in SPA routes
and settings discovery. This dedicated page has the workspace name, explanatory
copy, layout nodes, a draft preview, and Restore defaults. Register one
workspace-bound contributor with `useSettingsSaveContributor`. Use the existing
floating Save changes and discard/navigation flows; no page-local save buttons.
The live sidebar retains saved state while the editor previews drafts locally.

Rows have visibility switches and reorder handles. Shortcut groups additionally
have a name, ordered contents, Add shortcut, remove, and move-to-group controls.
Use the repository's existing drag-and-drop dependency and keyboard sensors.
Explicit Move up/down and Move to group controls provide an alternative.
Do not implement drag-to-execute or navigation while dragging.

Capture the workspace and base layout revision on edit. A conflicting save
keeps the draft and offers reload/discard or deliberate reapplication after the
latest revision is loaded. Do not retry by silently overwriting another client.
A workspace switch invokes existing dirty-state protection; after discard or
successful save, rebind the editor. Stale callbacks cannot write the new scope.

## Recovery

Unavailable targets keep their position with a generic icon and localized
Unavailable label. A disabled menu item explains the missing capability without
showing cached private resource details. They can be removed or replaced in
settings. Temporary plugin loading is distinguished from completed registration
without a matching target. Returning resources re-resolve the saved identity.

Save failures retain draft and authoritative saved layout. Unsupported schema
versions preserve stored data and offer refresh rather than destructive repair.
Malformed stored known-version data falls back to defaults for display without
overwriting the stored entry on read; surface a reset/recovery notice in settings.
No-workspace state disables editing and never displays a previous workspace's
resource names. Reset remains an explicit saved operation.

## Phone

Nearest exemplar: `MobileMenuSheet`, with an inset Drawer, fixed header, and one
internal scroll region. Reuse this navigation surface through `AppNavSections`.
A phone group uses a full-width disclosure row, followed by a compact icon strip;
expansion reveals labelled shortcuts beneath it. The extra line preserves 44px
hit targets and name space. More contains overflow, without horizontal page scroll.

The editor uses direct full-page settings navigation because it is a multi-step
editing task. Show the layout list, then a focused section editor, then a picker;
Back returns to the draft without losing changes. Retain the shared save control
and guard. Use `100dvh`, one active scroll owner, and safe-area padding. Shared
operations and state power both presentations. No desktop preference is rewritten
because the phone needs a different composition or overflow count.

## Verification and documentation

Tests cover validation, scoped CAS and revisions, reset, default projection,
reference resolution, layout operations, fetch readiness, delayed callbacks,
editor behavior, navigation rendering, and activity states. The dedicated
desktop and phone sidebar customization E2E files cover the mixed four-shortcut
editing flow, saved navigation, phone composition, and a pinned automation
status transition. Existing desktop and phone E2E regression suites continue
to cover the established automation list, settings navigation, phone access,
and 44px touch sizing. Broader overflow and plugin lifecycle scenarios remain
represented by the named unit/component suites rather than unrun browser files.

Implementation updates `docs/public/use-kandev.md` with navigation,
hide-versus-disable semantics, persistence, and recovery. The public guide and
coverage manifest are validated with the documentation checks.

## Related decisions

- [Portable settings](../../../decisions/0041-backend-owned-portable-user-settings.md)
- [Navigation manifest boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)

This design applies existing ownership rules. The preference model and local
tradeoffs fit this document; no additional ADR is required.

## Delivery

- [Plan and work orders](../../../plans/sidebar-customization/plan.md)
