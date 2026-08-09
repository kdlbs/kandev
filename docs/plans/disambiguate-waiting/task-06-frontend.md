---
task: 06
title: "Frontend — icon promotion, resolvers, six producers, discard rule"
wave: 1
status: done
depends_on: [03]
spec_acs: AC-23, AC-34, AC-52, AC-58, AC-59, AC-73a, AC-83, AC-84
---

# Task 06 — Frontend

Complete the full frontend data chain for `parkedOnBackgroundWork`. Two separate
concerns that must both land: (A) wiring the resolvers so the icon renders given the
prop, and (B) wiring the six producers so the prop reaches the resolvers from live data.
An implementation that does only (A) passes every icon criterion with all surfaces dark.
AC-83 and AC-84 exist precisely to catch that build.

## 1. Icon promotion (prerequisite for everything else)

**Move `BackgroundWorkTaskIcon`** from `apps/web/components/task/task-item.tsx:165-185`
into `apps/web/lib/ui/state-icons.tsx`. Add a `className?: string` prop, following the
`InterruptedTaskIcon` precedent. Export it.

The sidebar passes `className="h-3.5 w-3.5 mt-[1px]"` (matching the existing sidebar
icon size); other call sites use no className override.

`state-icons.tsx` already has `getTaskStateIconConfig` and `getSessionStateIconConfig`.
Both resolvers are updated in step 2.

## 2. Resolver A — `task-item.tsx` private ladder

**`apps/web/components/task/task-item.tsx`** (lines ~187–292):

Insert a new rung near the top of the private classification ladder, before the
`classifyTask` call:

```tsx
if (task.parkedOnBackgroundWork) {
  return <BackgroundWorkTaskIcon className="h-3.5 w-3.5 mt-[1px]" />;
}
```

The spec maps resolver A to: sidebar task list and the desktop sidebar (via
`buildSidebarItem`), and the mobile task switcher sheet (via `toSheetItem`).

Tests in `task-item.test.tsx` (which already defines `BACKGROUND_ICON_TEST_ID` at `:11`):
- **AC-23**: task with `parkedOnBackgroundWork=true` and state=`WAITING_FOR_INPUT` renders
  the background icon.
- **AC-34**: parked overrides the turn-finished icon for `state=REVIEW`.
- **AC-59 second half**: parked=false leaves existing matrix unchanged (no-change assertion
  over the private ladder).

## 3. Resolver B — `state-icons.tsx` shared resolver

**`apps/web/lib/ui/state-icons.tsx`** — `getTaskStateIconConfig`:

Add a parked branch that returns the `BackgroundWorkTaskIcon` config when
`task.parkedOnBackgroundWork` is true, before the existing state-based branches.

The board card (`kanban-card-content.tsx`) and the `/tasks` list row
(`rich-task-list-row.tsx`) go through this resolver.

Also update `getSessionStateIconConfig`: when `session.parkedOnBackgroundWork` is true,
return the background icon config. The session icon appears at:
- `sessions-dropdown.tsx:475`
- `session-reopen-menu.tsx:204`
- `mobile-sessions-section.tsx:132`

Tests in `state-icons.test.tsx`:
- **AC-52**: `getTaskStateIconConfig` with parked=true returns the background icon.
- **AC-59 first half**: existing state matrix unchanged for parked=false.
- **AC-73a**: `getSessionStateIconConfig` with parked=true returns the background icon.

## 4. Board early returns

**`apps/web/components/kanban-card-content.tsx`** — BOTH early returns must change:

- Line `:275` — the path a parked task hits (returns null today). Add the parked check
  before this return so parked tasks render the icon instead of null.
- Line `:282` — the spinner return. Also add the parked check here so a parked session
  during a generating state renders the parked icon rather than the spinner.

Do not change only `:275` — the spec explicitly calls out that both must change.

## 5. Six producers — wire parked fields from task record

All six producers read from the TASK record (snake_case wire fields), not from
`TaskStatusSummary` (which gains no parked field).

### 5a. `apps/web/lib/kanban/map-task.ts` — `toKanbanTask` (~line 152)

