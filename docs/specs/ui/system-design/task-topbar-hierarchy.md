---
status: current
system: ui
requirements:
  - REQ-UI-TASK-TOPBAR-001
---

# Task topbar hierarchy design

## Boundaries and mapping

`TaskTopBar` composes existing domain controls through `PageTopbar`. This change
owns their presentation only. REQ-UI-TASK-TOPBAR-001 maps to the components and
responsive behavior below; existing mutation hooks remain authoritative.

## Composition

- Preserve the measured breadcrumb/title and adaptive `WorkflowStepper`.
- Keep review, plugin, unarchive, Office, and task action controls inline.
- Keep `TaskRightPanelsToggle` directly accessible. Put `LayoutPresetSelector`,
  `EditorsMenu`, `OpenTaskFolderButton`, and the existing debug toggle in a
  `TaskTopBarTools` disclosure. Label each row; reuse controls and their disabled
  states rather than duplicating action handlers. The editor and folder stay
  adjacent in the Workspace row. Applying or resetting a layout and successfully
  opening an editor close Task tools.
- Give `TaskAssigneeControl` a compact presentation for this header. Use
  `ComboboxOption.renderTriggerLabel` to show an avatar/icon while preserving
  searchable full labels and assignment semantics in the picker.
- Render `TaskTopBarMetrics` in the task header. Other `TopbarMetrics` callers keep
  the inline fallback. The task component uses `StatusSurfaceMetrics` in its grid
  presentation within the disclosure. Mount the metric content only while open,
  so a closed disclosure does not acquire an unnecessary subscription.

## Responsive interaction

Use a shared task-chrome disclosure wrapper with a fine-pointer `Popover` and
a `useTouchDrawer`-selected `Drawer`, following `CompactWorkflowDisclosureSurface`.
Desktop triggers use 28 px controls and omit the Task tools text below `xl`; coarse-pointer triggers use 44 px targets.
The touch drawer has a fixed title, one internal scroll region, and safe-area
padding. Reuse Radix focus and Escape handling; test nested picker interactions.

Phones retain `SessionMobileTopBar`, native navigation, and existing Status
drawer. The nearest exemplars are the compact workflow drawer and phone menu.
They share domain state with the desktop header without mounting that header.

## State and recovery

Disclosure state is local and ephemeral. Closing does not persist a preference.
Existing controls retain error handling, loading state, authentication, and
executor availability. No backend API, data migration, or new telemetry is needed.

## Related contracts

- [App status bar](app-status-bar.md): fallback presentation and user settings.
- [Mobile task chrome](mobile-task-chrome.md): native phone composition.
- [Implementation plan](../../../plans/task-topbar-hierarchy/plan.md).
