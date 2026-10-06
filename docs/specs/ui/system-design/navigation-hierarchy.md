---
status: current
system: ui
requirements:
  - REQ-UI-NAV-HIERARCHY-001
  - REQ-UI-NAV-HIERARCHY-002
  - REQ-UI-NAV-HIERARCHY-003
  - REQ-UI-NAV-HIERARCHY-004
---

# Navigation hierarchy design

## Boundary and evidence

The 2026-10-05 update restores desktop action placement after feedback on PR #4063.
Source HEAD is `48adb0ce739`; the earlier controls are in `dd7dfa81634^`.
The [placement plan](../../../plans/sidebar-action-placement/plan.md) records that delivery.
The original delivery remains historical evidence for task-panel and phone behavior.

The 2026-10-06 [preferences plan](../../../plans/sidebar-presentation-preferences/plan.md)
extends that delivery with saved presentation choices. Its source baseline is
`40dcfeacb2c98cf650132c7c787340114c1b91ad` (PR #4239). UI owns these reusable
presentation preferences; the existing user-settings service owns storage and updates.

This revises navigation presentation without changing route, task, integration,
or settings ownership. The original investigation used source HEAD
`75a37f34eb6bd0e0a46e9a35a46491bb9f52f7ef` and the supplied annotated references.
That delivery used a neutral creation surface, secondary Chat/Terminal actions,
and compact task rows. This draft changes desktop presentation preferences and
their upgrade defaults, retaining task-panel and phone composition.

At the investigated baseline, Home preceded an ordinary New Task item,
integration links lacked indentation, filter cues represented drafts only, and
task rows were flat. Both default and saved phone composition placed tools after
the embedded task list. This implementation revises that presentation and order.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-UI-NAV-HIERARCHY-001 | Desktop composition; Existing authority; Footer |
| REQ-UI-NAV-HIERARCHY-002 | Task panel; State groups and task rows |
| REQ-UI-NAV-HIERARCHY-003 | Phone composition; Handoff and recovery; Verification |
| REQ-UI-NAV-HIERARCHY-004 | Saved sidebar preferences; Desktop composition; Phone composition |

## Existing authority

Continue resolving destinations through `core-destinations.ts`,
`useStaticDestinations`, `useAppDestinations`, and the existing sidebar layout
projection. Do not introduce a parallel provider list or infer availability from
the screenshot. Built-in integration sections exclude separately projected plugin
nodes in saved layouts; custom shortcut groups keep their header actions.

`models.DefaultSidebarLayout` in
`apps/backend/internal/user/models/sidebar_layouts.go` remains the canonical
uncustomized layout. Keep its New Task node before Home and align the unsaved
`AppSidebarPrimaryNav` fallback. No layout-node schema/version change is needed.
Do not reorder saved nodes. `SidebarLayoutNavigation` must project
eligible visible Inbox entries without displacing New Task from the top of the default
layout; user-defined desktop order continues to win.

Retain `SidebarView`, `SidebarViewDraft`, `useEffectiveSidebarView`,
`selectSidebarViews`, and workspace-specific synchronization. No grouping default
or saved view is migrated. The demo explicitly selects state grouping.

## Saved sidebar preferences

Add two fields to the existing user settings contract:

| Stored/API key | Client state | Values and new-user default |
| --- | --- | --- |
| `sidebar_fast_actions_enabled` | `sidebarFastActionsEnabled` | Boolean, `false` |
| `sidebar_new_task_style` | `sidebarNewTaskStyle` | `simple` or `compact`, `simple` |

These are account presentation preferences, independent of workspace-keyed
`SidebarLayout` nodes and revisions. They are not runtime feature flags.
Use pointer fields for PATCH input so omission differs from explicit false.
Reject null, wrong types, and unsupported style values before saving any changes.
Extend `models.UserSettings`, `dto.UserSettingsResponse`, the update DTO,
service patch application, store marshal/scan helpers, settings-event payload,
Go SPA boot-state mapping, and the user-preferences settings catalog. Regenerate
both settings-contract snapshots so mutable field inventory stays complete.
Follow the existing revision/CAS behavior. Omitted fields in unrelated patches
must preserve the saved values. No new endpoint or event is required.

### Upgrade and new-user initialization

`sqliteRepository.initSchema` applies `runMigrations` before `ensureDefaultUser`.
Use that ordering to backfill existing rows in the `users.settings` JSON blob.
For each absent preference, add the legacy value independently: enabled fast
actions and `compact`. Preserve explicit values, unknown JSON keys, saved layouts,
and task views. Do not decode into a partial settings struct and rewrite the blob.
Keep the backfill transactional and compatible with SQLite and PostgreSQL through
the existing dialect-neutral repository. Increment `settings_revision` only for
changed rows and honor concurrent-write detection. A malformed settings blob or
write failure must roll back the migration and fail initialization with context.

Seed `false` and `simple` explicitly in both `ensureDefaultUser` and `CreateUser`
when inserting a user. Fresh schema initialization has no existing user rows to
backfill. New accounts in upgraded databases also receive explicit new defaults.
Reopening either database must preserve seeded values. Missing individual fields
in a legacy blob get only that field's backfill. No database-age heuristic,
browser-storage flag, or reset on normal settings reads is needed.

`defaultUserSettings` supplies new defaults after migration. Extend
`UserSettingsState`, HTTP types, `default-state.ts`, and
`mapUserSettingsData`/`mapUserSettingsResponse` in `lib/ssr/user-settings.ts`.
WebSocket updates already reuse `mapUserSettingsData`; retain stale-revision
filtering and partial-update preservation. Boot hydration must use the server's
saved values before projecting the sidebar.

### Settings editing

Place a Sidebar appearance group in `LayoutSettings`' existing Sidebar tab,
beside the embedded `SidebarLayoutEditor`. Use `SettingsRow` and existing
Switch/Select primitives with localized labels and help. Add a focused settings
save contributor for the two preferences using `useSettingsSaveContributor`.
This group is user-scoped and remains usable without an active workspace.
The workspace layout editor keeps its own draft and save contributor.

Follow the existing appearance-settings draft/patch/rebase pattern. Save only
changed fields, apply the authoritative response, preserve newer local edits,
and keep drafts on failure. Discard restores the latest saved values. A settings
event may update clean draft fields but must not erase dirty ones. Do not add
local Save buttons, immediate persistence, or a layout-reset side effect.

## Desktop composition

Keep `AppSidebarHeader`'s compact brand, workspace picker, and collapse row.
The expanded header uses an 18px bold Kandev brand and a 12px workspace trigger
label, vertically centered within the existing header. Scope the smaller type
to this trigger; phone workspace selection and dropdown options retain their
existing readable typography and touch targets.
Preserve the 320px default expanded width, resize behavior, 56px rail, and
settings takeover. `AppSidebarNewTaskItem` reads both presentation preferences.
The `compact` style uses the current `AppSidebarNavItem`, `IconSquarePlus`,
left-aligned 13px label, neutral hover treatment, and 36px navigation-row height.
The `simple` style uses an ordinary neutral Button with a centered creation
icon and label at 28px on fine-pointer desktop. Remove oversized min-height and
padding overrides from this design. Coarse pointers retain at least 44px.
Keep the configured keyboard binding and omit a visible shortcut hint.
Reuse the request subscription, workspace-specific dialogs, and collapsed rail control.
`NewTaskButton` remains the phone creation primitive with its existing touch sizing.

When fast actions are enabled, place Quick terminal then Quick Chat to the
right of New Task in the same row for either style. Omit the labelled utility bar.
Use `SurfaceAction` from `components/actions/surface-action.tsx` for compact 24px desktop
icon controls and 44px coarse-pointer targets.
Keep Quick Chat activity indicators and localized activity names.
Keep keyboard and hover tooltips without invoking actions on focus.
Keep the icon controls separate from the creation button.
When fast actions are disabled, use a shared-boundary equal-width utility bar
below New Task with labelled Quick Chat then Terminal. Its fine-pointer desktop
height is 28px, and its coarse-pointer active targets are at least 44px.
Use existing localized labels and launch callbacks. Never render both utility
presentations together. The collapsed rail retains its current creation launcher
and does not gain the expanded-only utility bar.
Workspace plugin controls follow them with the same slot props and error boundaries.
Use a wrapping plugin container so excessive registrations cannot overlap or
squeeze the built-in actions. Keep the built-in row together.
The phone `MobileQuickActions` retains its labelled utility bar and existing handoff.
No new dialog host, launcher, or saved-layout projection is introduced.

Add a navigation presentation to `AppSidebarSection` or a small composed header
using its existing collapse state. Apply it only to built-in Automations,
Canvases, and Integrations: sentence-case label, leading icon, trailing chevron,
and indented children without vertical guide lines. Tasks and Office headings
keep their distinct section treatment. Disclosure and any separate action controls remain siblings, never
nested buttons. Do not impose changes on `ShortcutSection`'s agreed icon header.

For `NavigationSectionHeader`, use a flexible labelled toggle, a sibling header-action
slot, then a separate rightmost chevron toggle.
The label owns `headerRef`, `aria-expanded`, and `aria-controls`.
The redundant chevron stays outside keyboard order and the accessibility tree,
and performs the same toggle when clicked. Its coarse-pointer hit area is 44px.
This restores the `headerAction` ordering already used by `SectionHeader`.
When fast actions are enabled, the Canvases settings shortcut remains visible
in both disclosure states. Otherwise omit the shortcut. Add a labelled canvas
settings child link when shortcuts are disabled so management remains reachable
even when the canvas list is populated or cannot load. Preserve the existing
empty-state creation action. Gate only the optional shortcut, not the section,
canvas list, subscription, or task-create launcher.
The shortcut keeps its current workspace route and never calls the toggle.
Automations and custom section children retain their current presentation.

Place Open automations after the expanded group's rows, using the same labelled
child treatment as Integration settings. The header only toggles the disclosure.
The phone wrapper owns the body and its final destination so saved-layout rows,
empty states, loading, and errors all retain that link. It uses the existing
`/automations` route and dismisses the phone menu on navigation.

When fast actions are enabled, render `IntegrationHeaderShortcuts` using eligible first-party entries from the
already resolved `visibleDestinations`. Place them in `headerAction` before the chevron.
Keep the old four-shortcut desktop capacity, manifest order, accessible labels,
and tooltips. On coarse pointers, show at most two shortcuts with 44px hit areas.
Use `useResponsiveBreakpoint().isFinePointer` for this capacity choice.
Remaining providers retain their named child links.
Plugin entries retain saved-layout ownership and cannot displace built-in header shortcuts.
Keep named children under the disclosure and offer the existing
`workspaceSettingsHref(workspaceId, "integrations")` setup path even with no
eligible providers. Derive active link state semantically (`aria-current`) and
visually. Home's current state covers `/`, `/tasks`, and `/threads` in regular
workspaces while its href still respects startup/workspace settings.

## Footer

`AppSidebarFooter` retains settings route/takeover coordination and unsaved-draft
guards. Its expanded footer is a single nonwrapping row: authenticated account
avatar when present, a growing labelled Settings action, Stats, theme toggle, and More
Actions. The account menu displays the actual identity and existing logout action;
no account is fabricated in no-auth installs. A connection warning remains visible
when the status bar is disabled.

Resolve Stats from `useStaticDestinations("sidebar", "insights")` by its built-in
`stats` identity. Render its icon, resolved label, and href through `FooterIconButton`
immediately before `ThemeToggle`, with `sidebar-stats-button` as its test identity.
Exclude only that destination from `SidebarFooterMenu`.
Plugin IDs remain namespaced, so a plugin's local `stats` ID cannot be excluded.
The expanded footer and collapsed rail both expose the direct Stats control.

Reuse the existing dropdown primitives for one labelled utilities menu containing
eligible plugin insight destinations, Improve Kandev, and release notes
when available. The More Actions trigger retains the unseen-release indicator.
All insight destinations keep their registered order, labels, test identities,
and navigation handlers. No plugin count changes the footer's geometry. Menu items have 44px hit targets
on coarse pointers. Saved
layouts still own their projected plugin entries, preventing duplicates. The old
three-inline-plugin partition and its constant are removed. This supersedes the
inline budget and icon-row presentation in the plugin footer design while keeping
its registration, availability, identity, ordering, and phone parity contracts.
The rail uses the remaining menu with square triggers. Phone utilities remain labelled
rows in the existing drawer; no desktop overflow menu is projected onto phones.

## Task panel

`TasksSection` stays outside optional navigation layout nodes. Add a restrained
separator before its heading and keep its flex-grow task scroller.
`TasksViewPicker` and `SidebarFilterBar` use the same effective view when deriving
filter state. Applied filters means a nonempty effective `filters` array; the
implicit archive visibility rule does not count as an explicit filter.
A saved filtered view still displays the primary-color filter cue. A draft gets
the existing unsaved cue with separate accessible copy; both facts can coexist.
Do not confuse sort/group/task-row changes with filtering or change persistence.
Reuse `SidebarFilterPopover` and its desktop/touch editor presentation.

Keep the view selector and filter action available at the contextual heading.
Use ordinary 28px desktop controls, allowing 24px only where the surrounding
inline-control convention requires it. Phone controls meet the 44px minimum.
New labels belong in the existing locale namespaces and all six shipped locales;
generate the Traditional Chinese pair and pseudo locale using repository scripts.

## State groups and task rows

Pass the resolved grouping dimension to `GroupHeader` through `TaskSwitcher` and
`GroupSection`. Only `groupKey === "state"` maps the existing resolved group key
to semantic state presentation. Use existing task-state styling/icon primitives;
do not infer state from translated group labels or the first row. The effective
task-tree state resolution, server page grouping, `matchingCount`, continuation
labels, sorting, and descendant counts remain authoritative. Unknown/non-state
groups use a neutral presentation. Add an accessible expanded state and a compact
tabular count badge without changing group identity.

Each named group has a leading disclosure chevron, semibold heading, count, and
4px of separation before the next group. Disclosure headers use a 28px minimum
on desktop and retain 44px on phones and coarse pointers. The task body is inset
beneath the header without a vertical guide; whitespace carries the hierarchy.
Use per-instance IDs to connect
the disclosure to its labelled group body, including repeated/continued groups.
Workflow-step headings remain neutral; task state is not inferred from their
names. Preserve nested row indentation and strengthen its decorative connector.
The ungrouped body has no extra inset. This shared inline hierarchy also
appears in the phone drawer with the existing scroll owner and touch targets.

Keep `TaskItem`, `TaskItemStatsRow`, and `TaskItemTrailing` as the row anatomy.
Use a transparent, borderless base surface for parent tasks and subtasks. Quiet
token-based hover and active-selection fills, a keyboard-focus ring, and the
existing multiselection ring distinguish interactive states without a permanent
card around every row. Small corner radii apply to these state treatments. Keep
existing vertical padding and at most a small inter-row gap. Avoid additional
wrappers that interfere with drag/drop, row keyboard handlers, context menus,
nested-tree indentation, or scroll anchoring.

The title shrinks/truncates before status/actions. Detail fields and trailing
slots continue to follow `SidebarTaskRowPresentation`, including details-hidden,
time, PR/MR status, missing data, repository-group deduplication, plugin metadata,
and queued-state cues. No duplicate status or timestamp is introduced.
The phone overflow button and group headers measure at least 44px throughout
the below-768px layout, including narrow fine-pointer windows. Coarse-pointer
controls keep that minimum above the breakpoint. The overflow button stays visible and must not
overlap the row's primary tap target or trailing values. State presentation must
not collapse review/completion, clarification, failures, or background work into
the four illustrative screenshot groups.

Command selection retains its brief row cue and keeps that row inside the list
while the viewport or hydrated group content changes height. Observe those
changes only for the cue's lifetime; cancel, replacement, and expiry disconnect
the observer, and a different selected row is never recentered by the old cue.

## Phone composition

Nearest shipped exemplars: `AppNavSurface` for the inset `Drawer`, fixed header,
dynamic viewport and focus return; `SessionTaskSwitcherSheet` and
`InlineTaskHeader` for the contextual task body; `MobileIntegrationsSection` and
`MobileAutomationsSection` for touch disclosures. This is a temporary navigation
choice, so retain the drawer rather than adding a route or bottom navigation.

Use `useResponsiveBreakpoint`'s below-768px phone branch. Keep the menu heading
and workspace picker outside the one `nav` content scroller. The default phone
body becomes:

1. Neutral New Task, Home and eligible visible Inbox links, Quick Chat/Quick terminal.
   Reuse `MobileQuickActions` and its native launch/focus handoff. Within app
   navigation, use secondary ghost actions with 44px touch targets in an
   equal-width utility bar with a shared boundary; translations can wrap.
   Show the shorter Terminal label while keeping
   its full accessible name. Other listing-menu consumers retain their treatment.
2. Eligible Automations, Canvases, Integrations, and plugin/custom tools.
3. Existing labelled page-local navigation, then contextual Tasks.
4. Optional fallback metrics and existing utilities/account actions.

Put tools before the task outlet for both `AppNavSections` and
`MobileSidebarLayoutNavigation`. Saved optional tools retain their relative order
and visibility; extract the visible Home/New Task built-ins for the phone primary
region without writing a new layout. Quick actions remain available when Home
is hidden. Office keeps its own task/local navigation and does not get regular
task views. Preserve plugin toolbar selection and separately customized links.

The labelled New Task control uses the existing `requestNewTaskCreation` path
after the menu closes. Retain one durable dialog host. Suppress only the inline
regular Tasks-header plus when that top action is visible; keep it when the
built-in is hidden. Do not remove the title-picker's own creation action or
explicit custom shortcuts. `MobileTaskNavigationProvider` continues to host
task controllers/dialogs above responsive headers.

Provider issue browsing reuses the actual GitHub page and
`MobileGitHubPage.mobileMenuButton`/Issues selection. Moving Integrations above
the task list removes the task-volume-dependent path. Do not create a second
provider submenu or issue query implementation in app navigation.

## Handoff and recovery

Keep close-before-dialog focus handoff, task-title switching, push versus replace
history, workspace-keyed task controllers, and existing delayed-response guards.
No navigation disclosure starts a task, automation, terminal, or provider call
beyond its normal availability/list subscription. Preserve automation polling
lifetime gates; do not duplicate readers to restyle headings.

Loading/denied/failed task reads remain inside Tasks so integration navigation
works independently. Integration setup is not evidence of a connection. Sidebar
actions in unresolved workspace state remain disabled and show no previous
workspace labels. Unknown groups/metadata fall back to neutral or omitted
presentation. No new API, telemetry, permission, or persisted collapse state is
introduced.

## Verification

Use behavioral component tests for creation/disclosure independence, default
versus saved order, active versus draft filters, and semantic state-group mapping.
Use focused Playwright geometry and action tests for desktop, phone, and the
768px boundary. Test phone Issues access from both listings and a workbench,
with a long task list, empty integrations, saved layouts, and workspace switches.
Include 360/393/767px, narrow fine-pointer input, coarse-pointer tablet targets,
dark/light, Portuguese labels, focus return, and creation-draft rotation.

The [work orders](../../../plans/navigation-hierarchy/plan.md) own exact test
commands and the isolated seeded comparison. The comparison serves the changed
branch, not unrelated current-main marketing content. Record its baseline SHA,
implementation diff hash, URLs, process ownership, seed manifest, mock proof, and
teardown command. Keep it running for the user's comparison until stopped.

## Related contracts and decisions

- [Sidebar customization](sidebar-customization.md)
- [Unified mobile navigation](unified-mobile-navigation.md)
- [Task-row presentation](sidebar-task-row-presentation.md)
- [Workspace sidebar views](workspace-sidebar-task-views.md)
- [Control sizing](control-sizing.md)
- [Navigation manifest boundaries](../../../decisions/2026-08-04-navigation-manifest-boundaries.md)
- [Phone entry-point ownership](../../../decisions/2026-09-15-phone-navigation-entry-points.md)

The existing boundaries remain sufficient; this local composition revision does
not require a new ADR.
