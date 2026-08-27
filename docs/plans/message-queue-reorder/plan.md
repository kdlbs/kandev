---
spec: docs/specs/ui/requirements/message-queue-reorder.md
created: 2026-08-07
status: draft
---

# Implementation Plan: Reorder Queued Messages

## Overview

Add a durable, server-persisted reorder operation for the per-session message
queue. The backend gains `message.queue.reorder` (full-order submission,
atomically validated against the visible pending set, positions rewritten in
one transaction). The frontend wires a new `reorderEntries` hook action
(optimistic reorder + reconcile) and converts the queue panel rows to
`@dnd-kit/sortable` with a dotted grab handle that floats over the row's left
edge on hover (always visible on touch). E2E covers desktop pointer and
mobile touch drags; two shipped specs' out-of-scope lines are updated.

Order: repository → service/handler → frontend API/hook → frontend UI →
E2E → docs.

---

## Backend

### Repository contract — `apps/backend/internal/orchestrator/messagequeue/repository.go`

Add to the `Repository` interface:

```go
// ReorderEntries atomically rewrites the FIFO positions of the session's
// pending entries to match orderedIDs. The submitted list must be an exact
// permutation of the currently visible pending (not reserved-in-flight)
// entry ids ordered as the caller wants them; any drift — missing, extra,
// or duplicate ids, or an id belonging to a reserved in-flight row —
// returns ErrQueueChanged and leaves the queue untouched. Reserved
// in-flight rows keep their place in the sequence; visible rows are
// interleaved in the submitted order and all positions are compacted to
// 1..N in one transaction.
ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error
```

Add `ErrQueueChanged = errors.New("queue changed during reorder")` beside the
other package errors in `types.go`.

### SQLite — `apps/backend/internal/orchestrator/messagequeue/repository_sqlite.go`

`ReorderEntries`: take `r.withSessionLock(sessionID)`, begin a tx (pattern:
`MergeIntoAbove`, `applyMergeWrites`). Inside the tx:

1. Read all rows ordered by position (`SELECT id, ... ORDER BY position ASC`).
2. Split into `visible` (not `IsReservedInFlight()`) and `reserved`.
3. Validate `orderedIDs` is a permutation of the visible ids (same set, no
   duplicates) → else `ErrQueueChanged`.
4. Build the merged sequence: walk the original order; reserved rows emit as
   themselves, visible rows emit from `orderedIDs` in order.