```ts
parkedOnBackgroundWork: task.parked_on_background_work ?? false,
parkedRevision: task.parked_revision ?? 0,
```

### 5b. `apps/web/lib/ws/handlers/kanban.ts` — both projections (~lines 83 and 121–124)

Two separate places update the board store from WS task-update frames. Both must propagate
the parked fields:

```ts
// Projection 1 (~line 83):
parkedOnBackgroundWork: update.parked_on_background_work ?? existing.parkedOnBackgroundWork,
parkedRevision: update.parked_revision ?? existing.parkedRevision,

// Projection 2 (~line 121-124):
parkedOnBackgroundWork: update.parked_on_background_work ?? false,
parkedRevision: update.parked_revision ?? 0,
```

Apply the epoch-based discard rule here (see §6 below).

### 5c. `apps/web/lib/ssr/mapper.ts` — `snapshotToState` (~line 51)

This is the Go boot-snapshot path. It does **not** route through `toKanbanTask`. Must
propagate parked fields independently:

```ts
parkedOnBackgroundWork: task.parked_on_background_work ?? false,
parkedRevision: task.parked_revision ?? 0,
```

### 5d. `apps/web/components/task/task-session-sidebar-item.ts` — `buildSidebarItem`

Read from the **task record**, not from `TaskStatusSummary`. The adjacent
`foregroundActivity` line uses the `hasSummary ? summary?.foreground_activity :
task.foregroundActivity` ternary — do NOT copy this pattern for parked. `TaskStatusSummary`
carries no parked field and gains none, so the mirrored line returns `undefined` for every
open task. Read directly:

```ts
parkedOnBackgroundWork: task.parkedOnBackgroundWork ?? false,
```

### 5e. `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts` — `toSheetItem`

Same as `buildSidebarItem` — read from the task record only:

```ts
parkedOnBackgroundWork: task.parkedOnBackgroundWork ?? false,
```

## 6. Merge-helper epoch-based discard rule

In `apps/web/lib/ws/handlers/tasks.ts` — the `mergeTaskUpdate` function (precedent:
`mergeCancellationProjection`):

When a WS task-update frame arrives, apply field-scoped discard:

```ts
function applyParkedFields(existing: Task, update: Partial<Task>): Partial<Task> {
  // If the incoming revision is older than what we already applied, keep the
  // existing parked fields (but apply all other fields normally).
  if (
    update.parked_revision !== undefined &&
    existing.parked_revision !== undefined &&
    update.parked_revision < existing.parked_revision
  ) {
    const { parked_on_background_work, parked_revision, ...rest } = update;
    return rest;
  }
  return update;
}
```

The epoch comparison is: a frame's parked fields are stale if they come from a prior
backend process (different `parked_epoch`). An epoch mismatch means a restart happened —
reset the parked state to the incoming frame's value regardless of revision. A same-epoch,
lower-revision frame discards only the three parked fields; all other fields apply normally.

Boot-snapshot reset: when the boot payload's `parked_epoch` differs from the WS
notification epoch, the boot state is stale — accept the WS notification's parked fields
unconditionally.

## Tests to write

- `state-icons.test.tsx` — AC-52, AC-59 first half, AC-73a
- `task-item.test.tsx` — AC-23, AC-34, AC-59 second half (uses `BACKGROUND_ICON_TEST_ID`)
- Producer unit tests for `toKanbanTask`, `snapshotToState` (verify parked field presence)
- Kanban handler tests for both WS projections (discard rule: lower revision discarded,
  higher revision applied)
- `buildSidebarItem` / `toSheetItem` tests: verify reading from task not summary (AC-83, AC-84)

## Acceptance criteria closed

- **AC-23**: sidebar renders background icon when parked.
- **AC-34**: parked overrides turn-finished icon.
- **AC-52**: shared task resolver returns background icon when parked.
- **AC-58**: board card renders background icon when parked.
- **AC-59**: no-change assertions over the existing state matrices.
- **AC-73a**: session resolver returns background icon when parked.
- **AC-83**: `buildSidebarItem` propagates parked from task record.
- **AC-84**: `toSheetItem` propagates parked from task record.
