---
id: "03-bound-frontend-stream-work"
title: "Bound frontend stream work"
status: done
wave: 3
depends_on: ["02-isolate-replaceable-session-delivery"]
plan: "plan.md"
spec: "../../specs/platform/requirements/bounded-task-status-delivery.md"
---

# Task 03: Bound Frontend Stream Work

## Acceptance

- During one animation frame, hundreds of updates for one message cause one
  Zustand update with the newest complete payload.
- Different message keys each make progress once per frame; add/delete and
  turn-settle barriers cannot resurrect or leave stale message state.
- Pending frame work is cleared when the router/client is disposed or replaced.
- Office task detail rerenders with equivalent task/session objects send no
  subscription changes or snapshot replays; real membership changes apply only
  the session-ID delta.
- Desktop and mobile share the same store behavior and no new copy/layout is
  introduced.

## TDD sequence

1. RED: add message-handler tests with a fake animation-frame scheduler for
   same-key replacement, multi-key fairness, barriers, and teardown.
2. RED: add an Office live-sync rerender test proving equivalent IDs do not
   unsubscribe/resubscribe and changed membership applies only a delta.
3. GREEN: extract the frame-batched replacement helper and route
   `session.message.updated` through it.
4. GREEN: extract/stabilize Office live sync around a sorted session-ID key and
   incremental membership reconciliation.
5. REFACTOR: keep transport helpers independent of React components and verify
   shared mobile/desktop state selectors do not need changes.

## Files likely touched

- `apps/web/lib/ws/handlers/messages.ts`
- new `apps/web/lib/ws/handlers/messages.test.ts`
- `apps/web/lib/ws/router.ts` or its disposal owner
- `apps/web/app/office/tasks/[id]/page.tsx`
- new extracted Office live-sync hook/helper and test
- focused session-slice tests if message merge semantics change

## Verification

```bash
cd apps/web && pnpm exec vitest run \
  lib/ws/handlers/messages.test.ts \
  'app/office/tasks/[id]/use-session-live-sync.test.tsx'
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run lint
```

## Dependencies

Task 02 defines replacement and semantic-barrier ordering at the wire boundary.

## Parallelism

Sequential. Task 04 needs the complete backend/frontend containment stack.

## Inputs

- Existing `registerMessagesHandlers`, session slice `updateMessage`, and
  Office `useSessionLiveSync` behavior.
- Mobile parity contract in the parent plan.

## Risks

- `requestAnimationFrame` is throttled in hidden tabs; retain only latest full
  state and reconcile on resume.
- A queued update after delete can apply stale state unless deletion cancels
  the key before store mutation.
- Sorting/memoizing IDs must not hide real membership changes or leak refcounts.

## Output contract

Report input versus store-update counts, barrier behavior, subscription call
counts across rerenders/membership changes, desktop/mobile shared-state proof,
test results, and files changed.

## Verification results

- `pnpm exec vitest run lib/ws/handlers/messages.test.ts 'app/office/tasks/[id]/use-session-live-sync.test.tsx'` — passed (7 tests).
- `pnpm run typecheck` — passed.
- `pnpm run lint` — passed for the web package.
- `session.message.updated` batching keeps one latest payload per key per
  animation frame; add/delete and turn-complete events flush as barriers.
- Office live sync sorts and deduplicates IDs, reconciles only membership
  deltas, and clears subscriptions on disconnect/unmount. Desktop and mobile
  continue to consume the same WebSocket/store path; no layout or copy changed.

## Files changed

- `apps/web/lib/ws/handlers/messages.ts`
- `apps/web/lib/ws/handlers/messages.test.ts`
- `apps/web/lib/ws/handlers/turns.ts`
- `apps/web/lib/ws/router.ts`
- `apps/web/lib/ws/use-websocket.tsx`
- `apps/web/app/office/tasks/[id]/page.tsx`
- `apps/web/app/office/tasks/[id]/use-session-live-sync.ts`
- `apps/web/app/office/tasks/[id]/use-session-live-sync.test.tsx`