5. Rewrite every row's position to `1..N` (`UPDATE queued_messages SET
   position = ? WHERE id = ? AND session_id = ?`; check `RowsAffected` per
   row — a vanished row during the tx means a drain raced the lock and
   `ErrQueueChanged` rolls back).
6. Commit. Defer `tx.Rollback()` as in the merge path.

Do not reuse `ListBySession` inside the tx (it reads from the read-only
handle); select within the tx so validation and writes see one snapshot under
the session lock.

### Memory — `apps/backend/internal/orchestrator/messagequeue/repository_memory.go`

Mirror under `r.mu.Lock()`: same visible/reserved split and permutation
validation against the stored slice (slice is not guaranteed position-sorted
after `ReplaceSession` — sort a copy by position first), then rewrite each
entry's `Position` to `1..N` in the merged sequence and bump
`r.nextPosition[sessionID]` to at least `N`.

### Tests — `apps/backend/internal/orchestrator/messagequeue/repository_reorder_test.go`

Table-driven over both repos (pattern: `repository_send_now_test.go` builds a
table of `new func(*testing.T) Repository`). Cases:

- reorder reverses a 3-entry queue; positions become 1..3, content/metadata/
  queued_by untouched (compare full entries).
- no-op order (same as current) succeeds and leaves positions unchanged.
- drift: missing id, extra id, duplicate id, empty list → `ErrQueueChanged`,
  queue unchanged (assert full snapshot identical).
- a reserved in-flight lifecycle row (insert via metadata
  `lifecycle_reserved_in_flight: true`) is not in the visible set: submitting
  only visible ids succeeds, the reserved row keeps its position, visible
  rows interleave around it; submitting the reserved id → `ErrQueueChanged`.
- concurrent drain during reorder: take head after building the submitted
  set, then reorder → `ErrQueueChanged` (validates the atomic read).

---

### Service — `apps/backend/internal/orchestrator/messagequeue/service.go`

```go
// ReorderEntries rewrites the pending FIFO order of a session's queue to
// match orderedIDs. Returns ErrQueueChanged when the submitted order does
// not match the current visible pending set (a drain/remove/merge raced
// the request); callers refetch the authoritative queue.
func (s *Service) ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error
```

Validation before the repo call: non-empty, no duplicate ids → `ErrQueueChanged`
or a dedicated validation error (handler maps both to `queue_changed`).
Delegate to `s.repo.ReorderEntries`; wrap only unexpected errors.

### WebSocket action — `apps/backend/pkg/websocket/actions.go`

Add next to the queue actions:

```go
ActionMessageQueueReorder = "message.queue.reorder" // Rewrite the visible pending order for a session
```

### Handler — `apps/backend/internal/orchestrator/handlers/queue_handlers.go`

- Add `ReorderEntries(ctx context.Context, sessionID string, orderedIDs []string) error`
  to the `QueueService` interface (real impl is `messagequeue.Service` via
  `orchestratorSvc.GetMessageQueue()` — `apps/backend/internal/backendapp/gateway.go`).
- Register `d.RegisterFunc(ws.ActionMessageQueueReorder, h.wsReorder)`.
- `wsReorder`: parse `{session_id, ordered_ids}`; validation errors for
  missing session, empty/duplicate `ordered_ids`; `authorizeSession`; call
  `ReorderEntries`; map `ErrQueueChanged` (and the service-level validation
  error) to error code `queue_changed`; other errors → `500`-style WS error.
  On success respond `{session_id, reordered: N}` and call `publishStatus`.
- Add `queueErrorCodeQueueChanged = "queue_changed"` constant (leave the
  existing send-now constant untouched; wire value is identical).

### Tests — handler + service

- `apps/backend/internal/orchestrator/handlers/queue_handlers_reorder_test.go`:
  happy path (mock `QueueService` + `mockEventBus` asserts `publishStatus`
  ran), validation errors (missing session, empty ids, duplicate ids),
  access denied, `ErrQueueChanged` → `queue_changed` error code, and a real
  `messagequeue.NewServiceMemory` round-trip (queue 2 entries via
  `QueueMessageWithMetadata`, reorder reversed, assert response + status).
  Add the action to the table in `queue_handlers_test.go` (mirror the
  `merge` row).
- Service-level validation covered in the repository tests plus a small
  `service_test.go` case for duplicate/empty input.

---

## Frontend

### API client — `apps/web/lib/api/domains/queue-api.ts`

```ts
export class QueueReorderError extends Error {
  readonly code = "queue_changed" as const;
  constructor(message?: string) { ... this.name = "QueueReorderError"; }
}

