---
id: message-queue-merge-03
title: Frontend merge control
status: completed
wave: 3
depends_on: ["message-queue-merge-02"]
plan: plan.md
spec: ../../specs/ui/requirements/message-queue-merge.md
---

# Task 03: Frontend merge control

## Acceptance

1. `mergeQueuedEntry` in `lib/api/domains/queue-api.ts` sends
   `message.queue.merge` and rethrows queue errors; `useQueue` exposes a
   `mergeEntry(entryId)` action that refetches after success and refetches on
   `QueueEntryNotFoundError`.
2. The queue panel renders a **Merge with above** ghost button
   (`data-testid="queue-entry-merge"`, title `Merge with above`,
   `IconArrowMerge`) on an entry only when the entry and the one above it are
   both mergeable and share the same sender kind (`user`↔`user`, `agent`↔
   `agent`); agent rows additionally require an identical non-empty
   `sender_task_id` in metadata (mirroring the backend's `mergeAllowed`), and
   the control is hidden for the first entry, mismatched kinds, and
   workflow/system entries.
3. Unit tests cover button gating and the click → `mergeEntry` wiring.

## Verification

- `cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/queued-ghost-message.test.tsx components/task/chat/queued-ghost-list.test.tsx`

## Files

- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/components/task/chat/queued-ghost-message.test.tsx`
- `apps/web/components/task/chat/queued-ghost-list.test.tsx`

## Inputs

- Spec sections: `What`, `Permissions`, `Scenarios`.
- Plan sections: `Frontend`.
- Patterns to mirror: `removeEntry` in `use-queue.ts`, the Edit/Remove ghost
  buttons in `queued-ghost-message.tsx`, `handleRemove`/`handleSave` in
  `queued-ghost-list.tsx`, and the existing `queued-ghost-list.test.tsx` /
  `queued-ghost-message.test.tsx` harnesses.
- Verify `IconArrowMerge` exists in the installed `@tabler/icons-react`
  (^3.36.1) before importing; otherwise use `IconArrowsJoin` or the closest
  merge glyph.

## Risks

- Frontend code-quality limits (≤100-line functions, complexity) — keep the
  mergeability helpers tiny and pure.
- `senderKindOf` is currently private in `queued-ghost-message.tsx`; export the
  merge helpers from the same module.
