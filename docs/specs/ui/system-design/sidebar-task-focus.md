---
status: current
system: ui
requirements:
  - REQ-UI-SIDEBAR-TASK-FOCUS-001
---

# Sidebar Task Focus System Design

## Purpose and boundaries

This design owns the presentation of the active task in the shared task-row
surface. The task/session state supplied through `activeTaskId` and
`selectedTaskId` remains authoritative; this design adds no state, persistence,
or task lifecycle behavior.

The design covers the desktop AppSidebar and the mobile task-switcher sheet
because both surfaces render the same task-row component. It does not own task
colors or the selection state that determines which row is active.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-SIDEBAR-TASK-FOCUS-001` | [Shared row rendering](#shared-row-rendering), [Responsive surfaces](#responsive-surfaces) |

## Shared row rendering

`TaskRowItem` in `apps/web/components/task/task-switcher-row.tsx` continues to
derive `isSelected` from the existing active and selected task IDs. It passes
that value to `TaskItem`, so the focus treatment follows the existing task
identity and never creates a second selection model.

`TaskItem` in `apps/web/components/task/task-item.tsx` owns the row classes and
the leading `SelectionBar`. An active row uses a stronger primary-tinted
surface and a theme-aware primary ring, equivalent to:

```text
bg-primary/15 hover:bg-primary/20 ring-1 ring-inset ring-primary/50
```

The task color remains the color of the leading marker. The primary ring keeps
the active state legible when the marker is red, yellow, or another saturated
color while keeping the focus treatment coherent with the tinted surface.
Inactive rows retain their current hover treatment and marker opacity.

The existing `data-active="true"` and `aria-current` attributes remain on the
active row. Multi-selection continues to use its existing background and ring
semantics, and active rows do not lose their task actions or metadata.

## Responsive surfaces

The desktop path is `AppSidebar` → `TasksSection` → `TaskSessionSidebar` →
`TaskSwitcher` → `TaskRowItem` → `TaskItem`.

The mobile path is `SessionTaskSwitcherSheet` → `MobileTaskList` → the same
`TaskSwitcher` and `TaskItem`. No mobile-only markup, state, scroll owner, or
touch target is needed for this styling-only change. The mobile row remains an
existing primary tap target inside the sheet, and the inset treatment stays
inside the row bounds so it cannot create document-level horizontal overflow.

## Data and persistence

No API, WebSocket payload, store field, task-color value, or persisted setting
changes. The active and selected task IDs already supplied to the row remain
the only inputs to the treatment.

## Failure and compatibility

If no task is active, rows retain their current inactive appearance. If a task
color is missing or invalid, the existing primary marker fallback remains in
place. Theme tokens are used for the active surface and ring, so light and
dark themes do not require separate row state.

## Verification

The existing desktop sidebar-open flow shall verify the active row's
`data-active` state and active treatment. The mobile sidebar task-action flow
shall verify the same treatment inside the task-switcher sheet and retain its
viewport-overflow assertion. The shared `TaskItem` test suite remains a
targeted regression check for row rendering and action behavior.