export async function reorderQueuedEntries(params: {
  session_id: string;
  ordered_ids: string[];
}): Promise<{ session_id: string; reordered: number }>
```

`reorderQueuedEntries` sends `{ action: "message.queue.reorder", payload }`
via `getWebSocketClient()`. Because `rethrowQueueError` currently maps
`queue_changed` to `QueueSendNowError`, check `asWSError(err)?.code ===
"queue_changed"` in `reorderQueuedEntries` and throw `QueueReorderError`
before falling through to `rethrowQueueError`.

### Hook — `apps/web/hooks/domains/session/use-queue.ts`

Add `reorderEntries(orderedIds: string[])`:

1. Optimistic: reorder the current `entries` by id using `setQueueEntries`
   (keep the same `meta`).
2. `await reorderQueuedEntries({ session_id, ordered_ids })`; on
   `QueueReorderError` refetch and swallow (self-recovering race, mirrors
   `mergeEntry`'s `QueueEntryNotFoundError` handling); on any other error
   refetch and rethrow.
3. On success refetch to reconcile (authoritative order + positions).

Expose `reorderEntries` from `useQueue` and from
`useQueuePanelHandlers`/`QueueAffordance` as `handleReorder(orderedIds)` that
toasts `chat:failedToReorderQueuedMessages` for non-race errors.

### State slice

No changes: `setQueueEntries` and `metaBySessionId` already exist in
`apps/web/lib/state/slices/session/types.ts` / the session store.

### i18n — `apps/web/src/locales/en/chat.json` (+ regenerate pseudo)

- `"reorderQueuedMessage": "Reorder queued message"` — handle aria-label
- `"sortable": "sortable"` — handle `aria-roledescription`
- `"failedToReorderQueuedMessages": "Failed to reorder queued messages."` —
  error toast

Run `pnpm i18n:pseudo` to regenerate `src/locales/pseudo/chat.json`, then
`pnpm i18n:check` and `pnpm i18n:ratchet`. No new copy may be a hardcoded
literal in JSX.

---

### Queue panel — `apps/web/components/task/chat/queued-ghost-list.tsx`

In `QueuePanel`, wrap the `queue-scroll-region` list with dnd-kit (pattern:
`apps/web/components/kanban/swimlane-container.tsx`):

```tsx
<DndContext
  sensors={sensors}
  collisionDetection={closestCenter}
  onDragStart={({ active }) => setActiveId(String(active.id))}
  onDragEnd={handleDragEnd}
  onDragCancel={() => setActiveId(null)}
>
  <SortableContext items={ids} strategy={verticalListSortingStrategy}>
    {entries.map((entry, index) => (
      <QueuedGhostMessage key={entry.id} ... canDrag={!isLoading && !cancellationPending} isDragging={activeId === entry.id} />
    ))}
  </SortableContext>
