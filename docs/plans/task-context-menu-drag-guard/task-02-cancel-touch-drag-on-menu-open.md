---
id: "task-context-menu-drag-guard-02"
title: "Cancel touch drag when the context menu opens"
status: done
wave: 1
depends_on: ["task-context-menu-drag-guard-01"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/subtask-reparenting-drag-drop.md"
---

# Task 02: Cancel touch drag when the context menu opens

## Acceptance

1. On touch, a long-press that opens the row context menu never results in row
   movement or reordering: the touch-drag the hold started is cancelled when
   the menu opens (no nest drop zones remain, the row is not dimmed while the
   menu is open, and dragging inside the open menu does not move the row).
2. Desktop is unaffected: right-click menus never dispatch a touchcancel that
   matters, and the existing desktop drag-from-menu and drag-to-nest E2E
   behavior is unchanged.
3. Regression coverage: a unit test asserts the menu-open touchcancel signal
   at the touchstart target; a mobile E2E proves the long-press gesture opens
   the menu without dragging the row; the existing mobile touch-drag
   re-parenting E2E stays green.

## Verification

Targeted unit test:

```sh
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-switcher-context-menu.test.tsx
```

E2E (rebuilds web + backend; run from `apps/web`):

```sh
cd apps/web && pnpm e2e:run tests/task/mobile-subtask-reparent-drag-drop.spec.ts -- --project=mobile-chrome
cd apps/web && pnpm e2e:run tests/task/subtask-reparent-drag-drop.spec.ts
```

Full gate:

```sh
make fmt && make typecheck && make test && make lint
```

## Files likely touched

- `apps/web/components/task/task-switcher-context-menu.tsx` — capture the
  touchstart target on the `ContextMenuTrigger` wrapper; on menu open dispatch
  a `touchcancel` `TouchEvent` at that element (dnd-kit `TouchSensor` cancels
  the active drag on `touchcancel`).
- `apps/web/components/task/task-switcher-context-menu.test.tsx` — touchstart
  containment assertions + menu-open `touchcancel` dispatch assertion
  (stub `TouchEvent` for happy-dom).
- `apps/web/e2e/tests/task/mobile-subtask-reparent-drag-drop.spec.ts` — new
  "long-press opens the context menu without dragging the row" test.
- `docs/specs/tasks/requirements/subtask-reparenting-drag-drop.md` — touch long-press
  cancellation contract (What bullet + scenario).

## Dependencies

Task 01 (the content-boundary guards) — this task adds the touch long-press
cancellation on top of it.

## Parallelism

Sequential; task 02 depends on task 01 and changes the same component.

## Inputs

- [Subtask re-parenting by drag and drop spec](../../specs/tasks/requirements/subtask-reparenting-drag-drop.md) (amended)
- [Fix plan](plan.md)
- dnd-kit `TouchSensor` source: activation after the 250ms delay; `touchcancel`
  listener attached to the touchstart target element while a drag is active;
  `handleCancel` → `onCancel` → `onDragCancel` (no drop).
- Adversarial review round 1 finding (major): the mobile long-press race,
  reproduced live on the dev instance (drag live at 300ms, menu at 900ms).

## Completion record

- Live dev-instance verification (CDP touch long-press, 390px viewport):
  drag live at ~300ms (row dim + nest zone), menu opens at ~900ms, drag
  cancelled at menu open (zones/dim clear while the menu stays open); moving
  the finger inside the open menu no longer moves or reorders the row.
  Initial document-level touchcancel dispatch did not cancel (the TouchSensor
  listens on the touchstart target element, not the document); corrected to
  dispatch at the captured touchstart target.
- Unit suite (`task-switcher-context-menu.test.tsx`): 10/10, including the
  touchstart containment, menu-open touchcancel (with dispatch-target
  assertions), multi-touch retention, stale-target clearing, and
  non-primary-finger touchend/touchcancel vs tracked-touch touchcancel
  identifier cases.
- Mobile E2E (`mobile-subtask-reparent-drag-drop.spec.ts`, mobile-chrome):
  2/2 — new "long-press opens the context menu without dragging the row" plus
  the existing touch-drag re-parenting test.
- Desktop E2E (`subtask-reparent-drag-drop.spec.ts`): 6/6 — desktop
  unaffected. The mobile test's initial bounding-box order assertion proved
  unreliable (duplicate hidden tree + content-visibility rows) and was
  replaced with DOM-order assertions; diagnostics confirmed the gesture
  caused no API, preference, or DOM-order change.
- `make fmt`, `make typecheck`, `make lint` passed. `make test`'s remaining
  failures are the environmental ones recorded in task 01.
