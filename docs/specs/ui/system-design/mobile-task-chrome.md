---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-TASK-CHROME-001
created: 2026-08-24
updated: 2026-09-22
owners:
  - Kandev
---

# Mobile Task Chrome System Design

## Purpose and boundaries

This design removes two phone-only top-bar entry points whose scope is owned
elsewhere: saved desktop layouts and general Git operations. It keeps task
movement in the existing task drawer and Git operations in the existing Changes
surface. It also moves plugin-provided session status and actions from the fixed
phone header into the shared app menu while retaining their inline tablet and
desktop presentation. No backend, store, persistence, permission, or
responsive-routing contract changes.

## Requirement mapping

| Requirement                     | Design section                                                                                                                                                    |
| ------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-UI-MOBILE-TASK-CHROME-001` | [Components and responsibilities](#components-and-responsibilities), [Interaction flow](#interaction-flow), and [Mobile design contract](#mobile-design-contract) |

## Current mismatch

`SessionMobileTopBar` currently renders `LayoutPresetSelector` with a mobile
mode even though layout profiles configure desktop Dockview. That mode cannot
apply a layout; it only exposes saved-layout management from unrelated task
chrome.

The same top bar also renders `GitActionsDropdown` on every phone panel. Its
commit, change-request, pull, push, rebase, merge, and contribution-recovery
commands duplicate capabilities already owned by `MobileChangesPanel`. The
shared app menu and title-triggered task picker expose task-row actions,
including **Move to**, so replacing the Git ellipsis with another task menu
would create a second path to the same commands.

`SessionMobileTopBar` still renders every `chat-top-bar` plugin registration
inside a non-shrinking action cluster. The slot accepts an unbounded number of
opaque plugin components and does not tell them whether the host is the desktop
top bar or the phone surface. Multiple compact contributions can therefore
consume most of a phone header even without causing document overflow, leaving
the title and branch summary technically present but no longer useful. The
listing header already avoids this failure by putting `main-top-bar`
contributions in the shared phone menu and passing an explicit presentation.

## Components and responsibilities

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx` keeps task title,
  repository/branch summary, applicable first-party status controls, approval,
  and the shared app-navigation trigger and title-triggered task picker. It does
  not mount layout, Git action, or plugin contribution surfaces inline. It passes
  the current task plugin contribution into the app menu instead. Phone triggers
  retain 44px touch targets.
- `apps/web/components/task/task-top-bar-plugin-actions.tsx` uses the public
  `ChatTopBarSlotProps` contract. It keeps desktop rendering unchanged and adds a
  mobile presentation that wraps contributions, constrains direct children to
  the available menu width, and applies the phone minimum target to host buttons.
  A reactive registration-presence hook prevents an empty Plugins section.
- `apps/web/components/navigation/app-nav-sheet.tsx` and
  `apps/web/components/navigation/app-nav-sections.tsx` accept optional
  page-scoped plugin content. `MobilePluginNavSection` renders that content and
  plugin navigation destinations under one localized **Plugins** heading after
  workspace actions and Automations and before Integrations. Arbitrary plugin
  interaction does not dismiss the menu; plugin links retain normal dismissal.
