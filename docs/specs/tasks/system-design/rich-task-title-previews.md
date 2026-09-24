---
status: current
system: tasks
requirements:
  - REQ-TASKS-RICH-TASK-TITLE-PREVIEWS-001
---

# Rich task title previews system design

## Purpose and boundaries

The task system owns the task-title preview contract and its interaction with Kanban selection. This design covers the selection-mode guard on that preview. The existing Kanban selection state, card click dispatch, and coarse-pointer navigation remain the sources of truth for their respective actions. No backend, API, or persisted state changes are needed.

## Requirement mapping

| Requirement                              | Design section                                                |
| ---------------------------------------- | ------------------------------------------------------------- |
| `REQ-TASKS-RICH-TASK-TITLE-PREVIEWS-001` | [Selection-mode preview guard](#selection-mode-preview-guard) |

## Selection-mode preview guard

`KanbanCardShell` already receives `isMultiSelectMode` and forwards card clicks to selection handling. It also renders `KanbanCardBody`, whose `enableTitleHover` prop controls whether `CardTitle` mounts `TaskTitleHoverCard`. The shell should disable that prop when multi-select is active. The title remains visible as plain card content so a click reaches the card's existing selection handler.

When multi-select starts, unmounting `TaskTitleHoverCard` closes any open preview, including its pointer and keyboard trigger. When multi-select ends, the title can mount its normal preview again. This uses the existing component lifecycle and does not add a second selection state or change the shared preview component for other consumers.

The same shell serves responsive Kanban cards. Coarse pointers already render the title as direct card content, so the guard preserves the phone task-navigation path and adds no touch-only disclosure.

## Verification boundary

A component regression should prove that selection mode removes the trigger and that it returns after the mode ends. A desktop browser regression should open a preview, enable multi-select, verify the preview closes and does not reopen on hover, and select a card through its title. Existing mobile Kanban coverage should continue to prove direct title navigation without a hover preview.
