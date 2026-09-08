---
status: current
system: ui
requirements:
  - REQ-UI-MOBILE-TASK-CHROME-001
created: 2026-08-24
owners:
  - Kandev
---

# Mobile Task Chrome System Design

## Purpose and boundaries

This design removes two phone-only top-bar entry points whose scope is owned
elsewhere: saved desktop layouts and general Git operations. It keeps task
movement in the existing task drawer and Git operations in the existing Changes
surface. No backend, store, persistence, permission, or responsive-routing
contract changes.

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
task drawer opened by `mobile-session-menu` already exposes task actions,
including **Move to**, so replacing the Git ellipsis with another task menu
would create a second path to the same commands.

## Components and responsibilities

- `apps/web/components/task/mobile/session-mobile-top-bar.tsx` keeps task title,
  repository/branch summary, applicable status/plugin controls, approval, and
  the task-drawer trigger. It stops mounting layout and Git action surfaces.
  The retained task-drawer trigger becomes a 44-by-44 CSS-pixel touch target.
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` remains the
  only phone top-chrome path to the task list and its row-level actions.
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

1. User taps retained hamburger control.
2. `SessionTaskSwitcherSheet` opens its existing inset bottom drawer.
3. User opens active task row's visible action menu.
4. Existing `TaskMoveContextMenuItems` moves the task to another permitted
   workflow step.

No new top-bar overflow, picker, state, or mutation path is introduced.

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
  unchanged. Phone task actions enter through the hamburger task drawer; Git
  actions enter through bottom-navigation **Changes**.
- **Nearest shipped exemplars:** `SessionTaskSwitcherSheet` contributes the
  inset task drawer and visible task-row actions. `MobileChangesPanel`
  contributes the focused, one-dimensional Git surface and its single internal
  scroll owner.
- **Information hierarchy and primary action:** Task identity remains first,
  followed by contextual status/actions and one task-navigation control.
  Neither layout management nor Git operations compete with the active panel.
- **Presentation choice and rationale:** No replacement surface is added.
  Task choices remain in the existing drawer because they are short contextual
  navigation/actions. Dense Git content remains in Changes because it needs
  repository, file, commit, and remote-state context.
- **Scroll, viewport, safe area, and touch:** Existing fixed top bar,
  `h-dvh` workbench, panel scroll owner, bottom navigation, drawer safe area,
  and menu containment remain. Removing two controls reduces crowding. The
  retained task-drawer trigger has a 44-by-44 CSS-pixel hit target. Standalone
  Changes recovery controls and their dialog actions retain 44 CSS-pixel touch
  targets through the full phone range below `md`.
- **Shared state and logic:** Phone and desktop continue to share task and Git
  state, mutations, eligibility, and feedback. Only phone composition loses
  duplicate entry points.
- **Mobile Playwright proof:** Tests assert both removed triggers are absent,
  the retained task drawer remains contained and can move the active task, Git
  operations complete through Changes, long titles do not overlap actions, and
  document horizontal overflow stays absent. Remote recovery geometry is also
  verified at 700 CSS pixels, between the `sm` and `md` breakpoints.

## Failure and recovery

- Missing or loading session Git data leaves Changes controls in their existing
  loading/disabled state; it does not reintroduce a top-bar action.
- Archived tasks keep existing read-only presentation and task navigation.
- Remote-contribution drift continues to fail closed through shared Changes
  action policy and exact confirmation/lease behavior.
- If optional task-list data is still hydrating, the existing task drawer owns
  its loading and recovery states.

## Persistence and compatibility

No persisted layout, task, session, or user-setting value changes. Removing the
phone selector does not delete saved layouts or alter desktop defaults. Existing
phone panel preference and desktop/tablet layout state remain unchanged.

## Related specifications and decisions

- [Mobile task navigation](../requirements/mobile-task-navigation.md)
- [Task layout profiles requirements](../requirements/task-layout-profiles.md)
- [Task layout profiles system design](task-layout-profiles.md)
- [Remote contribution tasks](../../tasks/system-design/remote-contribution-tasks.md)
- [Remote contribution head drift](../../../decisions/2026-08-10-remote-contribution-head-drift.md)
- [Local-first contribution replacement](../../../decisions/2026-08-12-local-first-contribution-replacement.md)
