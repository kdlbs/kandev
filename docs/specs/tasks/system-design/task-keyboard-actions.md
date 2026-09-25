---
status: current
system: tasks
requirements:
  - REQ-TASKS-KEYBOARD-ACTIONS-001
  - REQ-TASKS-KEYBOARD-ACTIONS-002
---

# Task keyboard actions system design

## Boundaries and mapping

The task system owns action semantics. The command registry supplies presentation
and navigation, using existing task handlers rather than new mutation transports.
No schema, public SDK, endpoint, or persistence changes are needed.

| Requirement | Design |
| --- | --- |
| REQ-TASKS-KEYBOARD-ACTIONS-001 | Scoped move submission |
| REQ-TASKS-KEYBOARD-ACTIONS-002 | Task commands and nested navigation |

## Scoped move submission

`WorkflowMoveOptionsForm` in `workflow-move-options.tsx` already owns a draft,
payload normalization, busy state, and a result contract where false preserves
input. Add a shared event predicate and submission guard near this form. Use
`formatShortcut` and existing platform helpers for the hint. The scoped key
handler consumes an accepted chord and calls the same guarded submit used by
the button. It must not be a document-level shortcut or change the base dialog's
plain-Enter contract. Use a synchronous in-flight ref as well as rendering busy
state so two events in one render interval cannot dispatch twice.

`StepMoveControls` in `workflow-stepper.tsx` and the options row in
`workflow-step-disclosure.tsx` compose `WorkflowMoveOptionsFields` directly.
Wire their local submission through the same guard and predicate inside the
existing key propagation boundaries; binding only the complete form misses
these surfaces. Keep move eligibility and current-step checks with their existing
owners. `workflow-move-proceed-button.tsx`, `WorkflowMoveDialog`, and
`MobileTaskMoveOptionsSurface` reuse the complete form.

On failure retain the draft and existing error feedback; always release the guard
in finally. On success retain current close/reset behavior. Move payloads continue
through `workflowMoveOptionsPayload`, `useTaskMoveOptions`, and existing move
hooks. Supplemental instructions never change durable workflow prompts.

## Task commands and nested navigation

Add sidebar task command composition alongside `SessionCommands` in a task-scoped host,
mounted once for the active task on desktop and mobile, including session-less
tasks. Preserve existing session/git/panel commands and keep Archive and Create subtask in `SessionCommands` with their existing IDs and confirmations. Do not register one command set per sidebar row.

Use `task-switcher-context-menu-items.tsx` as the action inventory. Extract
reusable action descriptors and eligibility only where the palette and row menu
would otherwise duplicate policy. Descriptors identify actions by stable IDs,
not translated labels, and retain callbacks or nested choices. Keep task mutations
in `useTaskActions`, `useTaskMenuActions`, priority/color/pin hooks, existing link
handlers, and existing relationship dialogs. Extract reusable domain portions
from `useSidebarActions` rather than mounting the whole sidebar in the palette.
Sidebar layout switching stays in its existing wrapper.

Adapt `useTaskPluginPrimaryMenuEntries` and `useTaskPluginLinkActions` reactively,
retaining order, icon, enabled/visibility predicates, presentation context, and
callbacks. Do not widen the plugin SDK or include card-only Edit contributions.
Reuse Archive's `TaskArchiveConfirmation` with `forceDialog`, rendered outside the palette and Delete's existing confirmed
removal path. Duplicate is represented but disabled.

The current internal `CommandItem` has no disabled flag or nested command list.
Add optional disabled and children fields to that internal shape. Extend
`CommandPanel`, its results, and shortcut handling with a local navigation stack
of parent IDs. Resolve children from the latest registered descriptors each render
so stale closures do not survive registry/task changes. Nested mode filters only
its child commands, suppressing unrelated task/file search results. Disabled
entries can be displayed but cannot become the default actionable selection or
execute from Enter, pointer, or programmatic selection. Back restores the prior
search and selection; Escape pops one level, then closes at root. Empty/removed
branches show a localized empty state or return to their valid parent.

Move to expands into current-workflow step choices from the same eligibility
projection used by `TaskMoveContextMenuItems`. Choosing a step closes the palette
and opens `TaskMoveOptionsSurface` with the chosen task/workflow/step captured and
instructions focused. The captured IDs are checked against live active context
before dispatch; a context change dismisses the surface. Change workflow opens
the task's shared workflow-change form and uses its version-checked move
contract, including destination step and task-only agent mappings, as defined
in [Change workflow](../requirements/change-workflow.md). It does not use the
immediate-move shortcut. An optional `immediateAction` on destination commands handles
Mod+Enter, closes the palette and sends normal defaults. Ignore repeated and
composing modified Enter events. Guard both action variants against stale task
context. Color and step rows use existing color classes; destination rows show a
trailing arrow and the immediate-move shortcut. Phone rows retain 44px targets;
archive uses the existing responsive confirmation dialog as the only overlay.
Color, Priority, Link, and nesting expose equivalent searchable child choices or
reuse their existing form when input is needed. No parsing of rendered menu DOM.

## Responsive composition and focus

Nearest precedents: `CommandPanelDialog`, the mobile task-switcher sheet,
`MobileTaskMoveOptionsSurface`, and `WorkflowMoveOptions`.
Use the existing keyboard command entry point on phones; existing task overflow remains the touch entry for task actions. The palette shows a fixed search
and Back header with one scrolling list; nested choices replace list content.
Choosing a move transfers from the palette into the existing move drawer, rather
than stacking active focus traps. The drawer keeps its full-height editable form,
safe-area padding, and internal scroll. Desktop uses its existing dialog shell.
Touch action rows meet 44px; desktop density remains unchanged. Restore focus on
cancellation; when task removal unmounts the opener, use existing navigation
fallback. Closing or switching active context clears action-owned state.

## Failure, security, and verification

Missing/unresolved task data fails closed; never fall back to a recently hovered
or searched task. Task commands do not grant extra capabilities. Existing backend
authorization remains authoritative and failures use current feedback. Revalidate
stale destination choices before dispatch and refresh their projection on rejection.
No new logs contain user prompt text and no new metrics are required.

Tests cover action inventory parity, active-task changes, reactive plugin removal,
no-session operation, disabled Duplicate, confirmations, nested selection, focus,
and the full keyboard move payload. Desktop/mobile E2E completes real moves and
checks draft retry plus phone geometry. See the exact work-order commands.

## Related contracts

- [Task menu grouping](task-menu-grouping.md)
- [Task action menus](task-actions-menu.md)
- [Archive confirmation](archive-confirmation.md)
- [Move overrides](../../workflow-step-move-overrides/spec.md)
- [Implementation plan](../../../plans/task-keyboard-actions/plan.md)
