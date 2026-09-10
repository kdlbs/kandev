---
id: "04-swipe-position-feedback"
title: "Synchronize swipe-position feedback"
status: done
wave: 4
depends_on:
  - 03-inline-pagination
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-003
acceptance_criteria:
  - AC-UI-THREADS-DECK-003.4
  - AC-UI-THREADS-DECK-003.9
  - AC-UI-THREADS-DECK-003.12
  - AC-UI-THREADS-DECK-003.13
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 04: Synchronize swipe-position feedback

## Summary

Make the inline pagination and task picker's current row follow phone scroll
position during a swipe, independently of detail hydration. Keep existing
session activation limits, stable order, and desktop behavior intact.

## In scope

- Expose `mobileTaskId` from the board's existing viewport owner using passive
  scroll, frame-coalesced shell geometry, and resize/order reconciliation.
- Consume that ID directly in the board's header slot and picker instead of
  deriving their current context from `detailTaskIds`.
- Add permanent targeted unit and mobile touch regressions after proving RED.
- Keep Threads guidance and public pagination description accurate after the
  behavior changes. Refresh the existing demo without reseeding.

## Out of scope

- Transcript prefetch policy, expanded detail budgets, session selection,
  global state, saved preferences, custom touch handling, continuous fractional
  dot animation, and other listing headers.

## Acceptance

1. The indicator changes as the nearest shell changes while two shells remain
   intersecting, including reversal, with no dependency on loading completion.
2. Deep links, picker selection, removal, resize, empty/single-thread states,
   and phone/tablet transitions leave a valid position and clean up scheduled
   work. Only one phone transcript is detail-active.
3. Existing 56-pixel topbar, bounded dots, no instruction text, full-width
   columns, local content overflow, and focus-return behavior remain intact.

## Verification

First add a regression named `updates mobile position across the midpoint
without changing intersecting membership` in
`thread-column-activation.test.tsx`. The temporary diagnostic found A still
selected with B covering 60% of a 300-pixel board. Assert the new presentation
identity rather than requiring a change to detail hydration policy. Add
reversal, pending-frame cleanup, resize, and admitted-order fallback cases.

Add `updates inline pagination during a held swipe before destination detail
loads` to `mobile-threads-swipe.spec.ts`. Share the existing CDP touch pattern,
assert actual intermediate geometry before `touchEnd`, and delay destination
detail delivery through existing test fixtures. Do not weaken this to a
settled-scroll or chat-mounted assertion. Then release and assert snap and
the one-conversation budget. Avoid arbitrary sleeps.

From the repository root (dependencies are already installed in this worktree):

```bash
cd apps/web
pnpm test components/threads/ lib/threads/ app/threads/threads-page-client.test.tsx
pnpm run typecheck
pnpm exec eslint components/threads/ app/threads/threads-page-client.tsx --max-warnings=0
pnpm run i18n:check
cd ../..
make build-web
cd apps/web
pnpm e2e:raw tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts --project=mobile-chrome --retries=0
pnpm e2e:raw tests/task/threads-view.spec.ts --project=chromium --retries=0
```

Run scoped formatting and spec checks after edits. Inspect the refreshed
360-pixel demo during a real swipe, not just after it. Preserve the database
and private Tailscale Serve route documented in the parent plan.

## Files likely touched

- `apps/web/components/threads/use-thread-column-activation.ts`
- `apps/web/components/threads/use-mobile-thread-position.ts`
- `apps/web/components/threads/thread-viewport-geometry.ts`
- `apps/web/components/threads/thread-column-activation.test.tsx`
- `apps/web/components/threads/threads-board.tsx` and its tests
- `apps/web/app/threads/threads-page-client.test.tsx`
- `apps/web/e2e/tests/task/mobile-threads-view.spec.ts`
- `apps/web/e2e/tests/task/mobile-threads-swipe.spec.ts` and its shared touch helper
- `apps/web/components/threads/AGENTS.md`
- `docs/public/sessions-and-review.md`

## Dependencies

Task 03 is complete. Read its current uncommitted implementation, not just HEAD.

## Risks

React renders on every scroll pixel, stale geometry after order/width changes,
late callbacks after unmount, or accidentally promoting additional transcripts.
Update only on identity changes and retain the existing network owners.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-THREADS-DECK-003`, especially .9, .12, and .13.
- Threads design: Phone position feedback and Responsive behavior.
- Existing activation hook tests and real-touch mobile Threads E2E helper.

## Results

Implemented after the user's explicit implementation request. Permanent unit
and real held-touch browser regressions failed on stale position before the
fix. The position signal now samples shared shell geometry once per frame and
updates header/picker without waiting for detail delivery. It also invalidates
the nearest-visible detail calculation: browser coverage exposed zero-area
edge intersections retaining the prior conversation after snap. The one-chat
budget and existing preload policy remain intact.

Passed 166 focused unit tests, seven mobile Threads tests, and 12 desktop
Threads tests, all browser tests with one worker and retries disabled.
Typecheck, scoped zero-warning lint, translation checks, and web build passed.
The shared-header work order owns the final combined demo/visual verification.
