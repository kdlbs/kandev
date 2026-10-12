---
status: current
system: ui
requirements:
  - REQ-UI-PLAN-EDITOR-TASK-SWITCH-001
---

# Plan Editor Task-Switch Stability System Design

## Purpose and boundaries

This design keeps plan-search work inside the lifetime of the current TipTap
editor. It prevents an outgoing editor from crossing a task boundary. It does
not change plan data, task navigation, layout restoration, or route recovery.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-PLAN-EDITOR-TASK-SWITCH-001` | [Editor ownership](#editor-ownership), [Search command gate](#search-command-gate), [Verification](#verification) |

## Confirmed failure

`TaskPlanPanel` keeps the TipTap `Editor` in parent state. The keyed editor
child is destroyed when the selected task changes. The parent can render once
with the old, non-null editor before the new child reports its editor.

TipTap 3.31 clears `commandManager` during `Editor.destroy()`. During that short
window, the closed-search effect reads `editor.commands.clearPlanSearch()`.
The `commands` getter then reads the cleared manager and throws. The route error
boundary contains the exception and shows the recovery screen.

## Editor ownership

`useTaskPlanPanelState` shall store the editor with the task ID that created it.
It shall expose the editor only when that owner ID matches the selected task ID.
This rule makes the old editor unavailable in the first render of a task
change. A delayed callback from the old child also remains unavailable.

Command-facing editor state and references shall obey the same ownership rule.
Plan-comment actions shall also reject a destroyed editor before they create or
remove an editor mark.

## Search command gate

`usePlanFindShortcut` shall treat a null editor or a destroyed editor as
unavailable. The gate shall run before these operations:

- reading the search extension state;
- adding or removing transaction listeners;
- setting or clearing the search query;
- moving to the next or previous match.

The hook shall use one local availability check for all command paths. Event
cleanup shall remain safe if TipTap destroys the editor before React runs the
cleanup function.

The task ownership rule and the destroyed-editor check are separate defenses.
The ownership rule stops cross-task reuse. The destroyed-editor check protects
same-task editor replacement and future delayed work.

## Responsive surfaces

Desktop and phone layouts use the same Plan panel and search hook. This change
does not alter composition, gestures, scrolling, safe areas, or touch targets.
No new mobile-specific browser test is required.

## Failure and compatibility

When no editor is available, search commands are temporary no-ops. React can
then mount the selected task editor without a route-level failure. Normal
search behavior resumes after the new editor reports that it is ready.

The existing route error boundary remains the final containment layer for
unrelated render errors. A dependency rollback is not part of this design.

## Verification

A focused hook test shall pass a non-null, destroyed editor whose `commands`
getter throws. The hook shall not read commands, state, or event methods from
that editor.

A panel-state test shall change the task ID before the replacement editor is
ready. It shall prove that consumers receive no editor from the outgoing task.
It shall then prove that the new editor becomes available for the selected
task.

A desktop Playwright test shall keep the Plan panel open and switch between two
sidebar tasks. It shall fail on a route error or browser page error. It shall
also confirm that plan search works in the selected task after the switch.
