---
id: "task-context-menu-drag-guard-01"
title: "Guard context-menu pointer events"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/tasks/requirements/subtask-reparenting-drag-drop.md"
---

# Task 01: Guard context-menu pointer events

## Acceptance

1. Pressing and moving inside the task row context menu (including the Color
   submenu trigger and its swatches) never starts a row drag: the menu's
   pointer-start events (`mousedown`, `pointerdown`, `touchstart`) are stopped
   at the menu content boundary before reaching the dnd-kit handle listeners.
2. Clicking a menu item still runs the item action and never activates or
   selects the row as a side effect.
3. Unit regression tests prove 1 and 2 (they must fail before the fix), the
   desktop E2E drag-from-menu regression passes, and the existing desktop and
   mobile subtask drag-and-drop E2E specs stay green.

## Verification

First-time worktree install (node_modules is absent):

```sh
cd apps && pnpm install --frozen-lockfile
```

Targeted unit test (RED then GREEN):

```sh
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-switcher-context-menu.test.tsx
```

E2E (rebuilds web + backend; run from `apps/web`):

```sh
cd apps/web && pnpm e2e:run tests/task/subtask-reparent-drag-drop.spec.ts
cd apps/web && pnpm e2e:run tests/task/mobile-subtask-reparent-drag-drop.spec.ts -- --project=mobile-chrome
```

Full gate:

```sh
make fmt && make typecheck && make test && make lint
```

## Files likely touched

- `apps/web/components/task/task-switcher-context-menu.tsx` — add
  bubble-phase `stopPropagation` guards (`onMouseDown`, `onPointerDown`,
  `onTouchStart`, `onClick`) to the `ContextMenuContent` in
  `TaskItemWithContextMenu`.
- `apps/web/components/task/task-switcher-context-menu.test.tsx` — new unit
  regression suite (wrapper spies stand in for the drag handle / row click).
- `apps/web/e2e/tests/task/subtask-reparent-drag-drop.spec.ts` — new desktop
  regression test: drag started inside the open context menu must not move
  the row.

Do **not** touch `apps/web/components/kanban-card-menu-items.tsx` or
`kanban-card-context-menu.tsx`: the kanban context menu is a fiber sibling of
the drag-listeners node and cannot leak (verified in the plan).

## Dependencies

None.

## Parallelism

Sequential; the guard and its regression tests change together.

## Inputs

- [Subtask re-parenting by drag and drop spec](../../specs/tasks/requirements/subtask-reparenting-drag-drop.md) (amended with the menu-containment scenarios)
- [Fix plan](plan.md)
- dnd-kit sensor behavior (`MouseSensor.activators` ignores only right-click;
  `PointerSensor` requires primary button) from `@dnd-kit/core` source.
- Existing precedent: `DropdownEntry` in `kanban-card-menu-items.tsx` guards
  portal menu `onClick`/`onPointerDown`; its test asserts the same containment.

## Completion record

- Unit regression suite (`task-switcher-context-menu.test.tsx`): RED confirmed
  (3/3 failed on the containment assertions before the fix) then GREEN after
  the guard; task-switcher suites 21/21.
- Desktop E2E `subtask-reparent-drag-drop.spec.ts`: 6/6 passed, including the
  new "starting a drag inside the row context menu does not drag the row".
- Mobile E2E `mobile-subtask-reparent-drag-drop.spec.ts` (mobile-chrome): 1/1
  passed — touch drag still works with the guard in place.
- `make fmt`, `make typecheck`, `make lint` passed. `make test`'s remaining
  failures are environmental and unrelated (no Docker daemon for
  `http-git-server` tests, missing `unzip` for `scripts/pr-state --job-log`,
  untagged worktree HEAD and service detection for backend launcher/updates
  tests, sandbox CPU timeouts in `git-base`/storage tests).
