---
spec: docs/specs/keep-transcript-position/spec.md
created: 2026-07-30
status: in-review
---

# Implementation Plan: Keep Transcript Scroll Position On Navigation

## Root Cause

Both message-list renderers already capture the transcript scroll offset
continuously and independent of the auto-scroll toggle:

- Native (`message-list-native.tsx`): `useAutoScroll`'s `captureScrollTop`
  writes `scrollTopBySessionId[sessionId]` on every scroll event
  (coalesced to one write per animation frame), unconditionally.
- Virtuoso (`message-list-virtuoso.tsx`): `captureSnapshot` writes
  `virtuosoStateBySessionId[sessionId]` on unmount (unconditionally) and
  immediately on every disable transition.

But both renderers only *read* that captured offset back at mount time
when auto-scroll is disabled:

- `resolveNativeInitialScrollTop` (`transcript-auto-scroll.ts:134-143`)
  returns `scrollHeight` (bottom) unconditionally when `enabled` is
  `true`, ignoring `savedScrollTop` entirely.
- `message-list-virtuoso.tsx`'s `restoreStateFrom` lazy initializer
  (`if (enabled || !sessionId) return undefined;`) skips restoring the
  snapshot when `enabled` is `true`.

Since auto-scroll is enabled by default and most users never touch the
toggle, any navigate-away-and-back (dockview remount) while scrolled up
snaps back to the bottom. Reproduced live via a throwaway e2e spec: a
task scrolled to `scrollTop=989` (`scrollHeight=1979`) landed at
`scrollTop=1939` (bottom) after navigating to the kanban board and back.

**Fix:** stop gating the mount-time restore on `enabled`. Always prefer
the captured offset when one exists; fall back to the bottom only when
nothing has been captured yet (first-time open) or a pending dockview
layout restore takes precedence (unchanged). Capture-side logic and
in-session live auto-follow/freeze behavior are untouched.

## Frontend

- `apps/web/components/task/chat/transcript-auto-scroll.ts`:
  `resolveNativeInitialScrollTop` drops the `enabled` branch and always
  returns `params.savedScrollTop ?? params.scrollHeight` (after the
  existing pending-layout-restore short-circuit). Remove the now-unused
  `enabled` field from its params type and update the doc comment.
- `apps/web/components/task/chat/message-list-native.tsx`:
  `useInitialScrollPosition` stops passing `enabled` into the resolver
  call (field removed); update the function's doc comment to describe
  the new "always restore captured offset, else bottom" contract.
- `apps/web/components/task/chat/message-list-virtuoso.tsx`:
  `restoreStateFrom`'s lazy initializer drops `enabled ||` from its
  guard — `if (!sessionId) return undefined;` — so it returns the
  captured `virtuosoStateBySessionId[sessionId]` snapshot whenever one
  exists, regardless of the toggle. No new exported function: the guard
  is a single inline condition, same shape as today, just without the
  `enabled` check. Update the surrounding comment (currently "Restore
  the saved position on first mount when disabled").
- `apps/web/components/task/chat/auto-scroll-toggle-button.tsx`'s doc
  comment cross-references both renderer files; update its wording if it
  describes the old enabled-gated restore behavior.

## Tests

- **Unit:** `apps/web/components/task/chat/transcript-auto-scroll.test.ts`
  — update the existing `resolveNativeInitialScrollTop` case for
  `enabled: true, savedScrollTop: 250` to expect `250` (not
  `scrollHeight`); keep the pending-layout-restore and
  no-saved-offset-falls-back-to-bottom cases unchanged. The virtuoso
  gate has no extracted pure function today and gets none here — it
  stays a single inline condition in `message-list-virtuoso.tsx`,
  covered by the E2E case below rather than a new unit test file.

## E2E Tests

- Extend `apps/web/e2e/tests/chat/auto-scroll-toggle.spec.ts` (or a
  sibling spec if scope grows) with a regression test that leaves
  auto-scroll **enabled** (no toggle click), scrolls to mid-transcript,
  navigates to the kanban board and back, and asserts the restored
  `scrollTop` is within tolerance of the pre-navigation value — mirroring
  the existing "preserves the frozen scroll position across navigating
  away and back" test's structure but without disabling the toggle.
- Add the same scenario against the virtuoso renderer (`?renderer=virtuoso`
  navigation, or however the suite already parameterizes renderer choice
  for long-transcript coverage) so both production-reachable renderers
  are covered.
- Confirm the existing disabled-state tests in the same file still pass
  unmodified (this fix only changes the enabled path).

## Implementation Waves

1. [ ] [task-01-native-renderer-restore](task-01-native-renderer-restore.md)
2. [ ] [task-02-virtuoso-renderer-restore](task-02-virtuoso-renderer-restore.md)
3. [ ] [task-03-e2e-and-verification](task-03-e2e-and-verification.md)

Tasks 01 and 02 touch disjoint files (native vs. virtuoso renderer) and
share no schema, migration, or generated contract — parallel-safe if the
user explicitly authorizes subagents. Task 03 depends on both.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/transcript-auto-scroll.test.ts
cd apps/web && pnpm e2e:run tests/chat/auto-scroll-toggle.spec.ts
cd apps/web && pnpm run typecheck && pnpm run lint
```

## Risks

- Removing the `enabled` gate must not affect live in-session auto-follow
  (new messages pulling the view to the bottom while mounted and
  enabled) — that path is driven by `useAutoScroll`'s scroll-event
  handling, not by the mount-time resolver, but each task must confirm
  the existing enabled-and-following e2e cases still pass.
- Virtuoso's `virtuosoStateBySessionId` is in-memory only (no
  sessionStorage fallback like native's `getStoredAutoScrollTop`) — a
  full page reload still falls back to the bottom for virtuoso; this
  plan does not add that fallback (out of scope per spec).
