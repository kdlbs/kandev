---
spec: ../../specs/tasks/requirements/subtask-reparenting-drag-drop.md
created: 2026-08-13
status: done
---
# Fix Plan: Task drag must not start from the context menu

## Root cause

The sidebar task tree (desktop sidebar and the mobile task switcher sheet)
wires dnd-kit's drag listeners onto a handle `div` that wraps the task row.
`TaskRow` renders `TaskItemWithContextMenu`, whose Radix `ContextMenuContent`
portal is a **fiber descendant** of that handle. React synthetic events do not
follow the DOM tree for portals; they bubble through the **React fiber tree**.
So a `mousedown`, `touchstart`, or `pointerdown` on any menu item (e.g. the
`Color` submenu trigger or one of its swatches) reaches the handle's sensor
listeners:

- `MouseSensor.activators` ignores only right-click (`button === 2`), so a
  left-press on a menu item activates the sensor; after the 8px
  `DRAG_ACTIVATION_DISTANCE` the row drags (dnd-kit source:
  `MouseSensor.activators` in `@dnd-kit/core`).
- `TouchSensor` activates on `touchstart` after a 250ms hold — **before** a
  ~700ms long-press can open the context menu. A stationary long-press on the
  row therefore starts a drag that is still live when the menu opens on top of
  it; the drag only ends when the finger lifts, so continuing to drag inside
  the open menu moves/reorders the row.
- A plain `click` on a menu item additionally fiber-bubbles to the row's
  `onClick` → `onSelectTask`, activating the task as a side effect.

The kanban card context menu is **not** affected: there the
`ContextMenuContent` is a fiber *sibling* of the card shell that holds the
listeners, so no leak exists (verified structurally in
`apps/web/components/kanban-card-context-menu.tsx`). The kanban *dropdown*
menu was already fixed the same way (`DropdownEntry` guards
`onClick`/`onPointerDown`; see `kanban-card-menu-items.tsx` and its test).

## Fix

Contain pointer-start and click events at the menu boundary in
`TaskItemWithContextMenu` (`apps/web/components/task/task-switcher-context-menu.tsx`):

- **Bubble-phase containment** on the `ContextMenuContent` for `onMouseDown`,
  `onPointerDown`, `onTouchStart`, and `onClick` stops events on any menu item
  or submenu (fiber descendants) from reaching the drag handle or the row.
  Item handlers run first (they are deeper in the fiber tree), so menu actions
  still work; Radix outside-click dismissal and keyboard activation are
  unaffected.
- **Touch long-press cancellation**: the touchstart target is captured on the
  trigger wrapper (capture phase). When the menu opens, a `touchcancel`
  `TouchEvent` is dispatched at that element — dnd-kit's `TouchSensor` listens
  for `touchcancel` on the touchstart target while a drag is active, so the
  in-flight long-press drag is cancelled (`onDragCancel`, no drop) instead of
  reordering the row. Inert on desktop (no touch sensor) and for quick taps
  (sensor already detached).

## Tests

### Unit regression (`apps/web/components/task/task-switcher-context-menu.test.tsx`)

Render `TaskItemWithContextMenu` inside a wrapper `div` with spy
`onMouseDown`/`onPointerDown`/`onTouchStart`/`onClick` handlers that stand in
for the drag handle and row click. Open the real menu and assert:

1. `mousedown`/`pointerdown` on the `Color` submenu trigger do not reach the
   wrapper (fails before the fix: the spies are called).
2. `touchstart` on the `Color` submenu trigger and on a color swatch inside
   the open submenu do not reach the wrapper.
3. `click` on a menu item (e.g. Archive) still invokes its action
   (`onArchiveTask`) and does **not** reach the wrapper's `onClick`.
4. Opening the menu dispatches a `touchcancel` at the touchstart target (the
   signal that cancels an in-flight touch drag).

i18n is initialized globally (`vitest.setup.ts`), so the real English labels
render; wrap in `StateProvider`/`ToastProvider` per `task-switcher.test.tsx`.
happy-dom does not construct `TouchEvent`, so tests 2/4 stub it.

### E2E (desktop, extend `apps/web/e2e/tests/task/subtask-reparent-drag-drop.spec.ts`)

Seed a parent with a subtask, right-click the subtask row, assert the context
menu opens (Color item visible), then press the mouse down on the Color item,
move ≥8px, assert **no** nest drop zone appears and the row order is unchanged,
then release.

### E2E (mobile, extend `apps/web/e2e/tests/task/mobile-subtask-reparent-drag-drop.spec.ts`)

Seed two root tasks, long-press a row via CDP touch (hold 1200ms), assert the
context menu opens while **no** nest drop zone renders and the row is not
dimmed, then drag inside the open menu and assert the row order and parent
relationship are unchanged (native list scroll may shift both rows together).

## Implementation Wave

1. [Guard context-menu pointer events](task-01-guard-context-menu-pointer-events.md) — done, sequential.
2. [Cancel touch drag when the context menu opens](task-02-cancel-touch-drag-on-menu-open.md) — done, sequential.

## Risks

- A capture-phase guard would break item selection (it would stop the event
  before it reaches the items); the guard must be bubble-phase on the content.
- Guarding only the top-level items would miss submenu content (Color); the
  content-level guard covers all fiber descendants.
- The kanban card context menu must stay untouched (no leak there); changing
  it would be speculative.
- The touch long-press cancel dispatches a synthetic `touchcancel` at the
  touchstart target. It is inert when no touch sensor is armed (desktop
  right-click, quick taps) but any other listener of `touchcancel` on that
  element would also see it; dnd-kit's sensor is the only known one. The
  250–700ms window where the drag is live before the menu opens still shows a
  brief row dim/nest-zone flash on touch long-press; the drag is cancelled the
  moment the menu opens.