</DndContext>
```

- `sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }))`.
- `handleDragEnd`: no-op when `over` is null or ids match; otherwise
  `arrayMove(ids, oldIndex, newIndex)` → `onReorder(newIds)`.
- `onReorder` = `handleReorder` from `QueueAffordance` (threaded through
  `QueuePanelDisclosure` + `QueuePanel` props, like `onMerge`).

### Row — `apps/web/components/task/chat/queued-ghost-message.tsx`

- `useSortable({ id: entry.id, disabled: editing || !canDrag })` on the row
  wrapper; apply `setNodeRef`, `style={{ transform: CSS.Transform.toString(transform), transition }}`,
  add `group relative` + `z-10 opacity-40` while `isDragging`.
- Add `canDrag` + `isDragging` props; `canDrag` defaults `true` for
  standalone renders/tests.
- New `QueueGrabHandle` component rendered when `!editing`: an absolutely
  positioned `<button type="button">` at `left-0`/`-left-1`, `top-1/2
  -translate-y-1/2`, `z-10`, `cursor-grab active:cursor-grabbing touch-none`,
  containing a 2×3 dot grid (`grid grid-cols-2`, 3px dots) plus
  `{...attributes} {...listeners}` from `useSortable`.
  - Visibility: `opacity-0 group-hover:opacity-100 group-focus-within:opacity-100
    transition-opacity` on fine pointers;
    `[@media(pointer:coarse)]:opacity-100` always visible with
    `[@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11` hit area
    and a translucent surface (`bg-background/80`) so the `#N` label stays
    faintly legible.
  - `aria-label={t("chat:reorderQueuedMessage")}`,
    `aria-roledescription={t("chat:sortable")}`.
  - Disabled state (`!canDrag`): no listeners, `cursor-default opacity-30`.
- The row body keeps its existing handlers; the handle is the only draggable
  surface (`listeners` only on the handle).

### Tests — `apps/web/components/task/chat/queued-ghost-list.test.tsx` (+ new)

- Handle visibility: hidden without hover on a fine pointer (opacity class),
  revealed by `mouseenter` on the row, hidden while the row is editing.
- Drag end wiring: simulate `DndContext` drag via pointer events on the
  handle (or call the panel's `handleDragEnd` with crafted `DragEndEvent`
  objects) and assert `reorderEntries` receives the `arrayMove`d id list.
- Disabled while `isLoading`/`cancellationPending`; handle absent/disabled
  during edit.
- `use-queue.test.ts`: `reorderEntries` optimistically updates the store
  order, refetches on success, refetches + rethrows on failure, swallows
  `QueueReorderError` after refetch.
- `queue-api.test.ts`: `reorderQueuedEntries` sends the right action/payload,
  maps `queue_changed` → `QueueReorderError`.

---

## Tests

- **Backend repo (unit, both impls):** `repository_reorder_test.go` —
  reversal, no-op, every drift case, reserved-row interleaving, atomic
  rollback on drift, concurrent-drain race.
- **Backend handler/service:** `queue_handlers_reorder_test.go` — validation,
  authorization, `queue_changed` mapping, publish-on-success, real
  in-memory round-trip.
- **Frontend unit:** `queue-api.test.ts`, `use-queue.test.ts`,
  `queued-ghost-list.test.tsx` — action payload, error mapping, optimistic
  order + reconcile, handle visibility/disabled states, drag-end wiring.
- Commands (run from `apps/backend` / `apps/web` as appropriate):
  - `cd apps/backend && go test ./internal/orchestrator/messagequeue/...`
  - `cd apps/backend && go test ./internal/orchestrator/handlers/...`
  - `cd apps && pnpm --filter @kandev/web test -- lib/api/domains/queue-api.test.ts hooks/domains/session/use-queue.test.ts components/task/chat/queued-ghost-list.test.tsx`
  - `cd apps/web && pnpm run typecheck`
  - `cd apps/web && pnpm run lint -- components/task/chat` (plus `i18n:ratchet`/`i18n:check`)

## E2E Tests

- **Scenario:** drag handle of `#3` above `#1` → order persists across
  reload; handle hidden until hover on desktop; touch drag on mobile.
- **File:** `apps/web/e2e/tests/chat/message-queue-reorder.spec.ts` and
  `apps/web/e2e/tests/chat/mobile-message-queue-reorder.spec.ts`
  (mobile file name is required for the `mobile-chrome` project).
- **What to verify:** queue 3 messages while the agent is busy (reuse the
  quick-chat + `typeWhileBusy` pattern from `message-queue.spec.ts`), open
  the panel, assert `#1/#2/#3` labels, `dragTo` the handle of the last row
  onto the first, assert the labels swapped and the order survives a reload
  (server-persisted). Desktop also asserts the handle is not visible before
  hover. Mobile asserts the handle is visible without hover and touch drag
  reorders. Keyboard: focus handle, Space, Arrow Up, Space, assert move.
  If `dragTo` proves flaky against dnd-kit pointer sensors, dispatch
  pointerdown/move/up manually (pattern: `file-tree-drag-drop.spec.ts`).
- Build note (from AGENTS.md): E2E runs against the production build — run
  `make build-web` (or `make test-e2e`) after frontend changes.

## Verification Results

Pending. On completion, synchronize with each task's `## Results`.

## Implementation Waves And Parallel Candidates

```
Wave 1 (sequential):
- [x] [task-01-backend-repository-reorder](task-01-backend-repository-reorder.md)

Wave 2 (sequential):
- [x] [task-02-backend-service-handler-reorder](task-02-backend-service-handler-reorder.md)

Wave 3 (sequential):
- [x] [task-03-frontend-api-hook-reorder](task-03-frontend-api-hook-reorder.md)

Wave 4 (sequential):
- [x] [task-04-frontend-dnd-ui](task-04-frontend-dnd-ui.md)

Wave 5 (sequential):
- [x] [task-05-e2e-reorder](task-05-e2e-reorder.md)

Wave 6 (sequential):
- [x] [task-06-docs-out-of-scope-updates](task-06-docs-out-of-scope-updates.md)
```

All tasks are `sequential` by default. `task-06` touches only docs and is
independent; it is listed last so the two shipped specs are updated after the
feature is proven, but it may be executed any time after the spec lands.

## Open Questions

(Delete when empty.)