- `apps/packages/plugin-sdk/src/index.ts` exports `ChatTopBarSlotProps` with a
  `presentation: "desktop" | "mobile"` field. The mobile value means the
  contribution is mounted in the shared phone menu; the desktop value means the
  contribution remains inline in the task top bar. Existing task, workspace,
  active-session, and session-list fields remain unchanged.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` remains the
  shared task-list and row-action owner for embedded Tasks and the title picker.
- `apps/web/components/task/mobile/mobile-changes-panel.tsx` continues to
  compose the shared `ChangesPanelHeader` and `ChangesPanelBody`. Those shared
  components retain commit, push, change-request, pull, rebase, merge, and
  remote-contribution actions through the existing `VcsDialogsProvider` and
  Git handlers.
- `apps/web/components/task/layout-preset-selector.tsx` becomes explicitly
  desktop-only at its call boundary. Its unused mobile prop and conditional
  menu branches are removed while desktop preset, reset, save, apply, and
  delete behavior stays intact.
- Phone-top-bar-specific Git menu, dialog, push-submenu, and contribution-drawer
  modules are deleted after their only production consumer is removed. Shared
  Changes/VCS modules remain the capability owners.

The phone title's branch and diff summary still reads existing session Git
status and commits. Its small aggregation helper moves into the surviving
top-bar module or another existing shared utility; retaining summary data does
not retain action ownership.

## Interaction flow

### Task action

1. User taps the hamburger to open shared app navigation and its embedded Tasks
   section, or taps the task title to open the separate inset task picker.
2. The existing task list presents saved views, filters, and task rows.
3. User opens active task row's visible action menu.
4. Existing `TaskMoveContextMenuItems` moves the task to another permitted
   workflow step.

Both entries reuse the same task-row actions and mutation path.

### Plugin contribution

1. On tablet or desktop, `TaskTopBarPluginActions` passes
   `presentation: "desktop"` and renders each `chat-top-bar` registration in the
   existing inline cluster.
2. On a phone, the persistent header renders only its first-party controls and
   shared app-navigation trigger. The reactive slot-presence check supplies the
   plugin contribution to `AppNavSheet` only while registrations exist.
3. The user opens the hamburger menu and reaches the contribution in the shared
   **Plugins** section. `TaskTopBarPluginActions` passes
   `presentation: "mobile"` with the same task and session identity.
4. The menu's single vertical scroll owner contains wrapping contributions and
   plugin navigation rows. Plugin-owned controls keep their own interaction and
   disclosure state; selecting a plugin navigation link closes the menu.

### Git action

1. User selects **Changes** from existing phone bottom navigation.
2. `MobileChangesPanel` renders shared Changes header and body controls.
3. User invokes the applicable Git or change-request action.
4. Existing shared hooks, dialogs, eligibility rules, feedback, and
   remote-contribution confirmation execute unchanged.

The global responsive treatment for Radix menus continues to contain phone
menus inside the viewport. Removing the top-bar duplicate does not change Git
state or operation semantics.

## Mobile design contract

- **Desktop outcome and mobile entry point:** Desktop and tablet top bars remain
  unchanged. Phone task and plugin contributions enter through the hamburger
  menu; Git actions enter through bottom-navigation **Changes**.
- **Nearest shipped exemplars:** `SessionTaskSwitcherSheet` contributes the
  inset task drawer and visible task-row actions. The listing
  `MainTopBarPluginActions` mobile presentation contributes menu placement,
  explicit presentation context, wrapping, and touch geometry. `MobileChangesPanel`
  contributes the focused, one-dimensional Git surface and its single internal
  scroll owner.
- **Information hierarchy and primary action:** Task identity remains first,
  followed by essential first-party status/actions and one app-navigation
  control. Optional plugin status/actions move into the existing Plugins group.
  Layout management and Git operations do not compete with the active panel.
- **Presentation choice and rationale:** No replacement surface is added. Task
  choices and optional plugin contributions share the existing drawer because
  they are temporary contextual navigation, status, and actions. Dense Git
  content remains in Changes because it needs repository, file, commit, and
  remote-state context.
- **Scroll, viewport, safe area, and touch:** Existing fixed top bar,
  `h-dvh` workbench, panel scroll owner, bottom navigation, drawer safe area,
  and menu containment remain. Removing fixed plugin content protects the task
  identity width. The retained app-menu trigger has a 44-by-44 CSS-pixel hit
  target; plugin controls created with `host.ui.Button` have at least a 44
  CSS-pixel active target in the menu.
  Standalone Changes recovery controls and their dialog actions retain 44
  CSS-pixel touch targets through the full phone range below `md`.
- **Shared state and logic:** Phone and desktop share task, session, plugin, and
  Git state, mutations, eligibility, and feedback. Only the phone plugin
  presentation and placement differ.
- **Mobile Playwright proof:** Tests install a real fixture plugin, create a
  long-title task, and assert that plugin contributions are absent from the
  persistent header, present in the menu with mobile context and touch geometry,
  contained with multiple contributions, and leave no document horizontal
  overflow. Existing tests continue to prove the removed layout/Git triggers,
  task movement, Changes actions, and remote recovery at 700 CSS pixels.

## Failure and recovery

- Missing or loading session Git data leaves Changes controls in their existing
  loading/disabled state; it does not reintroduce a top-bar action.
- Archived tasks keep existing read-only presentation and task navigation.
- A plugin render failure remains isolated by `PluginErrorBoundary`; removing or
  disabling the last `chat-top-bar` registration removes the menu contribution
  and does not leave an empty Plugins section unless plugin navigation rows exist.
- Remote-contribution drift continues to fail closed through shared Changes
  action policy and exact confirmation/lease behavior.
- If optional task-list data is still hydrating, the existing task drawer owns
  its loading and recovery states.

## Persistence and compatibility

No persisted layout, task, session, or user-setting value changes. Adding the
`presentation` field is backward compatible for existing structural slot
consumers. Removing the phone selector does not delete saved layouts or alter
desktop defaults. Existing phone panel preference and desktop/tablet layout
state remain unchanged.

## Related specifications and decisions

- [Mobile task navigation](../requirements/mobile-task-navigation.md)
- [Task layout profiles requirements](../requirements/task-layout-profiles.md)
- [Task layout profiles system design](task-layout-profiles.md)
- [Remote contribution tasks](../../tasks/system-design/remote-contribution-tasks.md)
- [Remote contribution head drift](../../../decisions/2026-08-10-remote-contribution-head-drift.md)
- [Local-first contribution replacement](../../../decisions/2026-08-12-local-first-contribution-replacement.md)

## Navigation entry points

The [unified phone navigation package](../../../plans/unified-mobile-navigation/plan.md)
changes the hamburger to app navigation and the task-title button to the existing
task picker. Its [design](../system-design/unified-mobile-navigation.md)
owns that entry-point change; existing task actions, saved-view controller
lifetime, history, and desktop/tablet guarantees here remain compatibility
requirements. Historical hamburger descriptions above describe the preceding composition;
the unified navigation design defines the current entry points.

The September 2026 revision embeds the collapsible Tasks sidebar directly in
the shared phone menu, replacing the Task views action and dedicated pinned
shortcuts. Kanban/Threads/List title dropdowns open display options; Threads
saved-view editing remains inside that surface. See REQ-UI-MOBILE-MENU-004/005
in the unified navigation requirements for the current composition.
