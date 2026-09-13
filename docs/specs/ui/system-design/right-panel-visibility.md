---
status: draft
system: ui
created: 2026-09-13
requirements:
  - REQ-UI-RIGHT-PANEL-VISIBILITY-001
owners:
  - kandev
---

# Right panel visibility system design

## Purpose and boundaries

Expose the existing visibility actions from task chrome that remains mounted when panels disappear.
UI owns the layout contract. No backend contract changes.
Reuse [layout profile restoration](task-layout-profiles.md) and the existing device-local exception in
[ADR 0041](../../../decisions/0041-backend-owned-portable-user-settings.md).
No new persistence boundary or ADR is needed.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.1, .4, .7 | Persistent control |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.2, .3 | Layout adapters |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.5 | Persistence and recovery |
| AC-UI-RIGHT-PANEL-VISIBILITY-001.6 | Phone composition |

## Persistent control

Add `TaskRightPanelsToggle` in `apps/web/components/task/task-right-panels-toggle.tsx`.
Place it immediately before `LayoutPresetSelector` in `TopbarToolsGroup` within `task-top-bar.tsx`.
Render it outside the archived-task editor-actions gate when a supported workbench exists.
`task-page-inner.tsx` already excludes the desktop header on phones.
Use `useResponsiveBreakpoint` to select the active layout and pointer sizing.

Use a shared Button and Tooltip with matching accessible next-action text:
`Show right panels` or `Hide right panels`. Set `aria-expanded` from effective visibility.
Use right-sidebar expand/collapse icons and `data-testid="task-right-panels-toggle"`.
The icon stays in one position. ASCII labels represent tooltip/accessibility text rather than mandatory visible text.
Remove the old `RightTopGroupActions` hide control from `dockview-header-actions.tsx` after the persistent control works.
Remove only its obsolete imports and wiring.
Avoid a duplicate right toggle in `DocumentControls` on these task pages; retain unrelated document controls.

`TopbarToolsGroup` currently forces descendant buttons to `h-7`.
Scope that rule so the new coarse-pointer button retains a real 44-by-44-pixel hit area.
Keep title truncation and existing overflow behavior. Do not place the toggle in a disappearing group or overflow menu.

## Layout adapters

A small hook, `useTaskRightPanelsToggle` in `apps/web/hooks/use-task-right-panels-toggle.ts`, selects existing state and actions.
It returns visibility, readiness, and the activation callback. It does not own another visibility boolean.

- Desktop workbench: read `useDockviewStore.rightPanelsVisible`; call `toggleRightPanels`.
  Require the current Dockview API and a settled restoration for readiness.
  Preserve `captureLiveWidths`, chat-scroll preservation, pinned-width enforcement, and the existing save path.
- Compact desktop: remove the early return that blocks showing while `defaultPreset === "compact"`.
  An explicit action can add the standard right column. Keep `useCompactDockviewDefault` and initial compact selection intact.
- Tablet fallback: use `useLayoutStore.columnsBySessionId[effectiveSessionId].right`, defaulting to true.
  Call `toggleRightPanel(effectiveSessionId)` only when that ID exists.
  Derive the same effective session as `useSessionLayoutState`; never write under an empty session key.
- Phone: return no supported desktop control. Do not mutate either wider-layout store.

`SessionTabletLayout` currently renders its right Panel unconditionally.
Render that column only when `layoutState.right` is true, allowing the left content to fill the workbench.
When hidden, do not persist a one-panel geometry over `task-layout-tablet-v1`.
Keep the saved two-panel split for reopening. Preserve the center component identity across this change.
Preview composition and inner Files/Terminal sizing retain their current contracts.

The existing Dockview hide predicate removes entire columns containing any Files, Changes, or Terminal panel.
A persistent control makes mixed custom layouts easier to reach.
Constrain removal to actual right-column ownership, rather than incidental right-panel IDs inside the center column.
Reuse the existing `isRightColumn` boundary. Preserve center Agent and PR Details panels in mixed-state regression tests.
Showing retains `removeRightPanelTabs` and the existing standard right-column insertion without duplicate canonical panels.
Do not introduce arbitrary custom-layout snapshots in this package.

## Persistence and recovery

Dockview restores device-local environment layouts and derives visibility from their shape.
Tablet fallback already persists columns under `layout-columns-by-session` and panel geometry under `task-layout-tablet-v1`.
The new control reads those owners; it adds no backend field or storage key.
Do not synchronize these distinct layout stores on resize or orientation changes.

Disable activation while the current API is absent or restoration is active.
Do not use a stale desktop API while the tablet fallback is mounted.
After restoration, read the current store value rather than retaining a click-time label.
Keep task/session navigation and existing Reset Layout semantics intact.

## Phone composition

Reuse `SessionMobileLayout` and `SessionMobileBottomNav`, the shipped task-domain exemplars.
Files and Terminal are frequent destinations with dense content, so they retain full-screen panel navigation.
Chat remains the route back to the conversation. No new drawer or sidebar is added.
Preserve the current dynamic viewport, safe-area handling, and panel-owned scrolling.

## Validation

Use component and hook tests for wiring, readiness, localization, and focus.
Use store regression tests for compact reopening and mixed center/right columns.
Use browser tests for real width recovery, repeated toggles, reload, pointer sizing, and responsive handoffs.
The [work order](../../../plans/right-panel-visibility/task-01-persistent-toggle.md) owns exact commands and scenarios.
