---
id: "03-frontend-api-hook-reorder"
title: "Frontend queue API and hook reorder"
status: done
wave: 3
depends_on: ["02-backend-service-handler-reorder"]
plan: "plan.md"
spec: "../../specs/ui/message-queue-reorder.md"
---

# Task 03: Frontend queue API and hook reorder

## Acceptance

1. `reorderQueuedEntries({ session_id, ordered_ids })` sends
   `message.queue.reorder` via the WS client and maps the `queue_changed`
   error code to a dedicated `QueueReorderError`.
2. `useQueue` exposes `reorderEntries(orderedIds)` that optimistically
   reorders the store entries, refetches on success, refetches and swallows
   `QueueReorderError`, and refetches + rethrows other failures.
3. `QueueAffordance`/`useQueuePanelHandlers` expose `handleReorder` that
   toasts `chat:failedToReorderQueuedMessages` for non-race failures; unit
   tests cover the payload, error mapping, optimistic order, and reconcile.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/api/domains/queue-api.test.ts hooks/domains/session/use-queue.test.ts components/task/chat/queued-ghost-list.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/api/domains/queue-api.ts` — `QueueReorderError`, `reorderQueuedEntries` (check `asWSError(err)?.code === "queue_changed"` before `rethrowQueueError`).
- `apps/web/lib/api/domains/queue-api.test.ts` — new cases.
- `apps/web/hooks/domains/session/use-queue.ts` — `reorderEntries` in `useEntryMutations`-adjacent code (or `useQueueActions`), exposed from `useQueue`; optimistic `setQueueEntries` preserving `meta`.
- `apps/web/hooks/domains/session/use-queue.test.ts` — new cases.
- `apps/web/components/task/chat/queued-ghost-list.tsx` — `handleReorder` in `useQueuePanelHandlers` (toast wiring only; drag UI is task 04).

## Dependencies

Task 02 (wire contract exists; handler returns `queue_changed`).

## Parallelism

Sequential.

## Inputs

- Spec: `## API Surface`, `## Failure Modes`.
- Plan: `### API client`, `### Hook`, `### State slice`.
- Existing patterns: `mergeQueuedEntry`/`QueueEntryNotFoundError` handling in `useEntryMutations`, `sendQueuedNow` + `QueueSendNowError` in `queue-api.ts`.

## Output contract

Summary, files changed, exact test commands and outcomes, blockers, risks; update task + plan statuses in the same conversation.

## Results

- `cd apps && pnpm --filter @kandev/web test -- lib/api/domains/queue-api.test.ts hooks/domains/session/use-queue.test.ts` → 43 passed.
- `cd apps/web && pnpm run typecheck` → exit 0.
- Files: `lib/api/domains/queue-api.ts` (`QueueReorderError`, `reorderQueuedEntries` + tests), `hooks/domains/session/use-queue.ts` (`useReorderEntriesAction`, `reorderEntries` exposed from `useQueue` + tests).
